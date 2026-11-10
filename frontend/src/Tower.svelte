<script lang="ts">
  import { onMount } from 'svelte'
  import { Cards, FitSession, Focus, GuardQuit, Kill, NewSession, Quit, Screen, StartFailures } from '../wailsjs/go/app/App'
  import type { core } from '../wailsjs/go/models'
  import AppBar from './AppBar.svelte'
  import Card from './Card.svelte'
  import Dialog from './Dialog.svelte'
  import EmptyState from './EmptyState.svelte'
  import Fullscreen from './Fullscreen.svelte'
  import NewSessionDialog from './NewSessionDialog.svelte'
  import PromptBar from './PromptBar.svelte'
  import ShortcutsDialog from './ShortcutsDialog.svelte'
  import Toasts from './Toasts.svelte'
  import type { GridSize } from './lib/bodySize'
  import { displayName } from './lib/displayName'
  import { errorMessage } from './lib/errors'
  import { routeKey, type KeyTarget } from './lib/keyRouting'
  import type { GridAction } from './lib/keys'
  import { backgroundOf } from './lib/cardText'
  import { insetRect, layout } from './lib/layout'
  import { move } from './lib/navigation'
  import { createRefresher } from './lib/refresher'
  import { createFailureTracker } from './lib/screenFailures'
  import { reconcileSelection } from './lib/selection'
  import { startFailureText } from './lib/startFailures'
  import { summarizeStates } from './lib/summary'
  import { EventsOn } from '../wailsjs/runtime/runtime'
  import { createFitRequests } from './lib/fitRequests'
  import { createToasts, type Toast, type ToastKind } from './lib/toasts'
  import { createTypingPause } from './lib/typingPause'

  let { warning }: { warning: string } = $props()

  const GRID_PADDING = 7
  const CARD_GAP = 10

  let cards = $state.raw<core.Card[]>([])
  let screens = $state.raw<Record<number, core.Screen>>({})
  let selected = $state<number | null>(null)
  let fullscreen = $state<core.Card | null>(null)
  let confirmKill = $state<core.Card | null>(null)
  let confirmQuit = $state(false)
  let unavailable = $state.raw<Set<number>>(new Set())
  let helpOpen = $state(false)
  let newSessionOpen = $state(false)
  let loaded = $state(false)
  let toasts = $state.raw<Toast[]>([])
  let areaWidth = $state(0)
  let areaHeight = $state(0)
  let promptBar: PromptBar

  const sizes = new Map<number, GridSize>()
  const fits = new Map<number, GridSize>()
  const fitRequests = createFitRequests()
  const visible = new Set<number>()
  const failures = createFailureTracker()
  const notifier = createToasts((list) => (toasts = list))
  // The thumbnails are not refreshed while the user types, so typing stays responsive.
  const typing = createTypingPause({ onResume: () => refresher.kick() })

  const gridWidth = $derived(Math.max(0, areaWidth - 2 * GRID_PADDING))
  const gridHeight = $derived(Math.max(0, areaHeight - 2 * GRID_PADDING))
  const placed = $derived(layout(cards.length, gridWidth, gridHeight))
  const summary = $derived(summarizeStates(cards))
  const selectedCard = $derived(cards.find((c) => c.PID === selected) ?? null)

  function notify(kind: ToastKind, text: string): void {
    notifier.push(kind, text)
  }

  function sameSize(a: GridSize | undefined, b: GridSize): boolean {
    return a !== undefined && a.cols === b.cols && a.rows === b.rows
  }

  function onMeasure(pid: number, size: GridSize | null): void {
    if (size === null) {
      sizes.delete(pid)
      return
    }
    const known = sizes.get(pid)
    sizes.set(pid, size)
    if (!sameSize(known, size)) refresher.kick()
  }

  function onFit(pid: number, size: GridSize | null): void {
    if (size === null) {
      fits.delete(pid)
      fitRequests.forget(pid)
    } else {
      fits.set(pid, size)
    }
  }

  /**
   * Resizes a session the application launched to the size its card wants,
   * when its last screen has another size. The engine leaves alone a session
   * that a client shows, such as the fullscreen view.
   */
  async function fit(card: core.Card, screen: core.Screen): Promise<void> {
    const size = fits.get(card.PID)
    if (!size || !fitRequests.shouldAsk(card.PID, size, { cols: screen.Width, rows: screen.Height }, card.Attached)) return
    const ticket = fitRequests.ticket(card.PID)
    try {
      const applied = await FitSession(card.PID, size.cols, size.rows)
      fitRequests.record(card.PID, size, applied ? 'applied' : 'kept', ticket)
    } catch {
      fitRequests.record(card.PID, size, 'failed', ticket)
    }
  }

  function onVisible(pid: number, isVisible: boolean): void {
    if (!isVisible) {
      visible.delete(pid)
      return
    }
    if (visible.has(pid)) return
    visible.add(pid)
    refresher.kick()
  }

  function syncUnavailable(list: core.Card[]): void {
    const next = new Set(list.filter((c) => failures.unavailable(c.PID)).map((c) => c.PID))
    const same = next.size === unavailable.size && [...next].every((pid) => unavailable.has(pid))
    if (!same) unavailable = next
  }

  async function loadScreens(list: core.Card[]): Promise<void> {
    const next: Record<number, core.Screen> = {}
    for (const card of list) {
      const known = screens[card.PID]
      if (known) next[card.PID] = known
    }
    screens = next
    failures.prune(list.map((c) => c.PID))
    fitRequests.prune(list.map((c) => c.PID))
    syncUnavailable(list)
    for (const card of list) {
      const size = sizes.get(card.PID)
      if (!card.InTmux || !size || !visible.has(card.PID)) continue
      if (fullscreen !== null || document.hidden || typing.active()) return
      try {
        const screen = await Screen(card.PID, size.rows)
        screens = { ...screens, [card.PID]: screen }
        failures.succeed(card.PID)
        if (fullscreen === null) await fit(card, screen)
      } catch {
        failures.fail(card.PID)
      }
      syncUnavailable(list)
    }
  }

  async function cycle(): Promise<void> {
    let list: core.Card[]
    try {
      list = [...(await Cards())].sort((a, b) => a.Order - b.Order)
    } catch (e) {
      notify('error', errorMessage(e))
      return
    }
    loaded = true
    try {
      for (const f of await StartFailures()) notify('error', startFailureText(f))
    } catch {
      // A failed read leaves the failures for the next cycle.
    }
    const oldPids = cards.map((c) => c.PID)
    cards = list
    selected = reconcileSelection(
      oldPids,
      list.map((c) => c.PID),
      selected,
    )
    await loadScreens(list)
  }

  const refresher = createRefresher({
    run: cycle,
    paused: () => document.hidden || fullscreen !== null,
  })

  onMount(() => {
    if (warning !== '') notify('warning', warning)
    refresher.start()
    const onVisibility = (): void => {
      if (!document.hidden) refresher.kick()
    }
    document.addEventListener('visibilitychange', onVisibility)
    // Closing the window asks here first, now that a confirmation can show.
    const stopQuitRequests = EventsOn('quit-requested', askQuit)
    void GuardQuit()
    return () => {
      stopQuitRequests()
      refresher.stop()
      typing.dispose()
      document.removeEventListener('visibilitychange', onVisibility)
      notifier.dispose()
    }
  })

  function openFullscreen(card: core.Card | null): void {
    if (!card) return
    if (!card.InTmux) {
      notify('info', 'Cette session ne tourne pas dans tmux : lecture seule.')
      return
    }
    fullscreen = card
  }

  function closeFullscreen(): void {
    if (fullscreen === null) return
    fitRequests.forget(fullscreen.PID)
    fullscreen = null
    refresher.kick()
  }

  async function focusCard(pid: number): Promise<void> {
    selected = pid
    try {
      const result = await Focus(pid)
      if (result.Warning) notify('warning', result.Warning)
      if (result.Mode === 'embedded') openFullscreen(cards.find((c) => c.PID === pid) ?? null)
    } catch (e) {
      notify('error', errorMessage(e))
    }
  }

  function newSession(): void {
    newSessionOpen = true
  }

  // A start in progress, folder dialog included, keeps a second one from opening another dialog.
  let starting = false

  async function startSession(name: string): Promise<void> {
    newSessionOpen = false
    if (starting) return
    starting = true
    try {
      await NewSession(name)
      refresher.kick()
    } catch (e) {
      notify('error', errorMessage(e))
    } finally {
      starting = false
    }
  }

  function askQuit(): void {
    confirmQuit = true
  }

  async function quitConfirmed(): Promise<void> {
    confirmQuit = false
    try {
      await Quit()
    } catch (e) {
      notify('error', errorMessage(e))
    }
  }

  async function killConfirmed(): Promise<void> {
    const card = confirmKill
    confirmKill = null
    if (!card) return
    try {
      await Kill(card.PID)
      refresher.kick()
    } catch (e) {
      notify('error', errorMessage(e))
    }
  }

  function askKill(card: core.Card | null): void {
    if (!card) return
    if (!card.Private && !card.Detached) {
      notify('info', "Seules les sessions lancées par l'application peuvent être arrêtées.")
      return
    }
    confirmKill = card
  }

  function openFullscreenOf(pid: number): void {
    openFullscreen(cards.find((c) => c.PID === pid) ?? null)
  }

  function askKillOf(pid: number): void {
    askKill(cards.find((c) => c.PID === pid) ?? null)
  }

  function locateSelected(): void {
    if (selected === null) return
    document.getElementById(`card-${selected}`)?.scrollIntoView({ block: 'nearest' })
  }

  function moveSelection(dir: 'left' | 'right' | 'up' | 'down'): void {
    if (cards.length === 0) return
    const index = cards.findIndex((c) => c.PID === selected)
    if (index < 0) {
      selected = cards[0].PID
      return
    }
    selected = cards[move(placed.rects, index, dir)].PID
  }

  function run(action: GridAction): void {
    switch (action.kind) {
      case 'move':
        moveSelection(action.dir)
        break
      case 'focus':
        if (selected !== null) void focusCard(selected)
        break
      case 'fullscreen':
        openFullscreen(selectedCard)
        break
      case 'prompt':
        promptBar.focus()
        break
      case 'new':
        newSession()
        break
      case 'kill':
        askKill(selectedCard)
        break
      case 'help':
        helpOpen = true
        break
    }
  }

  function keyTarget(target: EventTarget | null): KeyTarget {
    if (target instanceof HTMLTextAreaElement || target instanceof HTMLInputElement) return 'text'
    return target instanceof HTMLButtonElement ? 'button' : 'other'
  }

  function onKeydown(e: KeyboardEvent): void {
    // In fullscreen, Ctrl+Q belongs to the session.
    if (fullscreen === null && e.ctrlKey && !e.shiftKey && !e.altKey && !e.metaKey && e.key.toLowerCase() === 'q') {
      e.preventDefault()
      askQuit()
      return
    }
    const route = routeKey(
      { fullscreen: fullscreen !== null, confirmKill: confirmKill !== null || confirmQuit, newSession: newSessionOpen, help: helpOpen },
      e,
      keyTarget(e.target),
    )
    if (route === null) return
    e.preventDefault()
    if (route.layer === 'confirm') {
      if (confirmQuit) {
        if (route.action === 'accept') void quitConfirmed()
        else confirmQuit = false
      } else if (route.action === 'accept') void killConfirmed()
      else confirmKill = null
    } else if (route.layer === 'newSession') {
      newSessionOpen = false
    } else if (route.layer === 'help') {
      helpOpen = false
    } else {
      run(route.action)
    }
  }

  function blurButton(): void {
    const active = document.activeElement
    if (active instanceof HTMLButtonElement && active.closest('[aria-modal="true"]') === null) active.blur()
  }

  function releaseMouseFocus(e: MouseEvent): void {
    if (e.detail === 0) return
    blurButton()
    setTimeout(blurButton, 0)
  }
</script>

<svelte:window onkeydown={onKeydown} onclickcapture={releaseMouseFocus} />

<div class="tower">
  <AppBar {summary} onHelp={() => (helpOpen = true)} onNew={newSession} onQuit={askQuit} />

  <div class="main">
    <div class="area" bind:clientWidth={areaWidth} bind:clientHeight={areaHeight}>
      {#if cards.length === 0}
        {#if loaded}
          <EmptyState onNew={newSession} />
        {/if}
      {:else}
        <div class="canvas" style:height="{placed.scrolls ? placed.contentHeight : gridHeight}px">
          {#each cards as card, i (card.PID)}
            {#if placed.rects[i]}
              <Card
                {card}
                background={backgroundOf(card)}
                screen={screens[card.PID]}
                unavailable={unavailable.has(card.PID)}
                rect={insetRect(placed.rects[i], CARD_GAP / 2)}
                selected={card.PID === selected}
                onSelect={(pid) => (selected = pid)}
                onFocus={(pid) => void focusCard(pid)}
                onFullscreen={openFullscreenOf}
                onKill={askKillOf}
                {onMeasure}
                {onFit}
                {onVisible}
              />
            {/if}
          {/each}
        </div>
      {/if}
    </div>
    <Toasts {toasts} onDismiss={(id) => notifier.dismiss(id)} />
  </div>

  <PromptBar
    bind:this={promptBar}
    card={selectedCard}
    dropBlocked={fullscreen !== null || helpOpen || confirmKill !== null || confirmQuit || newSessionOpen}
    onSent={() => {
      typing.end()
      refresher.kick()
    }}
    onTyping={() => typing.touch()}
    onLocate={locateSelected}
  />

  {#if fullscreen !== null}
    <Fullscreen card={fullscreen} onExit={closeFullscreen} />
  {/if}

  {#if helpOpen}
    <ShortcutsDialog onClose={() => (helpOpen = false)} />
  {/if}

  {#if newSessionOpen}
    <NewSessionDialog onSubmit={(name) => void startSession(name)} onClose={() => (newSessionOpen = false)} />
  {/if}

  {#if confirmQuit}
    <Dialog title="Quitter Claude Tower" role="alertdialog" onClose={() => (confirmQuit = false)}>
      <p class="confirm">
        Les sessions continuent de tourner après la fermeture : quitter l’application ne les arrête pas. Tu les
        retrouves en la relançant.
      </p>
      <div class="actions">
        <button class="btn" data-autofocus title="Échap" onclick={() => (confirmQuit = false)}>Annuler</button>
        <button class="btn btn-primary" title="Entrée" onclick={() => void quitConfirmed()}>Quitter</button>
      </div>
    </Dialog>
  {:else if confirmKill !== null}
    <Dialog title="Arrêter la session" role="alertdialog" onClose={() => (confirmKill = null)}>
      <p class="confirm">
        {#if confirmKill.Detached}
          Arrêter la session d’arrière-plan <strong>{displayName(confirmKill)}</strong> ? Sa conversation est
          conservée : <code>claude --resume</code> la retrouve.
        {:else}
          Arrêter la session <strong>{displayName(confirmKill)}</strong> (PID {confirmKill.PID}) ? Le processus sera
          interrompu.
        {/if}
      </p>
      <div class="actions">
        <button class="btn" data-autofocus title="Échap" onclick={() => (confirmKill = null)}>Annuler</button>
        <button class="btn btn-danger" onclick={() => void killConfirmed()}>Arrêter la session</button>
      </div>
    </Dialog>
  {/if}
</div>

<style>
  .tower {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .main {
    position: relative;
    flex: 1;
    min-height: 0;
  }

  .area {
    position: absolute;
    inset: 0;
    padding: 7px;
    overflow-x: hidden;
    overflow-y: auto;
  }

  .canvas {
    position: relative;
    width: 100%;
  }

  .confirm {
    margin: 0 0 20px;
    color: var(--text-2);
    line-height: 1.5;
  }

  .confirm strong {
    color: var(--text);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
