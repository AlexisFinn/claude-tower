import { describe, expect, it } from 'vitest'
import { canvasScale, cellBounds, MAX_CANVAS_PIXELS } from './thumbnailLayout'

describe('canvasScale', () => {
  it('oversamples twice the device pixel ratio', () => {
    expect(canvasScale(1, 500, 300)).toBe(2)
    expect(canvasScale(2, 500, 300)).toBe(4)
    expect(canvasScale(1.5, 500, 300)).toBe(3)
  })

  it('treats an invalid ratio as 1', () => {
    expect(canvasScale(0, 500, 300)).toBe(2)
    expect(canvasScale(NaN, 500, 300)).toBe(2)
  })

  it('bounds the backing store by trimming the oversampling', () => {
    const scale = canvasScale(1, 1900, 1000)
    expect(scale).toBeLessThan(2)
    expect(1900 * 1000 * scale * scale).toBeLessThanOrEqual(MAX_CANVAS_PIXELS + 1)
  })

  it('never goes below the display resolution', () => {
    expect(canvasScale(2, 1600, 900)).toBe(2)
    expect(canvasScale(1, 10000, 10000)).toBe(1)
  })
})

describe('cellBounds', () => {
  it('shares the edges between neighbours', () => {
    for (let i = 0; i < 50; i++) {
      expect(cellBounds(i, 3.37)[1]).toBe(cellBounds(i + 1, 3.37)[0])
    }
  })

  it('spans exactly the rounded grid width', () => {
    expect(cellBounds(0, 3.37)[0]).toBe(0)
    expect(cellBounds(199, 3.37)[1]).toBe(Math.round(200 * 3.37))
  })

  it('gives whole-number cells of that size', () => {
    expect(cellBounds(2, 10)).toEqual([20, 30])
  })
})
