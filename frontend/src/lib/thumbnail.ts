import type { IBufferCell } from '@xterm/xterm'
import { FALLBACK_CELL_RATIO, type BodyGeometry, type CellSize } from './bodySize'
import { createGlyphCache, isBoxGlyph, type GlyphCache, type Shape } from './boxDrawing'
import { changedRows } from './rowDiff'
import { readRow, type BufferLike, type DrawCell, type Row } from './screenCells'
import { DEFAULT_BG } from './termPalette'
import { cellBounds } from './thumbnailLayout'

/** Font stack of the thumbnails: the bundled Source Code Pro, then the usual monospace fonts. */
export const THUMBNAIL_FONT_FAMILY = '"Tower Mono", ui-monospace, "DejaVu Sans Mono", "Liberation Mono", monospace'

/**
 * Loads the faces of the bundled font that the thumbnails draw with: a canvas
 * draws with a fallback font, and measureFont caches its metrics, until they
 * are loaded. A face that fails to load leaves the fallback fonts.
 */
export async function loadThumbnailFont(): Promise<void> {
  const faces = ['12px', 'bold 12px', 'italic 12px', 'italic bold 12px'].map((f) => `${f} "Tower Mono"`)
  if (typeof document === 'undefined' || !document.fonts) return
  await Promise.allSettled(faces.map((f) => document.fonts.load(f)))
}

const FALLBACK_ASCENT = 0.93
const FALLBACK_DESCENT = 0.24
const REFERENCE_SIZE = 100

/** Cell size and vertical font metrics, per pixel of font size. */
export interface FontMetrics {
  cell: CellSize
  ascent: number
  descent: number
}

const metricsCache = new Map<string, FontMetrics>()

/** Measures the cell size and the vertical metrics of a monospace font family, once per family. */
export function measureFont(family: string): FontMetrics {
  const known = metricsCache.get(family)
  if (known) return known
  const fallback: FontMetrics = { cell: FALLBACK_CELL_RATIO, ascent: FALLBACK_ASCENT, descent: FALLBACK_DESCENT }
  const ctx = document.createElement('canvas').getContext('2d')
  let metrics = fallback
  if (ctx) {
    ctx.font = `${REFERENCE_SIZE}px ${family}`
    const m = ctx.measureText('M')
    if (m.width > 0) {
      metrics = {
        cell: { width: m.width / REFERENCE_SIZE, height: FALLBACK_CELL_RATIO.height },
        ascent: m.fontBoundingBoxAscent > 0 ? m.fontBoundingBoxAscent / REFERENCE_SIZE : FALLBACK_ASCENT,
        descent: m.fontBoundingBoxDescent > 0 ? m.fontBoundingBoxDescent / REFERENCE_SIZE : FALLBACK_DESCENT,
      }
    }
  }
  metricsCache.set(family, metrics)
  return metrics
}

function paintShapes(ctx: CanvasRenderingContext2D, shapes: Shape[], x: number, y: number, color: string): void {
  ctx.fillStyle = color
  ctx.strokeStyle = color
  for (const s of shapes) {
    if (s.kind === 'rect') {
      if (s.alpha !== 1) ctx.globalAlpha = s.alpha
      ctx.fillRect(x + s.x, y + s.y, s.w, s.h)
      ctx.globalAlpha = 1
    } else {
      ctx.lineWidth = s.width
      ctx.beginPath()
      ctx.moveTo(x + s.x1, y + s.y1)
      ctx.lineTo(x + s.x2, y + s.y2)
      ctx.stroke()
    }
  }
}

/** The pixel geometry of a canvas, in backing pixels. */
interface Pixels {
  width: number
  cellWidth: number
  cellHeight: number
  font: number
  /** Canvas font strings by style: bit 0 is bold, bit 1 is italic. */
  fonts: string[]
  glyphs: GlyphCache
  metrics: FontMetrics
}

function fontVariants(size: number, family: string): string[] {
  return ['', 'bold ', 'italic ', 'italic bold '].map((style) => `${style}${size}px ${family}`)
}

function paintRow(ctx: CanvasRenderingContext2D, row: Row, y: number, px: Pixels): void {
  const right = (c: DrawCell): number => cellBounds(c.wide ? c.col + 1 : c.col, px.cellWidth)[1]
  const [y0, y1] = cellBounds(y, px.cellHeight)
  const height = y1 - y0
  ctx.save()
  ctx.beginPath()
  ctx.rect(0, y0, px.width, height)
  ctx.clip()
  ctx.fillStyle = DEFAULT_BG
  ctx.fillRect(0, y0, px.width, height)
  for (const c of row.cells) {
    if (c.bg === null) continue
    const x0 = cellBounds(c.col, px.cellWidth)[0]
    ctx.fillStyle = c.bg
    ctx.fillRect(x0, y0, right(c) - x0, height)
  }
  const { ascent, descent } = px.metrics
  const baseline = y0 + Math.round((height - (ascent + descent) * px.font) / 2 + ascent * px.font)
  const underline = Math.max(1, Math.round(px.font / 14))
  let font = ''
  let fill = ''
  for (const c of row.cells) {
    const x0 = cellBounds(c.col, px.cellWidth)[0]
    const code = c.ch === '' ? 0 : (c.ch.codePointAt(0) ?? 0)
    if (c.ch !== '' && c.ch.length === 1 && isBoxGlyph(code)) {
      paintShapes(ctx, px.glyphs(code, right(c) - x0, height), x0, y0, c.fg)
      fill = ''
    } else if (c.ch !== '') {
      const next = px.fonts[(c.bold ? 1 : 0) | (c.italic ? 2 : 0)]
      if (next !== font) {
        ctx.font = next
        font = next
      }
      if (c.fg !== fill) {
        ctx.fillStyle = c.fg
        fill = c.fg
      }
      ctx.fillText(c.ch, x0, baseline)
    }
    if (c.underline) {
      ctx.fillStyle = c.fg
      fill = c.fg
      ctx.fillRect(x0, Math.min(y1 - underline, baseline + underline), right(c) - x0, underline)
    }
  }
  ctx.restore()
}

/** Returns the device pixels per CSS pixel to draw at: the device pixel ratio, or 1 when it is unusable. */
function displayScale(dpr: number): number {
  return Number.isFinite(dpr) && dpr > 0 ? dpr : 1
}

/** Draws a terminal buffer into a canvas, redrawing only the rows that changed. */
export interface Thumbnail {
  /**
   * Draws the first rows of the buffer for a geometry. The backing store has
   * the display resolution, so each of its pixels is displayed on one device
   * pixel and the font rasterizer smooths the text itself. Everything is redrawn, and the
   * canvas aligned, when the geometry, the font or the device pixel ratio
   * changed.
   */
  draw(buffer: BufferLike, geometry: BodyGeometry, dpr: number): void
  /**
   * Shifts the canvas by at most half a device pixel so that its top left
   * corner falls on a whole device pixel, so that the browser does not
   * resample it. To be called again whenever the canvas moves.
   */
  align(dpr: number): void
}

/** Creates a thumbnail that draws into a canvas with a font family. */
export function createThumbnail(canvas: HTMLCanvasElement, family: string = THUMBNAIL_FONT_FAMILY): Thumbnail {
  const ctx = canvas.getContext('2d', { alpha: false })
  const metrics = measureFont(family)
  let layoutKey = ''
  let signatures: string[] = []
  let cell: IBufferCell | undefined
  let shift = { x: 0, y: 0 }
  let pixels: Pixels | null = null

  /** Aligns the canvas from its current position, without clearing its shift first, so no layout is forced twice. */
  function align(dpr: number): void {
    const scale = displayScale(dpr)
    const r = canvas.getBoundingClientRect()
    const left = (r.left - shift.x) * scale
    const top = (r.top - shift.y) * scale
    const next = { x: (Math.round(left) - left) / scale, y: (Math.round(top) - top) / scale }
    if (Math.abs(next.x - shift.x) < 1e-3 && Math.abs(next.y - shift.y) < 1e-3) return
    shift = next
    canvas.style.transform = next.x === 0 && next.y === 0 ? '' : `translate(${next.x}px, ${next.y}px)`
  }

  return {
    align,
    draw(buffer, g, dpr) {
      if (!ctx) return
      const scale = displayScale(dpr)
      const width = Math.max(1, Math.round(g.cols * g.cellWidth * scale))
      const cellWidth = g.cellWidth * scale
      const cellHeight = g.cellHeight * scale
      const font = g.font * scale
      const height = Math.max(1, Math.round(g.rows * cellHeight))
      const key = `${width}x${height}|${g.cols}x${g.rows}|${cellWidth}x${cellHeight}|${font}`
      if (key !== layoutKey || pixels === null) {
        layoutKey = key
        pixels = { width, cellWidth, cellHeight, font, fonts: fontVariants(font, family), glyphs: createGlyphCache(), metrics }
        canvas.width = width
        canvas.height = height
        // Each backing pixel is displayed on exactly one device pixel, so the
        // browser never resamples the color stripes.
        canvas.style.width = `${width / scale}px`
        canvas.style.height = `${height / scale}px`
        signatures = []
        align(dpr)
      }
      cell ??= buffer.getNullCell()
      ctx.textBaseline = 'alphabetic'
      ctx.textAlign = 'left'
      const rows: Row[] = []
      for (let y = 0; y < g.rows; y++) rows.push(readRow(buffer.getLine(y), g.cols, cell))
      const next = rows.map((r) => r.signature)
      for (const y of changedRows(signatures, next)) paintRow(ctx, rows[y], y, pixels)
      signatures = next
    },
  }
}
