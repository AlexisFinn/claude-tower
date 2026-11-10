<script module lang="ts">
  import { measureFont, THUMBNAIL_FONT_FAMILY } from './lib/thumbnail'

  // The font is the same in every card, and loaded before they mount, so its cell size per pixel of font is measured once.
  const cellRatio = measureFont(THUMBNAIL_FONT_FAMILY).cell
</script>

<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { Unicode11Addon } from '@xterm/addon-unicode11'
  import type { core } from '../wailsjs/go/models'
  import Icon from './Icon.svelte'
  import StatePill from './StatePill.svelte'
  import { parseActivity } from './lib/activity'
  import { bodyGeometry, fitGrid, sameGeometry, type BodyGeometry, type GridSize } from './lib/bodySize'
  import { backgroundLabel, type BackgroundTasks } from './lib/cardText'
  import { displayName, nameTag } from './lib/displayName'
  import { watchDevicePixelRatio } from './lib/devicePixelRatio'
  import { frameClass } from './lib/frame'
  import type { Rect } from './lib/layout'
  import { screenSequence } from './lib/screenSequence'
  import { createThumbnail, type Thumbnail } from './lib/thumbnail'

  interface Props {
    card: core.Card
    screen: core.Screen | undefined
    unavailable: boolean
    rect: Rect
    selected: boolean
    background?: BackgroundTasks
    onSelect: (pid: number) => void
    onFocus: (pid: number) => void
    onFullscreen: (pid: number) => void
    onKill: (pid: number) => void
    onMeasure: (pid: number, size: GridSize | null) => void
    /** Receives the terminal size that fills the body at a legible font, for a session the application may resize. */
    onFit: (pid: number, size: GridSize | null) => void
    onVisible: (pid: number, visible: boolean) => void
  }

  let {
    card,
    screen,
    unavailable,
    rect,
    selected,
    background,
    onSelect,
    onFocus,
    onFullscreen,
    onKill,
    onMeasure,
    onFit,
    onVisible,
  }: Props = $props()

  let root: HTMLDivElement
  let canvas: HTMLCanvasElement | undefined = $state()
  let bodyWidth = $state(0)
  let bodyHeight = $state(0)
  let term = $state.raw<Terminal | null>(null)
  let thumbnail = $state.raw<Thumbnail | null>(null)
  let geometry = $state.raw<BodyGeometry | null>(null)
  let dpr = $state(window.devicePixelRatio || 1)
  let lastContentKey = ''
  let lastPaintKey = ''
  let current: { geometry: BodyGeometry; dpr: number } | null = null

  // The card object is replaced on every refresh: effects depend on these values instead.
  const pid = $derived(card.PID)

  const inTmux = $derived(card.InTmux)
  const activity = $derived(parseActivity(card.Activity))
  const activityIcon = $derived(
    activity.kind === 'tool' ? 'zap' : activity.kind === 'prompt' ? 'arrowRight' : 'message',
  )
  const backgroundText = $derived(backgroundLabel(background))
  const shownName = $derived(displayName(card))
  const tagName = $derived(nameTag(card))
  const paneCols = $derived(screen && screen.Width > 0 ? screen.Width : null)
  const paneRows = $derived(screen && screen.Height > 0 ? screen.Height : null)

  $effect(() => {
    if (!inTmux || !canvas) return
    const t = new Terminal({
      cols: 80,
      rows: 24,
      disableStdin: true,
      scrollback: 0,
      allowProposedApi: true,
    })
    t.loadAddon(new Unicode11Addon())
    t.unicode.activeVersion = '11'
    lastContentKey = ''
    lastPaintKey = ''
    term = t
    thumbnail = createThumbnail(canvas)
    return () => {
      term = null
      thumbnail = null
      current = null
      t.dispose()
    }
  })

  onMount(() => {
    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) onVisible(pid, entry.isIntersecting)
    })
    observer.observe(root)
    const stopWatching = watchDevicePixelRatio((ratio) => (dpr = ratio))
    return () => {
      observer.disconnect()
      stopWatching()
      onVisible(pid, false)
      onMeasure(pid, null)
      onFit(pid, null)
    }
  })

  // The geometry only depends on the body and pane sizes, and the canvas is
  // taken out of the flow, so drawing never changes the body size again.
  $effect(() => {
    if (!inTmux || bodyWidth <= 0 || bodyHeight <= 0) return
    const next = bodyGeometry(paneCols, paneRows, bodyWidth, bodyHeight, cellRatio)
    if (!sameGeometry(untrack(() => geometry), next)) geometry = next
    onMeasure(pid, { cols: next.cols, rows: next.rows })
  })

  const resizable = $derived(card.Private && !card.Detached)
  $effect(() => {
    if (!resizable || !inTmux || bodyWidth <= 0 || bodyHeight <= 0) {
      onFit(pid, null)
      return
    }
    onFit(pid, fitGrid(bodyWidth, bodyHeight, cellRatio))
  })

  /** Draws the buffer with the latest geometry; a write still pending finishes by calling it again. */
  function paint(): void {
    if (!term || !thumbnail || !current) return
    thumbnail.draw(term.buffer.active, current.geometry, current.dpr)
  }

  $effect(() => {
    const t = term
    const g = geometry
    const ratio = dpr
    if (!t || !g || !thumbnail || !screen) return
    const contentKey = `${screen.Hash}|${g.cols}x${g.rows}`
    const paintKey = `${contentKey}|${g.font}|${ratio}`
    if (paintKey === lastPaintKey) return
    lastPaintKey = paintKey
    current = { geometry: g, dpr: ratio }
    if (contentKey === lastContentKey) {
      paint()
      return
    }
    lastContentKey = contentKey
    t.resize(g.cols, g.rows)
    t.write(screenSequence(screen.Lines, g.rows), paint)
  })

  $effect(() => {
    if (selected) root.scrollIntoView({ block: 'nearest' })
  })

  const placement = $derived(`${rect.x}|${rect.y}|${rect.width}|${rect.height}`)
  let alignFrame = 0

  /** Realigns the canvas on device pixels once the layout is done, at most once per frame. */
  function scheduleAlign(): void {
    if (alignFrame !== 0) return
    alignFrame = requestAnimationFrame(() => {
      alignFrame = 0
      thumbnail?.align(dpr)
    })
  }

  // A card that moves or scrolls without redrawing realigns its canvas.
  $effect(() => {
    void placement
    if (thumbnail) scheduleAlign()
  })

  onMount(() => {
    window.addEventListener('scroll', scheduleAlign, { capture: true, passive: true })
    return () => {
      window.removeEventListener('scroll', scheduleAlign, { capture: true })
      cancelAnimationFrame(alignFrame)
    }
  })

  function act(e: MouseEvent, action: (pid: number) => void): void {
    e.stopPropagation()
    onSelect(pid)
    action(pid)
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -- the grid keys are handled on the window -->
<div
  bind:this={root}
  id="card-{pid}"
  class="card {frameClass(card.State, selected)}"
  style:left="{rect.x}px"
  style:top="{rect.y}px"
  style:width="{rect.width}px"
  style:height="{rect.height}px"
  role="group"
  aria-label="{shownName}, {card.Label}"
  tabindex="-1"
  onclick={() => onSelect(pid)}
  ondblclick={() => onFocus(pid)}
>
  <header>
    <div class="line">
      <span class="name" title={shownName}>{shownName}</span>
      {#if tagName !== ''}<span class="tag id" title={tagName}>{tagName}</span>{/if}
      <StatePill state={card.State} label={card.Label} />
      <div class="trail">
        <span class="since">{card.Since}</span>
        <!-- svelte-ignore a11y_no_static_element_interactions -- stops the double click from reaching the card -->
        <div class="tools" ondblclick={(e) => e.stopPropagation()}>
          {#if card.InTmux}
            <button class="icon-btn" aria-label="Plein écran" title="Plein écran (o)" onclick={(e) => act(e, onFullscreen)}>
              <Icon name="maximize" />
            </button>
          {/if}
          <button class="icon-btn" aria-label="Aller au terminal" title="Aller au terminal (Entrée)" onclick={(e) => act(e, onFocus)}>
            <Icon name="terminal" />
          </button>
          {#if card.Private || card.Detached}
            <button class="icon-btn" aria-label="Arrêter la session" title="Arrêter la session (x)" onclick={(e) => act(e, onKill)}>
              <Icon name="x" />
            </button>
          {/if}
        </div>
      </div>
    </div>
    <div class="line meta">
      <span class="cwd"><Icon name="folder" size={14} /><span class="path" title={card.Cwd}>{card.Cwd}</span></span>
      {#if card.Detached}
        <span class="tag" title="Session d’arrière-plan de Claude Code">arrière-plan</span>
      {:else}
        {#if card.Private}<span class="tag">privée</span>{/if}
        <span class="tag">{card.InTmux ? 'tmux' : 'hors tmux'}</span>
      {/if}
      {#if backgroundText !== ''}<span class="tag">{backgroundText}</span>{/if}
    </div>
  </header>

  {#if card.InTmux}
    <div class="screen">
      <div class="fill" bind:clientWidth={bodyWidth} bind:clientHeight={bodyHeight}>
        <canvas bind:this={canvas} class:ready={geometry !== null}></canvas>
      </div>
      {#if unavailable}
        <div class="stale" role="status">
          <Icon name="monitorOff" size={18} />
          <span>Écran indisponible</span>
        </div>
      {/if}
    </div>
    <footer title={card.Activity}>
      <span class="kind {activity.kind}"><Icon name={activityIcon} size={14} /></span>
      <span class="last" class:none={activity.kind === 'none'}>
        {activity.kind === 'none' ? 'Aucune activité' : activity.text}
      </span>
    </footer>
  {:else}
    <div class="screen plain">
      <p class="activity-text">{card.Activity === '' ? 'Aucune activité' : card.Activity}</p>
      {#if card.Detached}
        <p class="readonly"><Icon name="terminal" size={14} />Session d’arrière-plan : rattachement en cours…</p>
      {:else}
        <p class="readonly"><Icon name="lock" size={14} />Lecture seule : session hors tmux</p>
      {/if}
    </div>
  {/if}
</div>

<style>
  .card {
    position: absolute;
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 10px;
    overflow: hidden;
    background: var(--surface);
    border: 1px solid color-mix(in srgb, var(--state) 45%, transparent);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow), 0 0 0 0 transparent, 0 0 0 0 transparent;
    cursor: default;
    transition:
      box-shadow 150ms var(--ease),
      border-color 150ms var(--ease);
  }

  .card::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 2px;
    background: var(--state);
    pointer-events: none;
  }

  .card.selected {
    box-shadow:
      var(--shadow),
      0 0 0 2px var(--state),
      0 0 0 6px var(--state-soft);
  }

  .card.frame-unknown {
    border-style: dashed;
  }

  .card.frame-unknown::before {
    opacity: 0.5;
  }

  .card:focus-visible {
    outline: none;
  }

  .card.frame-waiting {
    animation: halo 2.4s ease-in-out infinite;
  }

  .card.frame-waiting.selected {
    animation-name: halo-selected;
  }

  header {
    flex: none;
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
  }

  .line {
    position: relative;
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    min-height: 28px;
  }

  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    font-size: 14px;
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .trail {
    flex: none;
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .since {
    font-size: 11.5px;
    color: var(--text-3);
    white-space: nowrap;
  }

  /* The buttons keep their place in the line while hidden, so that showing
     them never covers the state pill or the duration. */
  .tools {
    display: flex;
    gap: 2px;
    opacity: 0;
    pointer-events: none;
    transition: opacity 120ms var(--ease);
  }

  .card:hover .tools,
  .card:has(.tools :focus-visible) .tools {
    opacity: 1;
    pointer-events: auto;
  }

  .meta {
    min-height: 0;
    gap: 6px;
    overflow: hidden;
  }

  .cwd {
    display: inline-flex;
    align-items: center;
    flex: 0 1 auto;
    gap: 5px;
    min-width: 0;
    color: var(--text-2);
  }

  .path {
    overflow: hidden;
    font-family: var(--mono);
    font-size: 11.5px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .tag {
    flex: none;
    padding: 0 7px;
    font-size: 11px;
    line-height: 17px;
    color: var(--text-3);
    white-space: nowrap;
    border: 1px solid var(--border);
    border-radius: 999px;
  }

  .tag.id {
    flex: 0 1 auto;
    min-width: 0;
    max-width: 8em;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .screen {
    position: relative;
    flex: 1;
    min-height: 0;
    overflow: hidden;
    background: var(--screen);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .fill {
    position: absolute;
    inset: 4px;
    overflow: hidden;
  }

  /* The canvas is sized in CSS pixels by the thumbnail and taken out of the flow, so it is
     clipped by .fill and never changes its size. */
  canvas {
    position: absolute;
    top: 0;
    left: 0;
    display: block;
    pointer-events: none;
    user-select: none;
    visibility: hidden;
  }

  canvas.ready {
    visibility: visible;
  }

  .stale {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 6px;
    color: var(--busy);
    background: color-mix(in srgb, var(--screen) 78%, transparent);
  }

  .plain {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 12px;
  }

  .activity-text {
    margin: 0;
    overflow: hidden;
    font-size: 14px;
    line-height: 1.5;
    color: var(--text);
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }

  .readonly {
    display: flex;
    align-items: center;
    gap: 6px;
    margin: auto 0 0;
    font-size: 12px;
    color: var(--text-2);
  }

  footer {
    flex: none;
    display: flex;
    align-items: center;
    gap: 7px;
    min-width: 0;
    min-height: 18px;
  }

  .kind {
    display: flex;
    color: var(--text-3);
  }

  .kind.tool {
    color: var(--busy);
  }

  .last {
    min-width: 0;
    overflow: hidden;
    font-size: 12px;
    color: var(--text-2);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .last.none {
    color: var(--text-3);
  }

  @keyframes halo {
    0%,
    100% {
      box-shadow: var(--shadow), 0 0 0 0 transparent, 0 0 0 3px var(--state-soft);
    }
    50% {
      box-shadow:
        var(--shadow),
        0 0 0 0 transparent,
        0 0 0 8px color-mix(in srgb, var(--state) 5%, transparent);
    }
  }

  @keyframes halo-selected {
    0%,
    100% {
      box-shadow: var(--shadow), 0 0 0 2px var(--state), 0 0 0 6px var(--state-soft);
    }
    50% {
      box-shadow:
        var(--shadow),
        0 0 0 2px var(--state),
        0 0 0 10px color-mix(in srgb, var(--state) 5%, transparent);
    }
  }
</style>
