<script lang="ts">
  import { onMount } from 'svelte'
  import { Cards, Focus, Kill, NewSession, Screen } from '../wailsjs/go/app/App'
  import type { core } from '../wailsjs/go/models'
  import Card from './Card.svelte'
  import Fullscreen from './Fullscreen.svelte'
  import PromptBar from './PromptBar.svelte'
  import type { GridSize } from './lib/bodySize'
  import { errorMessage } from './lib/errors'
  import { gridKeyAction, type GridAction } from './lib/keys'
  import { layout } from './lib/layout'
  import { move } from './lib/navigation'
  import { createRefresher } from './lib/refresher'
  import { createFailureTracker } from './lib/screenFailures'
  import { reconcileSelection } from './lib/selection'

  let { warning }: { warning: string } = $props()

  const NOTICE_MS = 6000

  let cards = $state.raw<core.Card[]>([])
  let screens = $state.raw<Record<number, core.Screen>>({})
  let selected = $state<number | null>(null)
  let fullscreen = $state<core.Card | null>(null)
  let confirmKill = $state<core.Card | null>(null)
  let unavailable = $state.raw<Set<number>>(new Set())
  let notice = $state('')
  let warningDismissed = $state(false)
  let areaWidth = $state(0)
  let areaHeight = $state(0)
  let promptBar: PromptBar
  let cancelButton: HTMLButtonElement | undefined = $state()
  let noticeTimer: ReturnType<typeof setTimeout> | undefined

  const sizes = new Map<number, GridSize>()
  const visible = new Set<number>()
  const failures = createFailureTracker()

  const placed = $derived(layout(cards.length, areaWidth, areaHeight))
  const selectedCard = $derived(cards.find((c) => c.PID === selected) ?? null)

  $effect(() => {
    cancelButton?.focus()
  })

  function showNotice(text: string): void {
    notice = text
    clearTimeout(noticeTimer)
    noticeTimer = setTimeout(() => (notice = ''), NOTICE_MS)
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
    syncUnavailable(list)
    for (const card of list) {
      const size = sizes.get(card.PID)
      if (!card.InTmux || !size || !visible.has(card.PID)) continue
      if (fullscreen !== null || document.hidden) return
      try {
        screens = { ...screens, [card.PID]: await Screen(card.PID, size.rows) }
        failures.succeed(card.PID)
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
      showNotice(errorMessage(e))
      return
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
    refresher.start()
    const onVisibility = (): void => {
      if (!document.hidden) refresher.kick()
    }
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      refresher.stop()
      document.removeEventListener('visibilitychange', onVisibility)
      clearTimeout(noticeTimer)
    }
  })

  function openFullscreen(card: core.Card | null): void {
    if (!card) return
    if (!card.InTmux) {
      showNotice('Cette session ne tourne pas dans tmux : lecture seule.')
      return
    }
    fullscreen = card
  }

  function closeFullscreen(): void {
    if (fullscreen === null) return
    fullscreen = null
    refresher.kick()
  }

  async function focusCard(pid: number): Promise<void> {
    selected = pid
    try {
      const result = await Focus(pid)
      if (result.Warning) showNotice(result.Warning)
      if (result.Mode === 'embedded') openFullscreen(cards.find((c) => c.PID === pid) ?? null)
    } catch (e) {
      showNotice(errorMessage(e))
    }
  }

  async function newSession(): Promise<void> {
    try {
      await NewSession()
      refresher.kick()
    } catch (e) {
      showNotice(errorMessage(e))
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
      showNotice(errorMessage(e))
    }
  }

  function askKill(card: core.Card | null): void {
    if (!card) return
    if (!card.Private) {
      showNotice("Seules les sessions lancées par l'application peuvent être arrêtées.")
      return
    }
    confirmKill = card
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
        void newSession()
        break
      case 'kill':
        askKill(selectedCard)
        break
    }
  }

  function onKeydown(e: KeyboardEvent): void {
    if (fullscreen !== null) return
    if (confirmKill !== null) {
      const onButton = e.target instanceof HTMLButtonElement
      if ((e.key === 'Enter' && !onButton) || e.key === 'y') void killConfirmed()
      else if (e.key === 'Escape' || e.key === 'n') confirmKill = null
      else return
      e.preventDefault()
      return
    }
    const target = e.target
    if (target instanceof HTMLTextAreaElement || target instanceof HTMLInputElement) return
    const action = gridKeyAction(e)
    if (action === null) return
    e.preventDefault()
    run(action)
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="tower">
  {#if warning !== '' && !warningDismissed}
    <div class="banner warning">
      <span>{warning}</span>
      <button onclick={() => (warningDismissed = true)}>Fermer</button>
    </div>
  {/if}
  {#if notice !== ''}
    <div class="banner notice">{notice}</div>
  {/if}

  <div class="area" bind:clientWidth={areaWidth} bind:clientHeight={areaHeight}>
    {#if cards.length === 0}
      <p class="empty">Aucune session. Appuyez sur n pour en lancer une.</p>
    {/if}
    <div class="canvas" style:height="{placed.scrolls ? placed.contentHeight : areaHeight}px">
      {#each cards as card, i (card.PID)}
        {#if placed.rects[i]}
          <Card
            {card}
            screen={screens[card.PID]}
            unavailable={unavailable.has(card.PID)}
            rect={placed.rects[i]}
            selected={card.PID === selected}
            onSelect={(pid) => (selected = pid)}
            onFocus={(pid) => void focusCard(pid)}
            {onMeasure}
            {onVisible}
          />
        {/if}
      {/each}
    </div>
  </div>

  <PromptBar
    bind:this={promptBar}
    card={selectedCard}
    onSent={() => refresher.kick()}
  />

  {#if fullscreen !== null}
    <Fullscreen pid={fullscreen.PID} name={fullscreen.Name} onExit={closeFullscreen} />
  {/if}

  {#if confirmKill !== null}
    <div class="modal" role="alertdialog" aria-modal="true" aria-label="Arrêter la session">
      <div class="dialog">
        <p>Arrêter la session {confirmKill.Name} (PID {confirmKill.PID}) ?</p>
        <div class="actions">
          <button bind:this={cancelButton} onclick={() => (confirmKill = null)}>Annuler (Échap)</button>
          <button class="danger" onclick={() => void killConfirmed()}>Arrêter (Entrée)</button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .tower {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .banner {
    flex: none;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 6px 12px;
    color: var(--bg);
  }

  .warning {
    background: var(--warn);
  }

  .notice {
    background: var(--info);
  }

  .banner button {
    margin-left: auto;
  }

  .area {
    position: relative;
    flex: 1;
    min-height: 0;
    overflow-x: hidden;
    overflow-y: auto;
  }

  .canvas {
    position: relative;
    width: 100%;
  }

  .empty {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    margin: 0;
    color: var(--muted);
  }

  .modal {
    position: fixed;
    inset: 0;
    z-index: 20;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.6);
  }

  .dialog {
    padding: 16px 20px;
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 6px;
  }

  .dialog p {
    margin: 0 0 16px;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }

  .danger {
    color: var(--danger);
  }
</style>
