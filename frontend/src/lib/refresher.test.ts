import { describe, expect, it } from 'vitest'
import { createRefresher, type Timers } from './refresher'

class FakeTimers implements Timers {
  next = 1
  pending = new Map<number, { fn: () => void; ms: number }>()
  set(fn: () => void, ms: number): unknown {
    const id = this.next++
    this.pending.set(id, { fn, ms })
    return id
  }
  clear(handle: unknown): void {
    this.pending.delete(handle as number)
  }
  fire(): void {
    const [id, t] = [...this.pending.entries()][0]
    this.pending.delete(id)
    t.fn()
  }
}

const flush = () => new Promise((resolve) => setTimeout(resolve, 0))

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>((r) => (resolve = r))
  return { promise, resolve }
}

describe('createRefresher', () => {
  it('schedules the next cycle after the end of the previous one', async () => {
    const timers = new FakeTimers()
    const gate = deferred()
    let runs = 0
    const r = createRefresher({
      run: () => {
        runs++
        return gate.promise
      },
      timers,
      delayMs: 1000,
    })
    r.start()
    await flush()
    expect(runs).toBe(1)
    expect(timers.pending.size).toBe(0)
    gate.resolve()
    await flush()
    expect(timers.pending.size).toBe(1)
    expect([...timers.pending.values()][0].ms).toBe(1000)
  })

  it('runs again when the timer fires', async () => {
    const timers = new FakeTimers()
    let runs = 0
    const r = createRefresher({ run: async () => void runs++, timers })
    r.start()
    await flush()
    timers.fire()
    await flush()
    expect(runs).toBe(2)
    expect(timers.pending.size).toBe(1)
  })

  it('skips cycles while paused and keeps polling', async () => {
    const timers = new FakeTimers()
    let runs = 0
    let paused = true
    const r = createRefresher({ run: async () => void runs++, paused: () => paused, timers })
    r.start()
    await flush()
    expect(runs).toBe(0)
    expect(timers.pending.size).toBe(1)
    timers.fire()
    await flush()
    expect(runs).toBe(0)
    paused = false
    timers.fire()
    await flush()
    expect(runs).toBe(1)
  })

  it('keeps going after a failed cycle', async () => {
    const timers = new FakeTimers()
    const r = createRefresher({
      run: async () => {
        throw new Error('boom')
      },
      timers,
    })
    r.start()
    await flush()
    expect(timers.pending.size).toBe(1)
  })

  it('does not overlap cycles on kick', async () => {
    const timers = new FakeTimers()
    const gate = deferred()
    let runs = 0
    const r = createRefresher({
      run: () => {
        runs++
        return gate.promise
      },
      timers,
    })
    r.start()
    await flush()
    r.kick()
    await flush()
    expect(runs).toBe(1)
  })

  it('kick replaces the pending timer with an immediate cycle', async () => {
    const timers = new FakeTimers()
    let runs = 0
    const r = createRefresher({ run: async () => void runs++, timers })
    r.start()
    await flush()
    r.kick()
    await flush()
    expect(runs).toBe(2)
    expect(timers.pending.size).toBe(1)
  })

  it('stops scheduling after stop', async () => {
    const timers = new FakeTimers()
    let runs = 0
    const r = createRefresher({ run: async () => void runs++, timers })
    r.start()
    await flush()
    r.stop()
    expect(timers.pending.size).toBe(0)
    r.kick()
    await flush()
    expect(runs).toBe(1)
  })

  it('does not reschedule when stopped during a cycle', async () => {
    const timers = new FakeTimers()
    const gate = deferred()
    const r = createRefresher({ run: () => gate.promise, timers })
    r.start()
    await flush()
    r.stop()
    gate.resolve()
    await flush()
    expect(timers.pending.size).toBe(0)
  })
})
