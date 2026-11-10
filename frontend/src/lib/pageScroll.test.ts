import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { onPageScroll } from './pageScroll'

let handlers: Map<string, () => void>
const addEventListener = vi.fn((type: string, handler: () => void) => handlers.set(type, handler))
const removeEventListener = vi.fn((type: string) => handlers.delete(type))

beforeEach(() => {
  handlers = new Map()
  addEventListener.mockClear()
  removeEventListener.mockClear()
  vi.stubGlobal('window', { addEventListener, removeEventListener })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('onPageScroll', () => {
  it('shares one capturing listener between the subscribers', () => {
    const a = vi.fn()
    const b = vi.fn()
    const stopA = onPageScroll(a)
    const stopB = onPageScroll(b)
    expect(addEventListener).toHaveBeenCalledTimes(1)
    expect(addEventListener).toHaveBeenCalledWith('scroll', expect.any(Function), { capture: true, passive: true })
    handlers.get('scroll')?.()
    expect(a).toHaveBeenCalledTimes(1)
    expect(b).toHaveBeenCalledTimes(1)
    stopA()
    stopB()
  })

  it('keeps listening while a subscriber is left and stops after the last one', () => {
    const a = vi.fn()
    const b = vi.fn()
    const stopA = onPageScroll(a)
    const stopB = onPageScroll(b)
    stopA()
    expect(removeEventListener).not.toHaveBeenCalled()
    handlers.get('scroll')?.()
    expect(a).not.toHaveBeenCalled()
    expect(b).toHaveBeenCalledTimes(1)
    stopB()
    expect(removeEventListener).toHaveBeenCalledTimes(1)
    expect(removeEventListener).toHaveBeenCalledWith('scroll', expect.any(Function), { capture: true })
    expect(handlers.has('scroll')).toBe(false)
  })

  it('listens again after every subscriber left', () => {
    onPageScroll(() => {})()
    const stop = onPageScroll(() => {})
    expect(addEventListener).toHaveBeenCalledTimes(2)
    stop()
  })

  it('subscribes the same function twice independently', () => {
    const a = vi.fn()
    const stop1 = onPageScroll(a)
    const stop2 = onPageScroll(a)
    stop1()
    handlers.get('scroll')?.()
    expect(a).toHaveBeenCalledTimes(1)
    stop2()
  })
})
