import { describe, expect, it } from 'vitest'
import { isQuitShortcut } from './quitShortcut'

const key = (k: string, mods: Partial<Record<'ctrlKey' | 'shiftKey' | 'altKey' | 'metaKey', boolean>> = {}) => ({
  key: k,
  ctrlKey: false,
  shiftKey: false,
  altKey: false,
  metaKey: false,
  ...mods,
})

describe('isQuitShortcut', () => {
  it('takes Ctrl+Q, whatever the case of the letter', () => {
    expect(isQuitShortcut(key('q', { ctrlKey: true }), false)).toBe(true)
    expect(isQuitShortcut(key('Q', { ctrlKey: true }), false)).toBe(true)
  })

  it('leaves Ctrl+Shift+Q, which leaves the fullscreen view, and other combinations alone', () => {
    expect(isQuitShortcut(key('Q', { ctrlKey: true, shiftKey: true }), false)).toBe(false)
    expect(isQuitShortcut(key('q', { ctrlKey: true, altKey: true }), false)).toBe(false)
    expect(isQuitShortcut(key('q'), false)).toBe(false)
  })

  it('does nothing while the fullscreen view or a dialog is open', () => {
    expect(isQuitShortcut(key('q', { ctrlKey: true }), true)).toBe(false)
  })
})
