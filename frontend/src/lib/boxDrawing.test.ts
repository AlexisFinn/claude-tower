import { describe, expect, it } from 'vitest'
import { glyphShapes, isBoxGlyph, type RectShape, type Shape } from './boxDrawing'

const W = 10
const H = 20

function rects(shapes: Shape[]): RectShape[] {
  return shapes.filter((s): s is RectShape => s.kind === 'rect')
}

function coverage(shapes: Shape[], w = W, h = H): boolean[][] {
  const grid = Array.from({ length: h }, () => Array<boolean>(w).fill(false))
  for (const r of rects(shapes)) {
    for (let y = r.y; y < r.y + r.h; y++) for (let x = r.x; x < r.x + r.w; x++) grid[y][x] = true
  }
  return grid
}

describe('isBoxGlyph', () => {
  it('covers U+2500 to U+259F', () => {
    expect(isBoxGlyph(0x24ff)).toBe(false)
    expect(isBoxGlyph(0x2500)).toBe(true)
    expect(isBoxGlyph(0x259f)).toBe(true)
    expect(isBoxGlyph(0x25a0)).toBe(false)
    expect(isBoxGlyph(0x41)).toBe(false)
  })
})

describe('glyphShapes', () => {
  it('returns nothing outside the range', () => {
    expect(glyphShapes(0x41, W, H)).toEqual([])
  })

  it('draws a light horizontal line across the whole cell width', () => {
    const grid = coverage(glyphShapes(0x2500, W, H))
    const rows = grid.map((r, y) => (r.every(Boolean) ? y : -1)).filter((y) => y >= 0)
    expect(rows).toHaveLength(1)
    expect(grid.flat().filter(Boolean)).toHaveLength(W)
  })

  it('draws a vertical line across the whole cell height at the same column in every cell', () => {
    const a = coverage(glyphShapes(0x2502, W, H))
    const col = a[0].findIndex(Boolean)
    expect(a.every((row) => row[col])).toBe(true)
    expect(a[0].filter(Boolean)).toHaveLength(1)
  })

  it('draws a heavy line twice as thick as a light one', () => {
    const light = coverage(glyphShapes(0x2500, W, 32)).flat().filter(Boolean).length
    const heavy = coverage(glyphShapes(0x2501, W, 32)).flat().filter(Boolean).length
    expect(heavy).toBe(2 * light)
  })

  it('draws a double line as two bands with a gap', () => {
    const grid = coverage(glyphShapes(0x2550, W, H))
    const filled = grid.map((r) => r.every(Boolean))
    const first = filled.indexOf(true)
    const last = filled.lastIndexOf(true)
    expect(filled.filter(Boolean)).toHaveLength(2)
    expect(last - first).toBe(2)
  })

  it('joins the arms of a corner at the center', () => {
    const grid = coverage(glyphShapes(0x250c, W, H))
    expect(grid.some((r) => r[W - 1])).toBe(true)
    expect(grid[H - 1].some(Boolean)).toBe(true)
    expect(grid[0].some(Boolean)).toBe(false)
    expect(grid.every((r) => !r[0])).toBe(true)
    const joined = grid.findIndex((r) => r[W - 1])
    const column = grid[H - 1].findIndex(Boolean)
    expect(grid[joined][column]).toBe(true)
  })

  it('draws a cross that touches the four edges', () => {
    const grid = coverage(glyphShapes(0x253c, W, H))
    expect(grid[0].some(Boolean)).toBe(true)
    expect(grid[H - 1].some(Boolean)).toBe(true)
    expect(grid.some((r) => r[0])).toBe(true)
    expect(grid.some((r) => r[W - 1])).toBe(true)
  })

  it('draws diagonals as lines', () => {
    expect(glyphShapes(0x2571, W, H)).toHaveLength(1)
    expect(glyphShapes(0x2572, W, H)).toHaveLength(1)
    expect(glyphShapes(0x2573, W, H)).toHaveLength(2)
    expect(glyphShapes(0x2571, W, H)[0].kind).toBe('line')
  })

  it('fills the cell with the full block', () => {
    expect(coverage(glyphShapes(0x2588, W, H)).flat().every(Boolean)).toBe(true)
  })

  it('fills the bottom or left fraction of the cell', () => {
    const lower = coverage(glyphShapes(0x2584, W, H))
    expect(lower.slice(0, H / 2).flat().some(Boolean)).toBe(false)
    expect(lower.slice(H / 2).flat().every(Boolean)).toBe(true)
    const left = coverage(glyphShapes(0x258c, W, H))
    expect(left.every((r) => r.slice(0, W / 2).every(Boolean) && !r.slice(W / 2).some(Boolean))).toBe(true)
  })

  it('draws shades as a translucent full cell', () => {
    for (const [code, alpha] of [
      [0x2591, 0.25],
      [0x2592, 0.5],
      [0x2593, 0.75],
    ] as const) {
      expect(glyphShapes(code, W, H)).toEqual([{ kind: 'rect', x: 0, y: 0, w: W, h: H, alpha }])
    }
  })

  it('draws quadrants without overlap', () => {
    const all = coverage([...glyphShapes(0x2598, W, H), ...glyphShapes(0x259d, W, H), ...glyphShapes(0x2596, W, H), ...glyphShapes(0x2597, W, H)])
    expect(all.flat().every(Boolean)).toBe(true)
    expect(coverage(glyphShapes(0x259a, W, H)).flat().filter(Boolean)).toHaveLength(W * H / 2)
  })

  it('keeps every shape inside the cell', () => {
    for (let code = 0x2500; code <= 0x259f; code++) {
      for (const r of rects(glyphShapes(code, 7, 15))) {
        expect(r.x).toBeGreaterThanOrEqual(0)
        expect(r.y).toBeGreaterThanOrEqual(0)
        expect(r.x + r.w).toBeLessThanOrEqual(7)
        expect(r.y + r.h).toBeLessThanOrEqual(15)
      }
    }
  })

  it('draws something for every character of the range', () => {
    for (let code = 0x2500; code <= 0x259f; code++) {
      expect(glyphShapes(code, W, H).length, code.toString(16)).toBeGreaterThan(0)
    }
  })
})
