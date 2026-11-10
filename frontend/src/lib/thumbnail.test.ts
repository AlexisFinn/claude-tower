import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { Terminal } from '@xterm/xterm'
import { bodyGeometry, type BodyGeometry } from './bodySize'
import { screenSequence } from './screenSequence'
import { createThumbnail } from './thumbnail'
import { DEFAULT_BG } from './termPalette'

interface Call {
  name: string
  args: unknown[]
  fillStyle: string
}

function fakeContext(calls: Call[]): CanvasRenderingContext2D {
  const state = { fillStyle: '' }
  const target: Record<string, unknown> = {
    measureText: () => ({ width: 60, fontBoundingBoxAscent: 93, fontBoundingBoxDescent: 24 }),
  }
  return new Proxy(target, {
    get(t, key: string) {
      if (key in t) return t[key]
      if (key === 'fillStyle') return state.fillStyle
      return (...args: unknown[]) => calls.push({ name: key, args, fillStyle: state.fillStyle })
    },
    set(t, key: string, value) {
      if (key === 'fillStyle') state.fillStyle = value
      else t[key] = value
      return true
    },
  }) as unknown as CanvasRenderingContext2D
}

class FakeCanvas {
  width = 0
  height = 0
  style: Record<string, string> = {}
  /** Position of the canvas before its own transform, in CSS pixels. */
  left = 0
  top = 0
  constructor(private readonly ctx: CanvasRenderingContext2D) {}
  getContext(): CanvasRenderingContext2D {
    return this.ctx
  }
  getBoundingClientRect(): { left: number; top: number } {
    const m = /translate\(([-\d.e]+)px, ([-\d.e]+)px\)/.exec(this.style.transform ?? '')
    return { left: this.left + (m ? Number(m[1]) : 0), top: this.top + (m ? Number(m[2]) : 0) }
  }
}

const ratio = { width: 0.6, height: 1.2 }
const original = globalThis.document

let calls: Call[]
let canvas: FakeCanvas
let term: Terminal

beforeEach(() => {
  calls = []
  const measure = fakeContext([])
  ;(globalThis as { document?: unknown }).document = { createElement: () => ({ getContext: () => measure }) }
  canvas = new FakeCanvas(fakeContext(calls))
  term = new Terminal({ cols: 20, rows: 5, scrollback: 0, allowProposedApi: true })
})

afterEach(() => {
  term.dispose()
  ;(globalThis as { document?: unknown }).document = original
})

function show(lines: string[], rows: number): Promise<void> {
  term.resize(20, rows)
  return new Promise((resolve) => term.write(screenSequence(lines, rows), resolve))
}

function draw(thumbnail: ReturnType<typeof createThumbnail>, g: BodyGeometry, dpr = 1): void {
  calls.length = 0
  thumbnail.draw(term.buffer.active, g, dpr)
}

function paintedRows(): number[] {
  const width = canvas.width
  return calls
    .filter((c) => c.name === 'fillRect' && c.fillStyle === DEFAULT_BG && c.args[0] === 0 && c.args[2] === width)
    .map((c) => c.args[1] as number)
}

function geometry(rows = 5, bodyWidth = 200): BodyGeometry {
  return bodyGeometry(20, rows, bodyWidth, rows * 30, ratio)
}

describe('createThumbnail', () => {
  const lines = ['\x1b[31mhello\x1b[0m', '┌──┐', '│世│', '└──┘', 'last']

  it('paints every row on the first draw and sizes the canvas', async () => {
    await show(lines, 5)
    const g = geometry()
    draw(createThumbnail(canvas as unknown as HTMLCanvasElement), g)
    expect(paintedRows()).toHaveLength(5)
    expect(calls.some((c) => c.name === 'fillText')).toBe(true)
    expect(canvas.width).toBeGreaterThan(0)
    expect(canvas.height).toBeGreaterThan(0)
    expect(canvas.style.width).toBe(`${canvas.width}px`)
    expect(canvas.style.height).toBe(`${canvas.height}px`)
  })

  it('clips every row to its own band', async () => {
    await show(lines, 5)
    draw(createThumbnail(canvas as unknown as HTMLCanvasElement), geometry())
    const names = calls.map((c) => c.name)
    expect(names.filter((n) => n === 'clip')).toHaveLength(5)
    expect(names.filter((n) => n === 'save')).toHaveLength(5)
    expect(names.filter((n) => n === 'restore')).toHaveLength(5)
    expect(names.indexOf('clip')).toBeLessThan(names.indexOf('fillRect'))
  })

  it('does not draw anything when nothing changed', async () => {
    await show(lines, 5)
    const thumbnail = createThumbnail(canvas as unknown as HTMLCanvasElement)
    draw(thumbnail, geometry())
    draw(thumbnail, geometry())
    expect(calls).toEqual([])
  })

  it('repaints only the row that changed', async () => {
    await show(lines, 5)
    const thumbnail = createThumbnail(canvas as unknown as HTMLCanvasElement)
    const g = geometry()
    draw(thumbnail, g)
    const all = paintedRows()
    await show([...lines.slice(0, 3), '└──┘x', 'last'], 5)
    draw(thumbnail, g)
    expect(paintedRows()).toEqual([all[3]])
  })

  it('repaints everything when the geometry changes', async () => {
    await show(lines, 5)
    const thumbnail = createThumbnail(canvas as unknown as HTMLCanvasElement)
    draw(thumbnail, geometry(5, 100))
    draw(thumbnail, geometry(5, 200))
    expect(paintedRows()).toHaveLength(5)
  })

  it('draws box glyphs with the cell size of the current geometry after it changes', async () => {
    await show(lines, 5)
    const rects = (): unknown[] => calls.filter((c) => c.name === 'fillRect').map((c) => c.args)
    const thumbnail = createThumbnail(canvas as unknown as HTMLCanvasElement)
    draw(thumbnail, geometry(5, 100))
    draw(thumbnail, geometry(5, 200))
    const changed = rects()
    draw(createThumbnail(canvas as unknown as HTMLCanvasElement), geometry(5, 200))
    expect(changed).toEqual(rects())
  })

  it('repaints everything when the font changes without a geometry size change', async () => {
    await show(lines, 5)
    const thumbnail = createThumbnail(canvas as unknown as HTMLCanvasElement)
    const g = geometry()
    draw(thumbnail, g)
    draw(thumbnail, { ...g, font: g.font + 1 })
    expect(paintedRows()).toHaveLength(5)
  })

  it('repaints everything when the device pixel ratio changes', async () => {
    await show(lines, 5)
    const thumbnail = createThumbnail(canvas as unknown as HTMLCanvasElement)
    const g = geometry()
    draw(thumbnail, g, 1)
    const width = canvas.width
    draw(thumbnail, g, 2)
    expect(paintedRows()).toHaveLength(5)
    expect(canvas.width).toBeGreaterThan(width)
  })

  it('resizes and repaints the whole canvas when the row count decreases', async () => {
    await show(lines, 5)
    const thumbnail = createThumbnail(canvas as unknown as HTMLCanvasElement)
    draw(thumbnail, geometry(5))
    const height = canvas.height
    await show(lines, 3)
    draw(thumbnail, geometry(3))
    expect(canvas.height).toBeLessThan(height)
    expect(paintedRows()).toHaveLength(3)
  })

  it('sets the text color again after a box drawing glyph', async () => {
    await show(['\x1b[38;2;32;32;32ma┌b\x1b[0m'], 1)
    draw(createThumbnail(canvas as unknown as HTMLCanvasElement), geometry(1))
    expect(calls.filter((c) => c.name === 'fillText').map((c) => [c.args[0], c.fillStyle])).toEqual([
      ['a', '#202020'],
      ['b', '#202020'],
    ])
  })

  it('underlines in the text color', async () => {
    await show(['\x1b[4;38;2;32;32;32mab\x1b[0m'], 1)
    const g = geometry(1)
    draw(createThumbnail(canvas as unknown as HTMLCanvasElement), g)
    const underlines = calls.filter((c) => c.name === 'fillRect' && (c.args[3] as number) < g.cellHeight / 2)
    expect(underlines).toHaveLength(2)
    expect(underlines.every((c) => c.fillStyle === '#202020')).toBe(true)
  })

  it('draws at the display resolution', async () => {
    await show(['hello', 'world'], 2)
    const g = geometry(2)
    draw(createThumbnail(canvas as unknown as HTMLCanvasElement), g, 2)
    expect(canvas.width).toBe(Math.round(g.cols * g.cellWidth * 2))
    expect(canvas.style.width).toBe(`${canvas.width / 2}px`)
    expect(paintedRows()).toHaveLength(2)
  })

  it('draws one backing pixel per device pixel below a ratio of 1 too', async () => {
    await show(['hello'], 1)
    const g = geometry(1)
    draw(createThumbnail(canvas as unknown as HTMLCanvasElement), g, 0.5)
    expect(canvas.width).toBe(Math.round(g.cols * g.cellWidth * 0.5))
    expect(canvas.style.width).toBe(`${canvas.width / 0.5}px`)
  })

  it('shifts the canvas onto whole device pixels when it is drawn and when it moves', async () => {
    await show(['hello'], 1)
    canvas.left = 10.3
    canvas.top = 4.8
    const thumbnail = createThumbnail(canvas as unknown as HTMLCanvasElement)
    draw(thumbnail, geometry(1), 2)
    // 10.3 CSS px is 20.6 device px, moved to 21; 4.8 is 9.6, moved to 10.
    const [x, y] = /translate\(([-\d.e]+)px, ([-\d.e]+)px\)/.exec(canvas.style.transform)!.slice(1).map(Number)
    expect(x).toBeCloseTo(0.2, 6)
    expect(y).toBeCloseTo(0.2, 6)
    const shifted = canvas.style.transform
    thumbnail.align(2)
    thumbnail.align(2)
    expect(canvas.style.transform).toBe(shifted)
    canvas.left = 3.1
    canvas.top = 0
    thumbnail.align(1.5)
    // 3.1 CSS px is 4.65 device px, moved to 5.
    expect(canvas.getBoundingClientRect().left * 1.5).toBeCloseTo(5, 6)
    canvas.left = 20
    canvas.top = 5
    thumbnail.align(2)
    expect(canvas.style.transform).toBe('')
    canvas.left = 7.1
    thumbnail.align(1)
    expect(canvas.getBoundingClientRect().left).toBeCloseTo(7)
  })
})
