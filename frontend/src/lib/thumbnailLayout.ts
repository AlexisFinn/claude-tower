/**
 * Returns the start and the end, in backing pixels, of cell number index when
 * every cell is size wide: the edges are whole pixels and shared by neighbours,
 * so cells join without seams and the grid keeps its exact width.
 */
export function cellBounds(index: number, size: number): [number, number] {
  return [Math.round(index * size), Math.round((index + 1) * size)]
}
