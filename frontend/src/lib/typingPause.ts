import { realTimers, type Timers } from './timers'

/** Options of createTypingPause. */
export interface TypingPauseOptions {
  /** Called when the pause ends. */
  onResume: () => void
  /** Delay between the last keystroke and the end of the pause. */
  delayMs?: number
  timers?: Timers
}

/** Tells whether the user is typing, until a delay after the last keystroke. */
export interface TypingPause {
  /** Records a keystroke: starts the pause, or extends it. */
  touch(): void
  /** Reports whether the pause is on. */
  active(): boolean
  /** Ends the pause now, without calling onResume. */
  end(): void
}

/**
 * Creates a typing pause: it is on from the first touch until delayMs after
 * the last one, then onResume is called once.
 */
export function createTypingPause(options: TypingPauseOptions): TypingPause {
  const { onResume, delayMs = 500, timers = realTimers } = options
  let handle: unknown = null

  const clearTimer = (): void => {
    if (handle !== null) timers.clear(handle)
    handle = null
  }

  return {
    touch() {
      clearTimer()
      handle = timers.set(() => {
        handle = null
        onResume()
      }, delayMs)
    },
    active() {
      return handle !== null
    },
    end() {
      clearTimer()
    },
  }
}
