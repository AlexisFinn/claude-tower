import { describe, expect, it } from 'vitest'
import { createFitRequests, FIT_RETRY_MS } from './fitRequests'

const card = { cols: 120, rows: 30 }
const big = { cols: 236, rows: 66 }

describe('createFitRequests', () => {
  it('asks a session whose screen has another size, and not one that has it', () => {
    const r = createFitRequests(() => 0)
    expect(r.shouldAsk(1, card, big, false)).toBe(true)
    expect(r.shouldAsk(1, card, card, false)).toBe(false)
  })

  it('does not ask again a size the session got, even if its screen never reaches it', () => {
    const r = createFitRequests(() => 0)
    r.record(1, card, 'applied')
    expect(r.shouldAsk(1, card, { cols: 59, rows: 30 }, false)).toBe(false)
    expect(r.shouldAsk(1, { cols: 100, rows: 30 }, { cols: 59, rows: 30 }, false)).toBe(true)
  })

  it('asks again on the next cycle when a client kept the size', () => {
    const r = createFitRequests(() => 0)
    r.record(1, card, 'kept')
    expect(r.shouldAsk(1, card, big, false)).toBe(true)
  })

  it('asks again a failed size only after the retry delay', () => {
    let t = 1000
    const r = createFitRequests(() => t)
    r.record(1, card, 'failed')
    expect(r.shouldAsk(1, card, big, false)).toBe(false)
    t += FIT_RETRY_MS
    expect(r.shouldAsk(1, card, big, false)).toBe(true)
  })

  it('never asks while a client shows the session, and asks again once it leaves', () => {
    const r = createFitRequests(() => 0)
    r.record(1, card, 'applied')
    expect(r.shouldAsk(1, card, big, true)).toBe(false)
    expect(r.shouldAsk(1, card, big, false)).toBe(true)
  })

  it('asks again after forget, and forgets the sessions that are gone', () => {
    const r = createFitRequests(() => 0)
    r.record(1, card, 'applied')
    r.record(2, card, 'applied')
    r.forget(1)
    expect(r.shouldAsk(1, card, big, false)).toBe(true)
    r.prune([1])
    expect(r.shouldAsk(2, card, big, false)).toBe(true)
  })
})
