/** Consecutive Screen failures after which a card shows its screen as unavailable. */
export const FAILURE_THRESHOLD = 3

/** Counts consecutive Screen failures per session. */
export interface FailureTracker {
  /** Records a failure and reports whether the screen is now unavailable. */
  fail(pid: number): boolean
  /** Records a success and reports whether the unavailable state changed. */
  succeed(pid: number): boolean
  unavailable(pid: number): boolean
  /** Forgets the sessions that are not in pids. */
  prune(pids: number[]): void
}

/** Creates a tracker: a session is unavailable after threshold failures in a row. */
export function createFailureTracker(threshold = FAILURE_THRESHOLD): FailureTracker {
  const counts = new Map<number, number>()
  const unavailable = (pid: number): boolean => (counts.get(pid) ?? 0) >= threshold
  return {
    fail(pid) {
      counts.set(pid, (counts.get(pid) ?? 0) + 1)
      return unavailable(pid)
    },
    succeed(pid) {
      const was = unavailable(pid)
      counts.delete(pid)
      return was
    },
    unavailable,
    prune(pids) {
      const keep = new Set(pids)
      for (const pid of [...counts.keys()]) if (!keep.has(pid)) counts.delete(pid)
    },
  }
}
