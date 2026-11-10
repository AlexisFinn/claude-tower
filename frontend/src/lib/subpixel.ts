/**
 * Subpixel rendering: an image drawn three times wider than the screen is
 * folded onto the red, green and blue stripes of each pixel of an LCD panel
 * whose stripes run left to right in RGB order, which triples the horizontal
 * resolution of the text.
 */

/** Horizontal oversampling of the source image: one sample per color stripe. */
export const SUBPIXELS = 3

/** Low-pass weights over five neighboring samples, which keep color fringes faint (the default filter of FreeType). */
const WEIGHTS = [8 / 256, 77 / 256, 86 / 256, 77 / 256, 8 / 256]

const TO_LINEAR = new Float32Array(256)
for (let i = 0; i < 256; i++) {
  const c = i / 255
  TO_LINEAR[i] = c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
}

const LEVELS = 4096
const TO_SRGB = new Uint8ClampedArray(LEVELS + 1)
for (let i = 0; i <= LEVELS; i++) {
  const c = i / LEVELS
  TO_SRGB[i] = Math.round(255 * (c <= 0.0031308 ? c * 12.92 : 1.055 * c ** (1 / 2.4) - 0.055))
}

/**
 * Folds src, an RGBA image width * SUBPIXELS wide and height high, into out,
 * an RGBA image width wide: the red, green and blue values of a pixel are the
 * filtered values of the same channel at its three subpixel positions. The
 * filter works in linear light, so thin strokes keep their weight.
 */
export function foldSubpixels(src: Uint8ClampedArray, out: Uint8ClampedArray, width: number, height: number): void {
  const wide = width * SUBPIXELS
  for (let y = 0; y < height; y++) {
    const row = y * wide * 4
    const outRow = y * width * 4
    for (let x = 0; x < width; x++) {
      const o = outRow + x * 4
      for (let ch = 0; ch < 3; ch++) {
        const centre = x * SUBPIXELS + ch
        let sum = 0
        for (let k = 0; k < WEIGHTS.length; k++) {
          const s = Math.min(wide - 1, Math.max(0, centre + k - 2))
          sum += WEIGHTS[k] * TO_LINEAR[src[row + s * 4 + ch]]
        }
        out[o + ch] = TO_SRGB[Math.round(Math.min(1, sum) * LEVELS)]
      }
      out[o + 3] = 255
    }
  }
}
