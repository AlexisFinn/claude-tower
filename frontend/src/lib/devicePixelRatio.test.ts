import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { watchDevicePixelRatio } from './devicePixelRatio'

interface FakeQuery {
  media: string
  handler: (() => void) | null
  removed: boolean
}

let queries: FakeQuery[]
let ratio: number

beforeEach(() => {
  queries = []
  ratio = 1
  vi.stubGlobal('window', {
    get devicePixelRatio() {
      return ratio
    },
    matchMedia: (media: string) => {
      const query: FakeQuery = { media, handler: null, removed: false }
      queries.push(query)
      return {
        addEventListener: (_type: string, handler: () => void) => (query.handler = handler),
        removeEventListener: () => (query.removed = true),
      }
    },
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

/** Changes the ratio the way the browser does: the pending query fires once. */
function change(to: number): void {
  ratio = to
  const pending = queries[queries.length - 1]
  const handler = pending.handler
  pending.handler = null
  handler?.()
}

describe('watchDevicePixelRatio', () => {
  it('shares one media query between the watchers', () => {
    const stopA = watchDevicePixelRatio(() => {})
    const stopB = watchDevicePixelRatio(() => {})
    expect(queries).toHaveLength(1)
    expect(queries[0].media).toBe('(resolution: 1dppx)')
    stopA()
    stopB()
  })

  it('tells every watcher the new ratio and watches the new one', () => {
    const a = vi.fn()
    const b = vi.fn()
    const stopA = watchDevicePixelRatio(a)
    const stopB = watchDevicePixelRatio(b)
    change(2)
    expect(a).toHaveBeenCalledWith(2)
    expect(b).toHaveBeenCalledWith(2)
    expect(queries).toHaveLength(2)
    expect(queries[1].media).toBe('(resolution: 2dppx)')
    change(1.5)
    expect(a).toHaveBeenLastCalledWith(1.5)
    stopA()
    stopB()
  })

  it('stops telling a watcher that left, and releases the query after the last one', () => {
    const a = vi.fn()
    const b = vi.fn()
    const stopA = watchDevicePixelRatio(a)
    const stopB = watchDevicePixelRatio(b)
    stopA()
    expect(queries[0].removed).toBe(false)
    change(2)
    expect(a).not.toHaveBeenCalled()
    expect(b).toHaveBeenCalledWith(2)
    stopB()
    expect(queries[1].removed).toBe(true)
  })

  it('watches again after every watcher left', () => {
    watchDevicePixelRatio(() => {})()
    const stop = watchDevicePixelRatio(() => {})
    expect(queries).toHaveLength(2)
    stop()
  })
})
