import { describe, expect, it } from 'vitest'
import { createToasts, MAX_TOASTS, TOAST_DURATION_MS, type Timers, type Toast } from './toasts'

/** Timers driven by hand: time only moves when the test advances it. */
function fakeTimers() {
  let now = 0
  let nextHandle = 1
  const pending = new Map<number, { at: number; fn: () => void }>()
  const timers: Timers = {
    set: (fn, ms) => {
      const handle = nextHandle++
      pending.set(handle, { at: now + ms, fn })
      return handle
    },
    clear: (handle) => {
      pending.delete(handle as number)
    },
  }
  function advance(ms: number): void {
    now += ms
    for (const [handle, timer] of [...pending]) {
      if (timer.at <= now && pending.delete(handle)) timer.fn()
    }
  }
  return { timers, advance, pending: () => pending.size }
}

function setup() {
  const clock = fakeTimers()
  let current: Toast[] = []
  const queue = createToasts((t) => (current = t), clock.timers)
  return { clock, queue, list: () => current }
}

describe('createToasts', () => {
  it('shows a pushed notification', () => {
    const { queue, list } = setup()
    const id = queue.push('info', 'Bonjour')
    expect(list()).toEqual([{ id, kind: 'info', text: 'Bonjour' }])
  })

  it('removes a notification after 5 s, an error after 8 s', () => {
    const { clock, queue, list } = setup()
    queue.push('info', 'a')
    queue.push('error', 'b')
    clock.advance(TOAST_DURATION_MS.info - 1)
    expect(list().map((t) => t.text)).toEqual(['a', 'b'])
    clock.advance(1)
    expect(list().map((t) => t.text)).toEqual(['b'])
    clock.advance(TOAST_DURATION_MS.error - TOAST_DURATION_MS.info - 1)
    expect(list().map((t) => t.text)).toEqual(['b'])
    clock.advance(1)
    expect(list()).toEqual([])
  })

  it('removes a dismissed notification and cancels its expiry', () => {
    const { clock, queue, list } = setup()
    const id = queue.push('warning', 'a')
    queue.dismiss(id)
    expect(list()).toEqual([])
    expect(clock.pending()).toBe(0)
  })

  it('ignores the dismissal of an unknown notification', () => {
    const { queue, list } = setup()
    queue.push('info', 'a')
    queue.dismiss(999)
    expect(list()).toHaveLength(1)
  })

  it('does not duplicate the same text and restarts its expiry', () => {
    const { clock, queue, list } = setup()
    const first = queue.push('error', 'même erreur')
    clock.advance(6000)
    const second = queue.push('error', 'même erreur')
    expect(second).toBe(first)
    expect(list()).toHaveLength(1)
    clock.advance(6000)
    expect(list()).toHaveLength(1)
    clock.advance(2000)
    expect(list()).toEqual([])
  })

  it('keeps separate notifications of different kinds with the same text', () => {
    const { queue, list } = setup()
    queue.push('info', 'x')
    queue.push('error', 'x')
    expect(list()).toHaveLength(2)
  })

  it('drops the oldest notifications beyond the maximum', () => {
    const { clock, queue, list } = setup()
    for (let i = 0; i < MAX_TOASTS + 2; i++) queue.push('info', `n${i}`)
    expect(list().map((t) => t.text)).toEqual(['n2', 'n3', 'n4', 'n5'])
    expect(clock.pending()).toBe(MAX_TOASTS)
  })

  it('cancels every pending expiry on dispose', () => {
    const { clock, queue } = setup()
    queue.push('info', 'a')
    queue.push('error', 'b')
    queue.dispose()
    expect(clock.pending()).toBe(0)
  })
})
