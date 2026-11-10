import { describe, expect, it } from 'vitest'
import type { Timers } from './refresher'
import { createTypingPause } from './typingPause'

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

describe('createTypingPause', () => {
  it('is off until a keystroke', () => {
    const pause = createTypingPause({ onResume: () => {}, timers: new FakeTimers() })
    expect(pause.active()).toBe(false)
  })

  it('is on after a keystroke and waits 500 ms by default', () => {
    const timers = new FakeTimers()
    const pause = createTypingPause({ onResume: () => {}, timers })
    pause.touch()
    expect(pause.active()).toBe(true)
    expect([...timers.pending.values()][0].ms).toBe(500)
  })

  it('resumes once, after the delay, and is off again', () => {
    const timers = new FakeTimers()
    let resumed = 0
    const pause = createTypingPause({ onResume: () => resumed++, timers })
    pause.touch()
    expect(resumed).toBe(0)
    timers.fire()
    expect(resumed).toBe(1)
    expect(pause.active()).toBe(false)
    expect(timers.pending.size).toBe(0)
  })

  it('restarts the delay on each keystroke', () => {
    const timers = new FakeTimers()
    let resumed = 0
    const pause = createTypingPause({ onResume: () => resumed++, timers, delayMs: 200 })
    pause.touch()
    pause.touch()
    pause.touch()
    expect(timers.pending.size).toBe(1)
    expect([...timers.pending.values()][0].ms).toBe(200)
    timers.fire()
    expect(resumed).toBe(1)
  })

  it('can pause again after a resume', () => {
    const timers = new FakeTimers()
    let resumed = 0
    const pause = createTypingPause({ onResume: () => resumed++, timers })
    pause.touch()
    timers.fire()
    pause.touch()
    expect(pause.active()).toBe(true)
    timers.fire()
    expect(resumed).toBe(2)
  })

  it('does not resume after dispose', () => {
    const timers = new FakeTimers()
    let resumed = 0
    const pause = createTypingPause({ onResume: () => resumed++, timers })
    pause.touch()
    pause.dispose()
    expect(pause.active()).toBe(false)
    expect(timers.pending.size).toBe(0)
    expect(resumed).toBe(0)
  })

  it('ends the pause on demand without resuming', () => {
    const timers = new FakeTimers()
    let resumed = 0
    const pause = createTypingPause({ onResume: () => resumed++, timers })
    pause.touch()
    pause.end()
    expect(pause.active()).toBe(false)
    expect(timers.pending.size).toBe(0)
    expect(resumed).toBe(0)
    pause.touch()
    expect(pause.active()).toBe(true)
  })
})
