<script module lang="ts">
  import type { CellSize } from './lib/bodySize'

  // The font is the same in every card, so the cell size is measured once.
  let measuredCell: CellSize | null = null
</script>

<script lang="ts">
  import { onMount } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { Unicode11Addon } from '@xterm/addon-unicode11'
  import type { core } from '../wailsjs/go/models'
  import { fitBody, MAX_FONT, type GridSize } from './lib/bodySize'
  import { frameClass } from './lib/frame'
  import type { Rect } from './lib/layout'
  import { screenSequence } from './lib/screenSequence'

  interface Props {
    card: core.Card
    screen: core.Screen | undefined
    unavailable: boolean
    rect: Rect
    selected: boolean
    onSelect: (pid: number) => void
    onFocus: (pid: number) => void
    onMeasure: (pid: number, size: GridSize | null) => void
    onVisible: (pid: number, visible: boolean) => void
  }

  let {
    card,
    screen,
    unavailable,
    rect,
    selected,
    onSelect,
    onFocus,
    onMeasure,
    onVisible,
  }: Props = $props()

  // The cell size is measured at the largest font and scaled to the others.
  const REF_FONT = MAX_FONT
  const FONT_FAMILY = 'ui-monospace, "SFMono-Regular", Menlo, "DejaVu Sans Mono", monospace'
  const FALLBACK_CELL: CellSize = { width: 6.6, height: 13 }

  let root: HTMLDivElement
  let host: HTMLDivElement | undefined = $state()
  let bodyWidth = $state(0)
  let bodyHeight = $state(0)
  let term = $state.raw<Terminal | null>(null)
  let cell = $state.raw<CellSize | null>(measuredCell)
  let lastKey = ''

  const paneCols = $derived(screen && screen.Width > 0 ? screen.Width : null)
  const fit = $derived(
    term !== null && cell !== null && bodyWidth > 0 && bodyHeight > 0
      ? fitBody(paneCols, bodyWidth, bodyHeight, cell, REF_FONT)
      : null,
  )

  function measure(t: Terminal): void {
    const screenEl = t.element?.querySelector<HTMLElement>('.xterm-screen')
    if (!screenEl || screenEl.clientWidth === 0 || screenEl.clientHeight === 0) {
      cell = FALLBACK_CELL
      return
    }
    measuredCell = { width: screenEl.clientWidth / t.cols, height: screenEl.clientHeight / t.rows }
    cell = measuredCell
  }

  $effect(() => {
    if (!card.InTmux || !host) return
    const t = new Terminal({
      disableStdin: true,
      scrollback: 0,
      fontSize: REF_FONT,
      fontFamily: FONT_FAMILY,
      lineHeight: 1,
      cursorInactiveStyle: 'none',
      allowProposedApi: true,
      theme: { background: '#0d1117', foreground: '#e6edf3' },
    })
    t.loadAddon(new Unicode11Addon())
    t.unicode.activeVersion = '11'
    t.open(host)
    t.write('\x1b[?25l')
    const frame = requestAnimationFrame(() => {
      if (cell === null) measure(t)
    })
    lastKey = ''
    term = t
    return () => {
      cancelAnimationFrame(frame)
      term = null
      t.dispose()
    }
  })

  onMount(() => {
    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) onVisible(card.PID, entry.isIntersecting)
    })
    observer.observe(root)
    return () => {
      observer.disconnect()
      onVisible(card.PID, false)
      onMeasure(card.PID, null)
    }
  })

  $effect(() => {
    if (!term || !fit) return
    if (term.options.fontSize !== fit.fontSize) term.options.fontSize = fit.fontSize
    term.resize(fit.cols, fit.rows)
    onMeasure(card.PID, { cols: fit.cols, rows: fit.rows })
  })

  $effect(() => {
    if (!term || !fit || !screen) return
    const key = `${screen.Hash}|${fit.cols}x${fit.rows}`
    if (key === lastKey) return
    lastKey = key
    term.write(screenSequence(screen.Lines, fit.rows))
  })

  $effect(() => {
    if (selected) root.scrollIntoView({ block: 'nearest' })
  })
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -- the grid keys are handled on the window -->
<div
  bind:this={root}
  class="card {frameClass(card.State, selected)}"
  style:left="{rect.x}px"
  style:top="{rect.y}px"
  style:width="{rect.width}px"
  style:height="{rect.height}px"
  role="button"
  tabindex="-1"
  onclick={() => onSelect(card.PID)}
  ondblclick={() => onFocus(card.PID)}
>
  <header>
    <span class="name">{card.Name}</span>
    <span class="label">{card.Label}</span>
    <span class="since">{card.Since}</span>
    <span class="cwd">{card.Cwd}</span>
  </header>
  {#if card.InTmux}
    <div class="activity">{card.Activity}</div>
    <div class="body" bind:clientWidth={bodyWidth} bind:clientHeight={bodyHeight}>
      <div class="term" bind:this={host}></div>
      {#if unavailable}<span class="stale">écran indisponible</span>{/if}
    </div>
  {:else}
    <div class="body plain">
      <pre>{card.Activity}</pre>
      <p>hors tmux : lecture seule</p>
    </div>
  {/if}
</div>

<style>
  .card {
    --state: var(--unknown);
    position: absolute;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    background: var(--bg);
    outline: 1px solid var(--state);
    outline-offset: -1px;
    cursor: default;
  }

  .card.selected {
    outline-width: 3px;
    outline-offset: -3px;
  }

  .frame-unknown {
    outline-style: dashed;
  }

  .frame-waiting {
    --state: var(--waiting);
  }
  .frame-busy {
    --state: var(--busy);
  }
  .frame-background {
    --state: var(--background);
  }
  .frame-idle {
    --state: var(--idle);
  }

  header {
    flex: none;
    display: flex;
    gap: 10px;
    padding: 6px 8px 0;
    white-space: nowrap;
    overflow: hidden;
  }

  .name {
    font-weight: bold;
  }

  .label {
    color: var(--state);
  }

  .since,
  .cwd {
    color: var(--muted);
  }

  .cwd {
    margin-left: auto;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .activity {
    flex: none;
    padding: 0 8px 4px;
    color: var(--muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .body {
    position: relative;
    flex: 1;
    min-height: 0;
    margin: 0 8px 6px;
    overflow: hidden;
  }

  .term {
    pointer-events: none;
    user-select: none;
  }

  .stale {
    position: absolute;
    right: 0;
    bottom: 0;
    padding: 0 4px;
    color: var(--warn);
    background: var(--panel);
  }

  .plain pre {
    margin: 0;
    white-space: pre-wrap;
    overflow: hidden;
    color: var(--muted);
  }

  .plain p {
    margin: 8px 0 0;
    color: var(--warn);
  }
</style>
