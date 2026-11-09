/** Returns the text of an error raised by a bound Go method or by the runtime. */
export function errorMessage(e: unknown): string {
  if (e instanceof Error) return e.message
  if (typeof e === 'string') return e
  return String(e)
}
