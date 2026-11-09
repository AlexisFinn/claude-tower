import { describe, expect, it } from 'vitest'
import { insetRect, layout, type Rect } from './layout'

const W = 1600
const H = 900

function area(rects: Rect[]): number {
  return rects.reduce((sum, r) => sum + r.width * r.height, 0)
}

function overlap(a: Rect, b: Rect): boolean {
  return a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height
}

describe('layout', () => {
  it('returns nothing for no session', () => {
    expect(layout(0, W, H).rects).toEqual([])
  })

  it('gives the whole area to one session', () => {
    expect(layout(1, W, H).rects).toEqual([{ x: 0, y: 0, width: W, height: H }])
  })

  it('puts two sessions side by side at full height', () => {
    expect(layout(2, W, H).rects).toEqual([
      { x: 0, y: 0, width: 800, height: H },
      { x: 800, y: 0, width: 800, height: H },
    ])
  })

  it('gives the last card of three the full width of the bottom row', () => {
    expect(layout(3, W, H).rects).toEqual([
      { x: 0, y: 0, width: 800, height: 450 },
      { x: 800, y: 0, width: 800, height: 450 },
      { x: 0, y: 450, width: W, height: 450 },
    ])
  })

  it('tiles four sessions as 2x2', () => {
    const rects = layout(4, W, H).rects
    expect(rects.map((r) => [r.x, r.y, r.width, r.height])).toEqual([
      [0, 0, 800, 450],
      [800, 0, 800, 450],
      [0, 450, 800, 450],
      [800, 450, 800, 450],
    ])
  })

  it('spreads the incomplete last row of five sessions', () => {
    const rects = layout(5, 1800, H).rects
    expect(rects.slice(0, 3).map((r) => r.width)).toEqual([600, 600, 600])
    expect(rects.slice(3).map((r) => [r.x, r.width])).toEqual([
      [0, 900],
      [900, 900],
    ])
  })

  it('tiles six sessions as 3x2', () => {
    const rects = layout(6, 1800, H).rects
    expect(rects.map((r) => r.width)).toEqual([600, 600, 600, 600, 600, 600])
    expect(rects.map((r) => r.height)).toEqual([450, 450, 450, 450, 450, 450])
  })

  it('covers the whole area without overlap for any count', () => {
    for (let n = 1; n <= 13; n++) {
      const { rects, scrolls } = layout(n, 2000, 1400)
      expect(scrolls).toBe(false)
      expect(rects).toHaveLength(n)
      expect(area(rects)).toBe(2000 * 1400)
      for (let i = 0; i < n; i++) {
        for (let j = i + 1; j < n; j++) expect(overlap(rects[i], rects[j])).toBe(false)
      }
    }
  })

  it('handles thirteen sessions with a single card in the last row', () => {
    const rects = layout(13, 2000, 1400).rects
    expect(rects).toHaveLength(13)
    expect(rects[12]).toEqual({ x: 0, y: 1050, width: 2000, height: 350 })
  })

  it('scrolls with 230 px rows when cards would be too small', () => {
    const result = layout(13, 1000, 600)
    expect(result.scrolls).toBe(true)
    expect(result.rects.every((r) => r.height === 230)).toBe(true)
    expect(result.rects.every((r) => r.width >= 320 || r.width === 1000)).toBe(true)
    expect(result.contentHeight).toBe(result.rects[12].y + 230)
  })

  it('uses fewer columns when the width is too small for the natural ones', () => {
    const result = layout(4, 700, 1000)
    expect(result.rects.map((r) => r.width)).toEqual([350, 350, 350, 350])
    expect(result.scrolls).toBe(false)
  })

  it('keeps the natural column count when the height forces the scroll', () => {
    const result = layout(9, 2000, 300)
    expect(result.rects.map((r) => [r.x, r.width])).toEqual([
      [0, 667],
      [667, 666],
      [1333, 667],
      [0, 667],
      [667, 666],
      [1333, 667],
      [0, 667],
      [667, 666],
      [1333, 667],
    ])
    expect(result.rects.every((r) => r.height === 230)).toBe(true)
    expect(result.contentHeight).toBe(690)
    expect(result.scrolls).toBe(true)
  })

  it('stretches the rows to the area when fewer columns leave room', () => {
    const result = layout(4, 600, 1000)
    expect(result.rects.map((r) => [r.x, r.y, r.width, r.height])).toEqual([
      [0, 0, 600, 250],
      [0, 250, 600, 250],
      [0, 500, 600, 250],
      [0, 750, 600, 250],
    ])
    expect(result.contentHeight).toBe(1000)
    expect(result.scrolls).toBe(false)
  })

  it('does not scroll at exactly 320x230 per card', () => {
    const result = layout(2, 640, 230)
    expect(result.rects).toEqual([
      { x: 0, y: 0, width: 320, height: 230 },
      { x: 320, y: 0, width: 320, height: 230 },
    ])
    expect(result.contentHeight).toBe(230)
    expect(result.scrolls).toBe(false)
  })

  it('drops a column one pixel under 320 per card', () => {
    const result = layout(2, 639, 230)
    expect(result.rects.map((r) => [r.y, r.width, r.height])).toEqual([
      [0, 639, 230],
      [230, 639, 230],
    ])
    expect(result.contentHeight).toBe(460)
    expect(result.scrolls).toBe(true)
  })

  it('scrolls one pixel under 230 per card', () => {
    const result = layout(2, 640, 229)
    expect(result.rects.map((r) => [r.width, r.height])).toEqual([
      [320, 230],
      [320, 230],
    ])
    expect(result.contentHeight).toBe(230)
    expect(result.scrolls).toBe(true)
  })

  it('rounds the content height to whole pixels', () => {
    const result = layout(7, 300, 1800)
    expect(result.contentHeight).toBe(1800)
    expect(result.scrolls).toBe(false)
  })

  it('keeps one column on a very narrow area', () => {
    const result = layout(3, 200, 400)
    expect(result.rects.map((r) => r.width)).toEqual([200, 200, 200])
    expect(result.scrolls).toBe(true)
  })
})

describe('insetRect', () => {
  it('shrinks the rect on every side', () => {
    expect(insetRect({ x: 10, y: 20, width: 100, height: 60 }, 5)).toEqual({
      x: 15,
      y: 25,
      width: 90,
      height: 50,
    })
  })

  it('never returns a negative size', () => {
    expect(insetRect({ x: 0, y: 0, width: 6, height: 4 }, 5)).toEqual({ x: 5, y: 5, width: 0, height: 0 })
  })
})
