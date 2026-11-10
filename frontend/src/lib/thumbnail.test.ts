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

interface Put {
  width: number
  height: number
  y: number
}

function image(width: number, height: number): ImageData {
  return { width, height, data: new Uint8ClampedArray(width * height * 4) } as unknown as ImageData
}

/** The canvas a thumbnail paints its rows into, three times wider: its calls are recorded. */
function scratchContext(calls: Call[]): CanvasRenderingContext2D {
  const ctx = fakeContext(calls) as unknown as Record<string, unknown>
  ctx.canvas = { width: 0, height: 0 }
  ctx.getImageData = (x: number, y: number, w: number, h: number) => {
    calls.push({ name: 'getImageData', args: [x, y, w, h], fillStyle: '' })
    return image(w, h)
  }
  return ctx as unknown as CanvasRenderingContext2D
}

/** The displayed canvas, which only receives folded rows. */
function displayContext(puts: Put[]): CanvasRenderingContext2D {
  const ctx = fakeContext([]) as unknown as Record<string, unknown>
  ctx.createImageData = image
  ctx.putImageData = (img: ImageData, _x: number, y: number) => puts.push({ width: img.width, height: img.height, y })
  return ctx as unknown as CanvasRenderingContext2D
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
let puts: Put[]
let scratch: CanvasRenderingContext2D
let canvas: FakeCanvas
let term: Terminal

beforeEach(() => {
  calls = []
  puts = []
  scratch = scratchContext(calls)
  ;(globalThis as { document?: unknown }).document = { createElement: () => ({ getContext: () => scratch }) }
  canvas = new FakeCanvas(displayContext(puts))
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
  puts.length = 0
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
    expect(puts).toEqual([])
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

  it('paints rows three times wider and folds each onto the displayed canvas', async () => {
    await show(['hello', 'world'], 2)
    const g = geometry(2)
    draw(createThumbnail(canvas as unknown as HTMLCanvasElement), g)
    expect(canvas.width).toBe(Math.round(g.cols * g.cellWidth))
    expect(puts.map((p) => p.width)).toEqual([canvas.width, canvas.width])
    expect(puts.map((p) => p.y)).toEqual([0, puts[0].height])
    expect(scratch.canvas.width).toBe(3 * canvas.width)
    expect(calls.filter((c) => c.name === 'setTransform').map((c) => c.args)).toEqual([
      [3, 0, 0, 1, 0, -0],
      [1, 0, 0, 1, 0, 0],
      [3, 0, 0, 1, 0, -puts[0].height],
      [1, 0, 0, 1, 0, 0],
    ])
    expect(calls.filter((c) => c.name === 'getImageData').map((c) => c.args)).toEqual([
      [0, 0, 3 * canvas.width, puts[0].height],
      [0, 0, 3 * canvas.width, puts[1].height],
    ])
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
