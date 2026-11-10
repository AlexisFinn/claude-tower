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
  constructor(private readonly ctx: CanvasRenderingContext2D) {}
  getContext(): CanvasRenderingContext2D {
    return this.ctx
  }
}

const ratio = { width: 0.6, height: 1.2 }
const original = globalThis.document

let calls: Call[]
let canvas: FakeCanvas
let term: Terminal

beforeEach(() => {
  calls = []
  const ctx = fakeContext([])
  ;(globalThis as { document?: unknown }).document = { createElement: () => ({ getContext: () => ctx }) }
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
    expect(canvas.style.width).toBe(`${g.cols * g.cellWidth}px`)
    expect(canvas.style.height).toBe(`${g.rows * g.cellHeight}px`)
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

  it('outlines the text glyphs in their color, but neither the box drawing nor the wide glyphs', async () => {
    await show(['\x1b[38;2;32;32;32mab\x1b[0m┌─世'], 1)
    const strokes: { ch: unknown; color: unknown; width: unknown }[] = []
    const ctx = fakeContext(calls)
    const record = new Proxy(ctx, {
      get(t, key: string) {
        if (key === 'strokeText') return (ch: unknown) => strokes.push({ ch, color: t.strokeStyle, width: t.lineWidth })
        return (t as unknown as Record<string, unknown>)[key]
      },
      set(t, key: string, value) {
        ;(t as unknown as Record<string, unknown>)[key] = value
        return true
      },
    })
    const g = geometry(1)
    draw(createThumbnail(new FakeCanvas(record) as unknown as HTMLCanvasElement), g)
    expect(strokes.map((s) => s.ch)).toEqual(['a', 'b'])
    expect(strokes.every((s) => s.color === '#585858' && typeof s.width === 'number' && s.width > 0)).toBe(true)
    expect(calls.filter((c) => c.name === 'fillText').map((c) => [c.args[0], c.fillStyle])).toEqual([
      ['a', '#585858'],
      ['b', '#585858'],
      ['世', '#e7ecf3'],
    ])
  })
})
