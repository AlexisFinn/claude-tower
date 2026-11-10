/** Oversampling of the canvas backing store relative to its displayed size, so the browser downscales an image. */
export const OVERSAMPLING = 2
/** Largest backing store of a thumbnail, in pixels. */
export const MAX_CANVAS_PIXELS = 2_500_000

/**
 * Returns how many backing pixels a CSS pixel of the canvas has: OVERSAMPLING
 * times the device pixel ratio, lowered (to 1 at least) so that the backing
 * store of a cssWidth x cssHeight canvas stays under MAX_CANVAS_PIXELS.
 */
export function canvasScale(dpr: number, cssWidth: number, cssHeight: number): number {
  const display = Math.max(1, Number.isFinite(dpr) ? dpr : 1)
  const area = Math.max(1, cssWidth * cssHeight)
  // The pixel cap only trims the oversampling: the scale never drops below the
  // display resolution, which would blur the text.
  return Math.max(display, Math.min(OVERSAMPLING * display, Math.sqrt(MAX_CANVAS_PIXELS / area)))
}

/**
 * Returns the start and the end, in backing pixels, of cell number index when
 * every cell is size wide: the edges are whole pixels and shared by neighbours,
 * so cells join without seams and the grid keeps its exact width.
 */
export function cellBounds(index: number, size: number): [number, number] {
  return [Math.round(index * size), Math.round((index + 1) * size)]
}
