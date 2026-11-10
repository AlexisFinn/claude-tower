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

  it('folds each channel from its own channel of the samples', () => {
    // Three wide pixels: pure red, pure green, pure blue, each spanning all three samples of one output pixel.
    const src = new Uint8ClampedArray(9 * 4)
    for (let s = 0; s < 9; s++) {
      const color = [0, 0, 0]
      color[Math.floor(s / 3)] = 255
      src.set([...color, 255], s * 4)
    }
    const out = new Uint8ClampedArray(3 * 4)
    foldSubpixels(src, out, 3, 1)
    // Each output channel only sees the same channel of its five nearest samples.
    expect(out[0]).toBe(255) // red of the red pixel
    expect(out[1]).toBe(49) // green of the red pixel: 8/256 of the green pixel
    expect(out[8 + 0]).toBe(0) // red of the blue pixel: the red samples are out of reach
    expect(out[4 + 1]).toBe(248) // green of the green pixel: 240/256, the outer samples being red and blue
    expect(out[8 + 2]).toBe(255) // blue of the blue pixel
  })

  it('filters in linear light', () => {
    // A white sample beside black ones: in linear light, 86/256 of white on the centre stripe is about 157 in sRGB,
    // where a filter in sRGB values would give 86.
    const [, middle] = fold([0, 0, 0, 0, 255, 0, 0, 0, 0])
    expect(middle[1]).toBeGreaterThanOrEqual(155)
    expect(middle[1]).toBeLessThanOrEqual(159)
  })

  it('handles the image edges without reading outside', () => {
    const [first] = fold([255, 0, 0, 0, 0, 0])
    expect(first[0]).toBeGreaterThan(0)
    expect(first[3]).toBe(255)
  })
})
