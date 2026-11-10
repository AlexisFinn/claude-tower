import { describe, expect, it } from 'vitest'
import type { KeyLike } from './keys'
import { routeKey, type KeyLayers } from './keyRouting'

const GRID: KeyLayers = { fullscreen: false, confirm: false, newSession: false, help: false }

function key(k: string, mods: Partial<KeyLike> = {}): KeyLike {
  return { key: k, ctrlKey: false, shiftKey: false, altKey: false, metaKey: false, ...mods }
}

const GRID_KEYS = ['ArrowLeft', 'h', 'j', 'k', 'l', 'Enter', 'o', 'i', '/', 'n', 'x', '?']

describe('routeKey on the grid', () => {
  it('maps a grid key to its action', () => {
    expect(routeKey(GRID, key('x'), 'other')).toEqual({ layer: 'grid', action: { kind: 'kill' } })
    expect(routeKey(GRID, key('j'), 'other')).toEqual({ layer: 'grid', action: { kind: 'move', dir: 'down' } })
  })

  it('ignores a key without action', () => {
    expect(routeKey(GRID, key('z'), 'other')).toBeNull()
    expect(routeKey(GRID, key('x', { ctrlKey: true }), 'other')).toBeNull()
  })

  it('never acts while typing in the composer', () => {
    for (const k of GRID_KEYS) expect(routeKey(GRID, key(k), 'text')).toBeNull()
  })

  it('leaves Enter to a focused button but not the other keys', () => {
    expect(routeKey(GRID, key('Enter'), 'button')).toBeNull()
    expect(routeKey(GRID, key('x'), 'button')).toEqual({ layer: 'grid', action: { kind: 'kill' } })
  })
})

describe('routeKey under a layer', () => {
  it('ignores every key in full screen', () => {
    for (const k of [...GRID_KEYS, 'Escape', 'y']) {
      expect(routeKey({ ...GRID, fullscreen: true }, key(k), 'other')).toBeNull()
    }
  })

  it('full screen wins over the dialogs', () => {
    expect(routeKey({ ...GRID, fullscreen: true, confirm: true, help: true }, key('y'), 'other')).toBeNull()
  })

  it('never lets a grid key act under a confirmation dialog', () => {
    const layers = { ...GRID, confirm: true }
    for (const k of ['ArrowLeft', 'h', 'j', 'k', 'l', 'o', 'i', '/', 'n', 'x', '?']) {
      const route = routeKey(layers, key(k), 'other')
      expect(route === null || route.layer === 'confirm').toBe(true)
    }
    expect(routeKey(layers, key('x'), 'other')).toBeNull()
    expect(routeKey(layers, key('?'), 'other')).toBeNull()
  })

  it('accepts or cancels a confirmation dialog', () => {
    const layers = { ...GRID, confirm: true }
    expect(routeKey(layers, key('y'), 'other')).toEqual({ layer: 'confirm', action: 'accept' })
    expect(routeKey(layers, key('Enter'), 'other')).toEqual({ layer: 'confirm', action: 'accept' })
    expect(routeKey(layers, key('Escape'), 'other')).toEqual({ layer: 'confirm', action: 'cancel' })
    expect(routeKey(layers, key('n'), 'other')).toEqual({ layer: 'confirm', action: 'cancel' })
  })

  it('leaves Enter to a focused button of a confirmation dialog', () => {
    expect(routeKey({ ...GRID, confirm: true }, key('Enter'), 'button')).toBeNull()
  })

  it('closes the help only on Escape or ?', () => {
    const layers = { ...GRID, help: true }
    expect(routeKey(layers, key('Escape'), 'other')).toEqual({ layer: 'help', action: 'close' })
    expect(routeKey(layers, key('?'), 'other')).toEqual({ layer: 'help', action: 'close' })
    for (const k of ['ArrowLeft', 'h', 'j', 'k', 'l', 'Enter', 'o', 'i', '/', 'n', 'x']) {
      expect(routeKey(layers, key(k), 'other')).toBeNull()
    }
  })

  it('cancels the new session dialog on Escape only, whatever has the focus', () => {
    const layers = { ...GRID, newSession: true }
    for (const target of ['text', 'button', 'other'] as const) {
      expect(routeKey(layers, key('Escape'), target)).toEqual({ layer: 'newSession', action: 'cancel' })
      for (const k of [...GRID_KEYS, 'y']) expect(routeKey(layers, key(k), target)).toBeNull()
    }
    expect(routeKey(layers, key('Escape', { ctrlKey: true }), 'text')).toBeNull()
  })

  it('full screen wins over the new session dialog', () => {
    expect(routeKey({ ...GRID, fullscreen: true, newSession: true }, key('Escape'), 'text')).toBeNull()
  })

  it('a confirmation dialog wins over the help', () => {
    expect(routeKey({ ...GRID, confirm: true, help: true }, key('?'), 'other')).toBeNull()
  })
})

describe('routeKey with modifiers on dialogs', () => {
  const CONFIRM: KeyLayers = { ...GRID, confirm: true }
  const HELP: KeyLayers = { ...GRID, help: true }

  it('never accepts a confirmation with a modified key', () => {
    for (const mods of [{ ctrlKey: true }, { altKey: true }, { metaKey: true }]) {
      expect(routeKey(CONFIRM, key('y', mods), 'other')).toBeNull()
      expect(routeKey(CONFIRM, key('Enter', mods), 'other')).toBeNull()
    }
    expect(routeKey(CONFIRM, key('y'), 'other')).toEqual({ layer: 'confirm', action: 'accept' })
  })

  it('closes the help only with an unmodified key', () => {
    expect(routeKey(HELP, key('Escape', { ctrlKey: true }), 'other')).toBeNull()
    expect(routeKey(HELP, key('Escape'), 'other')).toEqual({ layer: 'help', action: 'close' })
  })
})
