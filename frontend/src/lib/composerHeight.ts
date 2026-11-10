/** What the last height measure of the composer text area knew about its text. */
export interface HeightState {
  /** Number of logical lines, or -1 before the first measure. */
  lines: number
  length: number
}

/** The state before the first measure: any text needs one. */
export const INITIAL_HEIGHT_STATE: HeightState = { lines: -1, length: 0 }

/** Returns the number of logical lines of a text, at least 1. */
export function countLines(text: string): number {
  let lines = 1
  for (let i = text.indexOf('\n'); i !== -1; i = text.indexOf('\n', i + 1)) lines++
  return lines
}

/** Returns the state to remember after measuring a text. */
export function heightStateOf(text: string): HeightState {
  return { lines: countLines(text), length: text.length }
}

/**
 * Reports whether the height of the text area must be measured again: the
 * number of logical lines changed, the text got shorter (the wrapped height
 * may have shrunk), or the content overflows the current height. overflows is
 * called last because reading the content height forces a layout.
 */
export function needsHeightMeasure(previous: HeightState, text: string, overflows: () => boolean): boolean {
  const next = heightStateOf(text)
  return next.lines !== previous.lines || next.length < previous.length || overflows()
}
