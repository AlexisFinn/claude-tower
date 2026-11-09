import { describe, expect, it } from 'vitest'
import { createFailureTracker } from './screenFailures'

describe('createFailureTracker', () => {
  it('flags a session after three consecutive failures', () => {
    const t = createFailureTracker()
    expect(t.fail(1)).toBe(false)
    expect(t.fail(1)).toBe(false)
    expect(t.unavailable(1)).toBe(false)
    expect(t.fail(1)).toBe(true)
    expect(t.unavailable(1)).toBe(true)
    expect(t.fail(1)).toBe(true)
  })

  it('counts each session on its own', () => {
    const t = createFailureTracker()
    t.fail(1)
    t.fail(1)
    t.fail(2)
    expect(t.fail(1)).toBe(true)
    expect(t.unavailable(2)).toBe(false)
  })

  it('removes the flag on a success and reports the change', () => {
    const t = createFailureTracker()
    for (let i = 0; i < 3; i++) t.fail(1)
    expect(t.succeed(1)).toBe(true)
    expect(t.unavailable(1)).toBe(false)
    expect(t.succeed(1)).toBe(false)
  })

  it('restarts the count after a success', () => {
    const t = createFailureTracker()
    t.fail(1)
    t.fail(1)
    t.succeed(1)
    t.fail(1)
    expect(t.fail(1)).toBe(false)
    expect(t.fail(1)).toBe(true)
  })

  it('forgets the sessions that are gone', () => {
    const t = createFailureTracker()
    for (let i = 0; i < 3; i++) {
      t.fail(1)
      t.fail(2)
    }
    t.prune([2])
    expect(t.unavailable(1)).toBe(false)
    expect(t.unavailable(2)).toBe(true)
  })

  it('honours a custom threshold', () => {
    const t = createFailureTracker(1)
    expect(t.fail(1)).toBe(true)
  })
})
