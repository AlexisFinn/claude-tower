/** The timer functions of the refresher, replaceable in tests. */
export interface Timers {
  set(fn: () => void, ms: number): unknown
  clear(handle: unknown): void
}

/** Options of createRefresher. */
export interface RefresherOptions {
  run: () => Promise<void>
  delayMs?: number
  paused?: () => boolean
  timers?: Timers
}

/** Runs a cycle in a loop, each cycle starting after the end of the previous one. */
export interface Refresher {
  start(): void
  stop(): void
  /** Runs a cycle now, unless one is already running. */
  kick(): void
}

const realTimers: Timers = {
  set: (fn, ms) => setTimeout(fn, ms),
  clear: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
}

/**
 * Creates a refresher. The next cycle is scheduled delayMs after the end of
 * the previous one, and a cycle is skipped while paused returns true.
 */
export function createRefresher(options: RefresherOptions): Refresher {
  const { run, delayMs = 1000, paused = () => false, timers = realTimers } = options
  let handle: unknown = null
  let running = false
  let active = false

  const schedule = (): void => {
    if (active && handle === null) handle = timers.set(tick, delayMs)
  }

  const clearTimer = (): void => {
    if (handle !== null) timers.clear(handle)
    handle = null
  }

  async function tick(): Promise<void> {
    handle = null
    if (!active) return
    if (!paused()) {
      running = true
      try {
        await run()
      } catch {
        // the next cycle retries
      } finally {
        running = false
      }
    }
    schedule()
  }

  return {
    start() {
      if (active) return
      active = true
      if (!running) void tick()
    },
    stop() {
      active = false
      clearTimer()
    },
    kick() {
      if (!active || running) return
      clearTimer()
      void tick()
    },
  }
}
