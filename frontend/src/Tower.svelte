<script lang="ts">
  import { onMount } from 'svelte'
  import { Cards, FitSession, Focus, AckQuit, GuardQuit, Kill, NewSession, Quit, Screen, StartFailures } from '../wailsjs/go/app/App'
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
  import { errorMessage } from './lib/errors'
  import { routeKey, type KeyTarget } from './lib/keyRouting'
  import { closeLayer, confirmation, keyLayers, layerOf, openLayer, type LayerKind, type Layers } from './lib/layers'
  import type { GridAction } from './lib/keys'
  import { backgroundOf } from './lib/cardText'
  import { insetRect, layout } from './lib/layout'
  import { move } from './lib/navigation'
  import { createRefresher } from './lib/refresher'
  import { createScreenFeed } from './lib/screenFeed'
  import { summarizeStates } from './lib/summary'
  import { EventsOn } from '../wailsjs/runtime/runtime'
  import { isQuitShortcut } from './lib/quitShortcut'
  import { createToasts, type Toast, type ToastKind } from './lib/toasts'
  import { createTypingPause } from './lib/typingPause'

  let { warning }: { warning: string } = $props()

  const GRID_PADDING = 7
  const CARD_GAP = 10

  let cards = $state.raw<core.Card[]>([])
  let screens = $state.raw<Record<number, core.Screen>>({})
  let selected = $state<number | null>(null)
  let layers = $state.raw<Layers>([])
  let unavailable = $state.raw<Set<number>>(new Set())
  let loaded = $state(false)
  let toasts = $state.raw<Toast[]>([])
  let areaWidth = $state(0)
  let areaHeight = $state(0)
  let promptBar: PromptBar

  const notifier = createToasts((list) => (toasts = list))
  // The thumbnails are not refreshed while the user types, so typing stays responsive.
  const typing = createTypingPause({ onResume: () => refresher.kick() })
  const feed = createScreenFeed({
    cards: Cards,
    screen: Screen,
    fitSession: FitSession,
    startFailures: StartFailures,
    fullscreenOpen: () => fullscreen !== null,
    documentHidden: () => document.hidden,
    typing: () => typing.active(),
    getSelected: () => selected,
    requestRefresh: () => refresher.kick(),
    onLoaded: () => (loaded = true),
    onCards: (list) => (cards = list),
    onScreens: (list) => (screens = list),
    onUnavailable: (list) => (unavailable = list),
    onSelected: (pid) => (selected = pid),
    notify,
  })

  const gridWidth = $derived(Math.max(0, areaWidth - 2 * GRID_PADDING))
  const gridHeight = $derived(Math.max(0, areaHeight - 2 * GRID_PADDING))
  const placed = $derived(layout(cards.length, gridWidth, gridHeight))
  const summary = $derived(summarizeStates(cards))
  const selectedCard = $derived(cards.find((c) => c.PID === selected) ?? null)
  const fullscreen = $derived(layerOf(layers, 'fullscreen')?.card ?? null)
  const confirmKill = $derived(layerOf(layers, 'kill')?.card ?? null)
  const confirmQuit = $derived(layerOf(layers, 'quit') !== undefined)
  const helpOpen = $derived(layerOf(layers, 'help') !== undefined)
  const newSessionOpen = $derived(layerOf(layers, 'newSession') !== undefined)

  function notify(kind: ToastKind, text: string): void {
    notifier.push(kind, text)
  }

  function dismiss(kind: LayerKind): void {
    layers = closeLayer(layers, kind)
  }

  function openHelp(): void {
    layers = openLayer(layers, { kind: 'help' })
  }

  const refresher = createRefresher({
    run: feed.cycle,
    paused: () => document.hidden || fullscreen !== null,
  })

  onMount(() => {
    if (warning !== '') notify('warning', warning)
    refresher.start()
    const onVisibility = (): void => {
      if (!document.hidden) refresher.kick()
    }
    document.addEventListener('visibilitychange', onVisibility)
    // Closing the window asks here first while this view can show the confirmation.
    const stopQuitRequests = EventsOn('quit-requested', onQuitRequested)
    const unguard = (): void => void GuardQuit(false)
    // A reload may not deliver this call; the close request then goes unanswered, and the window closes after the wait.
    window.addEventListener('beforeunload', unguard)
    void GuardQuit(true)
    return () => {
      unguard()
      window.removeEventListener('beforeunload', unguard)
      stopQuitRequests()
      refresher.stop()
      typing.end()
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
    layers = openLayer(layers, { kind: 'fullscreen', card })
  }

  function closeFullscreen(): void {
    if (fullscreen === null) return
    feed.forgetFit(fullscreen.PID)
    dismiss('fullscreen')
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
    layers = openLayer(layers, { kind: 'newSession' })
  }

  // A start in progress, folder dialog included, keeps a second one from opening another dialog.
  let starting = false

  async function startSession(name: string): Promise<void> {
    dismiss('newSession')
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
    layers = openLayer(layers, { kind: 'quit' })
  }

  /** Answers a close request of the window: the confirmation shows above everything else. */
  function onQuitRequested(): void {
    void AckQuit()
    closeFullscreen()
    dismiss('help')
    dismiss('newSession')
    askQuit()
  }

  async function quitConfirmed(): Promise<void> {
    dismiss('quit')
    try {
      await Quit()
    } catch (e) {
      notify('error', errorMessage(e))
    }
  }

  async function killConfirmed(): Promise<void> {
    const card = confirmKill
    dismiss('kill')
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
    layers = openLayer(layers, { kind: 'kill', card })
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
        openHelp()
        break
    }
  }

  function keyTarget(target: EventTarget | null): KeyTarget {
    if (target instanceof HTMLTextAreaElement || target instanceof HTMLInputElement) return 'text'
    return target instanceof HTMLButtonElement ? 'button' : 'other'
  }

  function onKeydown(e: KeyboardEvent): void {
    if (isQuitShortcut(e, layers.length > 0)) {
      e.preventDefault()
      askQuit()
      return
    }
    const route = routeKey(keyLayers(layers), e, keyTarget(e.target))
    if (route === null) return
    e.preventDefault()
    if (route.layer === 'confirm') {
      if (confirmation(layers) === 'quit') {
        if (route.action === 'accept') void quitConfirmed()
        else dismiss('quit')
      } else if (route.action === 'accept') void killConfirmed()
      else dismiss('kill')
    } else if (route.layer === 'newSession') {
      dismiss('newSession')
    } else if (route.layer === 'help') {
      dismiss('help')
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
  <AppBar {summary} onHelp={openHelp} onNew={newSession} onQuit={askQuit} />

  <div class="main">
    <div class="area" bind:clientWidth={areaWidth} bind:clientHeight={areaHeight}>
      {#if cards.length === 0}
        {#if loaded}
          <EmptyState onNew={newSession} />
        {/if}
      {:else}
        <div class="canvas" style:height="{placed.scrolls ? placed.contentHeight : gridHeight}px">
          {#each cards as card, i (card.PID)}
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
              onMeasure={feed.measure}
              onFit={feed.fit}
              onVisible={feed.visibility}
            />
          {/each}
        </div>
      {/if}
    </div>
    <Toasts {toasts} onDismiss={(id) => notifier.dismiss(id)} />
  </div>

  <PromptBar
    bind:this={promptBar}
    card={selectedCard}
    dropBlocked={layers.length > 0}
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
    <ShortcutsDialog onClose={() => dismiss('help')} />
  {/if}

  {#if newSessionOpen}
    <NewSessionDialog onSubmit={(name) => void startSession(name)} onClose={() => dismiss('newSession')} />
  {/if}

  {#if confirmQuit}
    <Dialog title="Quitter Claude Tower" role="alertdialog" onClose={() => dismiss('quit')}>
      <p class="confirm">
        Les sessions continuent de tourner après la fermeture : quitter l’application ne les arrête pas. Tu les
        retrouves en la relançant.
      </p>
      <div class="actions">
        <button class="btn" title="Échap" onclick={() => dismiss('quit')}>Annuler</button>
        <button class="btn btn-primary" data-autofocus title="Entrée" onclick={() => void quitConfirmed()}>Quitter</button>
      </div>
    </Dialog>
  {:else if confirmKill !== null}
    <Dialog title="Arrêter la session" role="alertdialog" onClose={() => dismiss('kill')}>
      <p class="confirm">
        {#if confirmKill.Detached}
          Arrêter la session d’arrière-plan <strong>{confirmKill.Name}</strong> ? Sa conversation est
          conservée : <code>claude --resume</code> la retrouve.
        {:else}
          Arrêter la session <strong>{confirmKill.Name}</strong> (PID {confirmKill.PID}) ? Le processus sera
          interrompu.
        {/if}
      </p>
      <div class="actions">
        <button class="btn" data-autofocus title="Échap" onclick={() => dismiss('kill')}>Annuler</button>
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
