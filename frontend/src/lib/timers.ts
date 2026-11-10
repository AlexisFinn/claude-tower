/** The timer functions of a component that schedules work, replaceable in tests. */
export interface Timers {
  set(fn: () => void, ms: number): unknown
  clear(handle: unknown): void
}

/** The timers of the browser. */
export const realTimers: Timers = {
  set: (fn, ms) => setTimeout(fn, ms),
  clear: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
}
