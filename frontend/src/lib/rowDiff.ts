/**
 * Returns the indexes of the rows to redraw: those whose signature differs
 * from the previous one. Every row is redrawn when there is no previous frame
 * or when the number of rows changed.
 */
export function changedRows(previous: readonly string[], next: readonly string[]): number[] {
  const all = previous.length !== next.length
  const changed: number[] = []
  for (let y = 0; y < next.length; y++) {
    if (all || previous[y] !== next[y]) changed.push(y)
  }
  return changed
}
