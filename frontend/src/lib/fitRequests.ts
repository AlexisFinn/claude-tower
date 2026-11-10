import type { GridSize } from './bodySize'

/** Wait before a size that failed is asked again. */
export const FIT_RETRY_MS = 30_000

/** What came of asking a session for a size. */
export type FitOutcome = 'applied' | 'kept' | 'failed'

/** Tracks the sizes asked of the sessions, so that the same size is not asked on every cycle. */
export interface FitRequests {
  /**
   * Reports whether to ask a session for wanted, given the size of its last
   * screen and whether a client shows it. A size the session got is not asked
   * again, even if the session cannot reach it (a window split in several
   * panes); a failed one is asked again after FIT_RETRY_MS.
   */
  shouldAsk(pid: number, wanted: GridSize, screen: GridSize, attached: boolean): boolean
  /** Records the outcome of asking a session for a size; a kept size is asked again on the next cycle. */
  record(pid: number, size: GridSize, outcome: FitOutcome): void
  /** Forgets the requests of a session, so that its size is asked again. */
  forget(pid: number): void
  /** Forgets the sessions not in pids. */
  prune(pids: readonly number[]): void
}

function same(a: GridSize, b: GridSize): boolean {
  return a.cols === b.cols && a.rows === b.rows
}

/** Creates the tracker; now returns the current time in milliseconds. */
export function createFitRequests(now: () => number = Date.now): FitRequests {
  const asked = new Map<number, { size: GridSize; retryAt: number }>()
  return {
    shouldAsk(pid, wanted, screen, attached) {
      // A client resizes the window while it shows the session: the size is asked again once it leaves.
      if (attached) {
        asked.delete(pid)
        return false
      }
      if (same(wanted, screen)) return false
      const last = asked.get(pid)
      return !last || !same(last.size, wanted) || now() >= last.retryAt
    },
    record(pid, size, outcome) {
      if (outcome === 'kept') asked.delete(pid)
      else asked.set(pid, { size, retryAt: outcome === 'applied' ? Infinity : now() + FIT_RETRY_MS })
    },
    forget(pid) {
      asked.delete(pid)
    },
    prune(pids) {
      const live = new Set(pids)
      for (const pid of asked.keys()) if (!live.has(pid)) asked.delete(pid)
    },
  }
}
