import type { IBufferCell } from '@xterm/xterm'
import { FALLBACK_CELL_RATIO, type BodyGeometry, type CellSize } from './bodySize'
import { glyphShapes, isBoxGlyph, type Shape } from './boxDrawing'
import { changedRows } from './rowDiff'
import { readRow, type BufferLike, type DrawCell, type Row } from './screenCells'
import { DEFAULT_BG } from './termPalette'
import { outlines, strokeWidth, textColor } from './thumbnailInk'
import { foldSubpixels, SUBPIXELS } from './subpixel'
import { canvasScale, cellBounds } from './thumbnailLayout'

/** Font stack of the thumbnails. */
export const THUMBNAIL_FONT_FAMILY = 'ui-monospace, "SFMono-Regular", Menlo, "DejaVu Sans Mono", "Liberation Mono", monospace'

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
  family: string
  metrics: FontMetrics
}

function paintRow(ctx: CanvasRenderingContext2D, row: Row, y: number, px: Pixels): void {
  const [y0, y1] = cellBounds(y, px.cellHeight)
  const height = y1 - y0
  ctx.save()
  ctx.beginPath()
  ctx.rect(0, y0, px.width, height)
  ctx.clip()
  ctx.fillStyle = DEFAULT_BG
  ctx.fillRect(0, y0, px.width, height)
  const right = (c: DrawCell): number => cellBounds(c.wide ? c.col + 1 : c.col, px.cellWidth)[1]
  for (const c of row.cells) {
    if (c.bg === null) continue
    const x0 = cellBounds(c.col, px.cellWidth)[0]
    ctx.fillStyle = c.bg
    ctx.fillRect(x0, y0, right(c) - x0, height)
  }
  const { ascent, descent } = px.metrics
  const baseline = y0 + Math.round((height - (ascent + descent) * px.font) / 2 + ascent * px.font)
  const underline = Math.max(1, Math.round(px.font / 14))
  const stroke = strokeWidth(px.font)
  ctx.lineJoin = 'round'
  let font = ''
  let fill = ''
  let outline = ''
  for (const c of row.cells) {
    const x0 = cellBounds(c.col, px.cellWidth)[0]
    const code = c.ch === '' ? 0 : (c.ch.codePointAt(0) ?? 0)
    const color = textColor(c.fg, c.bg)
    if (c.ch !== '' && c.ch.length === 1 && isBoxGlyph(code)) {
      // Box drawing keeps its own color: frames are drawn as solid shapes, which stay visible.
      paintShapes(ctx, glyphShapes(code, right(c) - x0, height), x0, y0, c.fg)
      fill = ''
      outline = ''
    } else if (c.ch !== '') {
      const next = `${c.italic ? 'italic ' : ''}${c.bold ? 'bold ' : ''}${px.font}px ${px.family}`
      if (next !== font) {
        ctx.font = next
        font = next
      }
      if (color !== fill) {
        ctx.fillStyle = color
        fill = color
      }
      ctx.fillText(c.ch, x0, baseline)
      if (outlines(c.ch, c.wide)) {
        if (color !== outline) {
          ctx.strokeStyle = color
          ctx.lineWidth = stroke
          outline = color
        }
        ctx.strokeText(c.ch, x0, baseline)
      }
    }
    if (c.underline) {
      ctx.fillStyle = color
      fill = color
      ctx.fillRect(x0, Math.min(y1 - underline, baseline + underline), right(c) - x0, underline)
    }
  }
  ctx.restore()
}

/** Shifts a canvas by less than a device pixel so that its top left corner falls on a whole device pixel. */
function snapToDevicePixels(canvas: HTMLCanvasElement, dpr: number): void {
  if (typeof canvas.getBoundingClientRect !== 'function') return
  canvas.style.transform = ''
  const r = canvas.getBoundingClientRect()
  const dx = (Math.round(r.left * dpr) - r.left * dpr) / dpr
  const dy = (Math.round(r.top * dpr) - r.top * dpr) / dpr
  if (dx !== 0 || dy !== 0) canvas.style.transform = `translate(${dx}px, ${dy}px)`
}

/** Draws a terminal buffer into a canvas, redrawing only the rows that changed. */
export interface Thumbnail {
  /**
   * Draws the first rows of the buffer for a geometry. The canvas is displayed
   * at the geometry size and its backing store is oversampled, so the browser
   * downscales an image. With subpixel rendering, the backing store has the
   * display resolution instead, and the canvas is shifted onto whole device
   * pixels, which the color stripes need. Everything is redrawn when the
   * geometry, the font, the device pixel ratio or the rendering changed.
   */
  draw(buffer: BufferLike, geometry: BodyGeometry, dpr: number, subpixel?: boolean): void
}

/** Creates a thumbnail that draws into a canvas with a font family. */
export function createThumbnail(canvas: HTMLCanvasElement, family: string = THUMBNAIL_FONT_FAMILY): Thumbnail {
  const ctx = canvas.getContext('2d', { alpha: false })
  const metrics = measureFont(family)
  let layoutKey = ''
  let signatures: string[] = []
  let cell: IBufferCell | undefined
  let wide: CanvasRenderingContext2D | null = null

  /** Paints a row three times wider into a scratch canvas, then folds it onto the color stripes of the canvas. */
  function paintSubpixelRow(row: Row, y: number, px: Pixels): void {
    if (!ctx) return
    const [y0, y1] = cellBounds(y, px.cellHeight)
    const height = y1 - y0
    if (height <= 0) return
    const width = px.width * SUBPIXELS
    wide ??= document.createElement('canvas').getContext('2d', { alpha: false, willReadFrequently: true })
    if (!wide) return
    if (wide.canvas.width < width || wide.canvas.height < height) {
      wide.canvas.width = Math.max(wide.canvas.width, width)
      wide.canvas.height = Math.max(wide.canvas.height, height)
    }
    wide.setTransform(SUBPIXELS, 0, 0, 1, 0, -y0)
    wide.textBaseline = 'alphabetic'
    wide.textAlign = 'left'
    paintRow(wide, row, y, px)
    wide.setTransform(1, 0, 0, 1, 0, 0)
    const src = wide.getImageData(0, 0, width, height)
    const out = ctx.createImageData(px.width, height)
    foldSubpixels(src.data, out.data, px.width, height)
    ctx.putImageData(out, 0, y0)
  }

  return {
    draw(buffer, g, dpr, subpixel = false) {
      if (!ctx) return
      const display = Math.max(1, Number.isFinite(dpr) ? dpr : 1)
      const scale = subpixel ? display : canvasScale(dpr, g.cols * g.cellWidth, g.rows * g.cellHeight)
      const px: Pixels = {
        width: Math.max(1, Math.round(g.cols * g.cellWidth * scale)),
        cellWidth: g.cellWidth * scale,
        cellHeight: g.cellHeight * scale,
        font: g.font * scale,
        family,
        metrics,
      }
      const height = Math.max(1, Math.round(g.rows * px.cellHeight))
      const key = `${px.width}x${height}|${g.cols}x${g.rows}|${px.font}|${subpixel}`
      if (key !== layoutKey) {
        layoutKey = key
        canvas.width = px.width
        canvas.height = height
        // With subpixel rendering, each backing pixel is displayed on exactly one
        // device pixel, so the browser never resamples the color stripes.
        canvas.style.width = subpixel ? `${px.width / display}px` : `${g.cols * g.cellWidth}px`
        canvas.style.height = subpixel ? `${height / display}px` : `${g.rows * g.cellHeight}px`
        signatures = []
      }
      if (subpixel) snapToDevicePixels(canvas, display)
      else canvas.style.transform = ''
      cell ??= buffer.getNullCell()
      ctx.textBaseline = 'alphabetic'
      ctx.textAlign = 'left'
      const rows: Row[] = []
      for (let y = 0; y < g.rows; y++) rows.push(readRow(buffer.getLine(y), g.cols, cell))
      const next = rows.map((r) => r.signature)
      for (const y of changedRows(signatures, next)) {
        if (subpixel) paintSubpixelRow(rows[y], y, px)
        else paintRow(ctx, rows[y], y, px)
      }
      signatures = next
    },
  }
}
