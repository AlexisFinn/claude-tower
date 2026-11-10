import { sameSize, type GridSize } from './bodySize'

/** Wait before a size that failed is asked again. */
export const FIT_RETRY_MS = 30_000
/** Wait before a size the session got is checked again, in case a client changed it unseen. */
export const FIT_RECHECK_MS = 5 * 60_000

/** What came of asking a session for a size. */
export type FitOutcome = 'applied' | 'kept' | 'failed'

/** Tracks the sizes asked of the sessions, so that the same size is not asked on every cycle. */
export interface FitRequests {
  /**
   * Reports whether to ask a session for wanted, given the size of its last
   * screen and whether a client shows it. A size the session got is asked
   * again after FIT_RECHECK_MS only, even if the session cannot reach it (a
   * window split in several panes); a failed one after FIT_RETRY_MS.
   */
  shouldAsk(pid: number, wanted: GridSize, screen: GridSize, attached: boolean): boolean
  /** Returns the ticket to record the outcome of a request with: forget makes older tickets void. */
  ticket(pid: number): number
  /** Records the outcome of a request; a kept size is asked again on the next cycle. */
  record(pid: number, size: GridSize, outcome: FitOutcome, ticket: number): void
  /** Forgets the requests of a session, so that its size is asked again. */
  forget(pid: number): void
  /** Forgets the sessions not in pids. */
  prune(pids: readonly number[]): void
}

/** Creates the tracker; now returns the current time in milliseconds. */
export function createFitRequests(now: () => number = Date.now): FitRequests {
  const asked = new Map<number, { size: GridSize; retryAt: number }>()
  const tickets = new Map<number, number>()
  return {
    shouldAsk(pid, wanted, screen, attached) {
      // A client resizes the window while it shows the session: the size is asked again once it leaves.
      if (attached) {
        asked.delete(pid)
        return false
      }
      if (sameSize(wanted, screen)) return false
      const last = asked.get(pid)
      return !last || !sameSize(last.size, wanted) || now() >= last.retryAt
    },
    ticket(pid) {
      return tickets.get(pid) ?? 0
    },
    record(pid, size, outcome, ticket) {
      if (ticket !== (tickets.get(pid) ?? 0)) return
      if (outcome === 'kept') asked.delete(pid)
      else asked.set(pid, { size, retryAt: now() + (outcome === 'applied' ? FIT_RECHECK_MS : FIT_RETRY_MS) })
    },
    forget(pid) {
      asked.delete(pid)
      tickets.set(pid, (tickets.get(pid) ?? 0) + 1)
    },
    prune(pids) {
      const live = new Set(pids)
      for (const pid of asked.keys()) if (!live.has(pid)) asked.delete(pid)
      for (const pid of tickets.keys()) if (!live.has(pid)) tickets.delete(pid)
    },
  }
}
