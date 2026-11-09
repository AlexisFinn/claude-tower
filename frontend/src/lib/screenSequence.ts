/**
 * Returns the escape sequence that redraws a card body from the last lines of
 * a screen: automatic wrap is off, so long lines are cut on the right, and
 * every line clears the rest of its row.
 */
export function screenSequence(lines: string[], rows: number): string {
  const shown = lines.slice(-Math.max(1, rows))
  const body = shown.map((line) => `${line}\x1b[K`).join('\r\n')
  return `\x1b[?7l\x1b[H${body}\x1b[J`
}
