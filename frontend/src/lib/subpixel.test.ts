import { describe, expect, it } from 'vitest'
import { foldSubpixels, SUBPIXELS } from './subpixel'

/** Builds a one-row RGBA image from gray levels. */
function grayRow(levels: number[]): Uint8ClampedArray {
  const img = new Uint8ClampedArray(levels.length * 4)
  levels.forEach((v, i) => img.set([v, v, v, 255], i * 4))
  return img
}

function fold(levels: number[]): number[][] {
  const width = levels.length / SUBPIXELS
  const out = new Uint8ClampedArray(width * 4)
  foldSubpixels(grayRow(levels), out, width, 1)
  return Array.from({ length: width }, (_, x) => Array.from(out.slice(x * 4, x * 4 + 4)))
}

describe('foldSubpixels', () => {
  it('keeps a uniform image unchanged', () => {
    expect(fold(Array(9).fill(200))).toEqual([
      [200, 200, 200, 255],
      [200, 200, 200, 255],
      [200, 200, 200, 255],
    ])
  })

  it('lights the stripes nearest to a thin line', () => {
    // A one-sample white line on the red stripe of the middle pixel.
    const [left, middle, right] = fold([0, 0, 0, 255, 0, 0, 0, 0, 0])
    expect(middle[0]).toBeGreaterThan(middle[1])
    expect(middle[1]).toBeGreaterThan(middle[2])
    expect(left[2]).toBeGreaterThan(left[0])
    expect(right).toEqual([0, 0, 0, 255])
  })

  it('places a line a third of a pixel to the right differently', () => {
    const a = fold([0, 0, 0, 255, 0, 0, 0, 0, 0])[1]
    const b = fold([0, 0, 0, 0, 255, 0, 0, 0, 0])[1]
    expect(b[1]).toBeGreaterThan(a[1])
    expect(b[0]).toBeLessThan(a[0])
  })

  it('handles the image edges without reading outside', () => {
    const [first] = fold([255, 0, 0, 0, 0, 0])
    expect(first[0]).toBeGreaterThan(0)
    expect(first[3]).toBe(255)
  })
})
