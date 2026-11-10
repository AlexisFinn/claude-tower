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
  import { bodyGeometry, fitGrid, sameGeometry, type BodyGeometry, type GridSize } from './lib/bodySize'
  import { watchDevicePixelRatio } from './lib/devicePixelRatio'
  import { onPageScroll } from './lib/pageScroll'
  import { screenSequence } from './lib/screenSequence'
  import { createThumbnail, type Thumbnail } from './lib/thumbnail'

  interface Props {
    pid: number
    screen: core.Screen | undefined
    /** Where the card is: a change realigns the canvas on device pixels. */
    placement: string
    /** Whether the session is one the application may resize. */
    resizable: boolean
    onMeasure: (pid: number, size: GridSize | null) => void
    /** Receives the terminal size that fills the body at a legible font, for a session the application may resize. */
    onFit: (pid: number, size: GridSize | null) => void
  }

  let { pid, screen, placement, resizable, onMeasure, onFit }: Props = $props()

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

  const paneCols = $derived(screen && screen.Width > 0 ? screen.Width : null)
  const paneRows = $derived(screen && screen.Height > 0 ? screen.Height : null)

  $effect(() => {
    if (!canvas) return
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
    const stopWatching = watchDevicePixelRatio((ratio) => (dpr = ratio))
    return () => {
      stopWatching()
      onFit(pid, null)
    }
  })

  // The geometry only depends on the body and pane sizes, and the canvas is
  // taken out of the flow, so drawing never changes the body size again.
  $effect(() => {
    if (bodyWidth <= 0 || bodyHeight <= 0) return
    const next = bodyGeometry(paneCols, paneRows, bodyWidth, bodyHeight, cellRatio)
    if (!sameGeometry(untrack(() => geometry), next)) geometry = next
    onMeasure(pid, { cols: next.cols, rows: next.rows })
  })

  $effect(() => {
    if (!resizable || bodyWidth <= 0 || bodyHeight <= 0) {
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
    const stopScrollWatch = onPageScroll(scheduleAlign)
    return () => {
      stopScrollWatch()
      cancelAnimationFrame(alignFrame)
    }
  })
</script>

<div class="fill" bind:clientWidth={bodyWidth} bind:clientHeight={bodyHeight}>
  <canvas bind:this={canvas} class:ready={geometry !== null}></canvas>
</div>

<style>
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
</style>
