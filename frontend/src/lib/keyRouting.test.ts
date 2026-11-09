import { describe, expect, it } from 'vitest'
import type { KeyLike } from './keys'
import { routeKey, type KeyLayers } from './keyRouting'

const GRID: KeyLayers = { fullscreen: false, confirmKill: false, help: false }

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
    expect(routeKey({ fullscreen: true, confirmKill: true, help: true }, key('y'), 'other')).toBeNull()
  })

  it('never lets a grid key act under the kill dialog', () => {
    const layers = { ...GRID, confirmKill: true }
    for (const k of ['ArrowLeft', 'h', 'j', 'k', 'l', 'o', 'i', '/', 'n', 'x', '?']) {
      const route = routeKey(layers, key(k), 'other')
      expect(route === null || route.layer === 'confirm').toBe(true)
    }
    expect(routeKey(layers, key('x'), 'other')).toBeNull()
    expect(routeKey(layers, key('?'), 'other')).toBeNull()
  })

  it('accepts or cancels the kill dialog', () => {
    const layers = { ...GRID, confirmKill: true }
    expect(routeKey(layers, key('y'), 'other')).toEqual({ layer: 'confirm', action: 'accept' })
    expect(routeKey(layers, key('Enter'), 'other')).toEqual({ layer: 'confirm', action: 'accept' })
    expect(routeKey(layers, key('Escape'), 'other')).toEqual({ layer: 'confirm', action: 'cancel' })
    expect(routeKey(layers, key('n'), 'other')).toEqual({ layer: 'confirm', action: 'cancel' })
  })

  it('leaves Enter to a focused button of the kill dialog', () => {
    expect(routeKey({ ...GRID, confirmKill: true }, key('Enter'), 'button')).toBeNull()
  })

  it('closes the help only on Escape or ?', () => {
    const layers = { ...GRID, help: true }
    expect(routeKey(layers, key('Escape'), 'other')).toEqual({ layer: 'help', action: 'close' })
    expect(routeKey(layers, key('?'), 'other')).toEqual({ layer: 'help', action: 'close' })
    for (const k of ['ArrowLeft', 'h', 'j', 'k', 'l', 'Enter', 'o', 'i', '/', 'n', 'x']) {
      expect(routeKey(layers, key(k), 'other')).toBeNull()
    }
  })

  it('the kill dialog wins over the help', () => {
    expect(routeKey({ fullscreen: false, confirmKill: true, help: true }, key('?'), 'other')).toBeNull()
  })
})

describe('routeKey with modifiers on dialogs', () => {
  const CONFIRM: KeyLayers = { fullscreen: false, confirmKill: true, help: false }
  const HELP: KeyLayers = { fullscreen: false, confirmKill: false, help: true }

  it('never accepts the stop confirmation with a modified key', () => {
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
