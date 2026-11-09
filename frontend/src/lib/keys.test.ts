import { describe, expect, it } from 'vitest'
import { gridKeyAction, terminalKeyAction, type KeyLike } from './keys'

const key = (k: string, mods: Partial<KeyLike> = {}): KeyLike => ({
  key: k,
  ctrlKey: false,
  shiftKey: false,
  altKey: false,
  metaKey: false,
  ...mods,
})

describe('gridKeyAction', () => {
  it('maps arrows and hjkl to moves', () => {
    expect(gridKeyAction(key('ArrowLeft'))).toEqual({ kind: 'move', dir: 'left' })
    expect(gridKeyAction(key('l'))).toEqual({ kind: 'move', dir: 'right' })
    expect(gridKeyAction(key('k'))).toEqual({ kind: 'move', dir: 'up' })
    expect(gridKeyAction(key('ArrowDown'))).toEqual({ kind: 'move', dir: 'down' })
  })

  it('maps the action keys', () => {
    expect(gridKeyAction(key('Enter'))).toEqual({ kind: 'focus' })
    expect(gridKeyAction(key('o'))).toEqual({ kind: 'fullscreen' })
    expect(gridKeyAction(key('i'))).toEqual({ kind: 'prompt' })
    expect(gridKeyAction(key('/'))).toEqual({ kind: 'prompt' })
    expect(gridKeyAction(key('n'))).toEqual({ kind: 'new' })
    expect(gridKeyAction(key('x'))).toEqual({ kind: 'kill' })
    expect(gridKeyAction(key('?', { shiftKey: true }))).toEqual({ kind: 'help' })
  })

  it('ignores other keys and shortcuts with modifiers', () => {
    expect(gridKeyAction(key('z'))).toBeNull()
    expect(gridKeyAction(key('n', { ctrlKey: true }))).toBeNull()
    expect(gridKeyAction(key('o', { metaKey: true }))).toBeNull()
  })
})

describe('terminalKeyAction', () => {
  it('catches Ctrl+Shift+Q whatever the case of the key', () => {
    expect(terminalKeyAction(key('Q', { ctrlKey: true, shiftKey: true }))).toBe('exit')
    expect(terminalKeyAction(key('q', { ctrlKey: true, shiftKey: true }))).toBe('exit')
  })

  it('does not catch Ctrl+Q alone', () => {
    expect(terminalKeyAction(key('q', { ctrlKey: true }))).toBeNull()
  })

  it('turns Shift+Enter into a newline', () => {
    expect(terminalKeyAction(key('Enter', { shiftKey: true }))).toBe('newline')
    expect(terminalKeyAction(key('Enter'))).toBeNull()
  })

  it('catches copy and paste', () => {
    expect(terminalKeyAction(key('C', { ctrlKey: true, shiftKey: true }))).toBe('copy')
    expect(terminalKeyAction(key('V', { ctrlKey: true, shiftKey: true }))).toBe('paste')
  })

  it('passes other keys on', () => {
    expect(terminalKeyAction(key('c', { ctrlKey: true }))).toBeNull()
    expect(terminalKeyAction(key('a'))).toBeNull()
  })
})
