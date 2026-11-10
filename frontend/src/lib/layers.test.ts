import { describe, expect, it } from 'vitest'
import type { core } from '../../wailsjs/go/models'
import { closeLayer, confirmation, keyLayers, layerOf, openLayer, type Layers } from './layers'
import { routeKey } from './keyRouting'

const card = (pid: number) => ({ PID: pid }) as core.Card
const key = (k: string) => ({ key: k, ctrlKey: false, shiftKey: false, altKey: false, metaKey: false })

describe('layers', () => {
  it('opens a layer once per kind, the last one replacing the first', () => {
    let layers: Layers = []
    layers = openLayer(layers, { kind: 'kill', card: card(1) })
    layers = openLayer(layers, { kind: 'kill', card: card(2) })
    expect(layers).toHaveLength(1)
    expect(layerOf(layers, 'kill')?.card.PID).toBe(2)
  })

  it('closes a layer and leaves the others open', () => {
    let layers: Layers = []
    layers = openLayer(layers, { kind: 'kill', card: card(1) })
    layers = openLayer(layers, { kind: 'quit' })
    layers = closeLayer(layers, 'quit')
    expect(layerOf(layers, 'quit')).toBeUndefined()
    expect(layerOf(layers, 'kill')?.card.PID).toBe(1)
  })

  it('returns the same list when the layer is not open', () => {
    const layers: Layers = [{ kind: 'help' }]
    expect(closeLayer(layers, 'quit')).toBe(layers)
  })
})

describe('keyLayers', () => {
  it('has no layer on the grid', () => {
    expect(keyLayers([])).toEqual({ fullscreen: false, confirm: false, newSession: false, help: false })
  })

  it('maps each kind to its layer, a kill or a quit confirmation both being a confirmation', () => {
    expect(keyLayers([{ kind: 'fullscreen', card: card(1) }])).toMatchObject({ fullscreen: true, confirm: false })
    expect(keyLayers([{ kind: 'help' }])).toMatchObject({ help: true, confirm: false })
    expect(keyLayers([{ kind: 'newSession' }])).toMatchObject({ newSession: true, confirm: false })
    expect(keyLayers([{ kind: 'kill', card: card(1) }])).toMatchObject({ confirm: true })
    expect(keyLayers([{ kind: 'quit' }])).toMatchObject({ confirm: true })
  })
})

describe('quit confirmation', () => {
  const QUIT: Layers = [{ kind: 'quit' }]
  const BOTH: Layers = [{ kind: 'kill', card: card(1) }, { kind: 'quit' }]

  it('is answered by the confirmation keys', () => {
    expect(routeKey(keyLayers(QUIT), key('y'), 'other')).toEqual({ layer: 'confirm', action: 'accept' })
    expect(routeKey(keyLayers(QUIT), key('Enter'), 'other')).toEqual({ layer: 'confirm', action: 'accept' })
    expect(routeKey(keyLayers(QUIT), key('Escape'), 'other')).toEqual({ layer: 'confirm', action: 'cancel' })
    expect(routeKey(keyLayers(QUIT), key('n'), 'other')).toEqual({ layer: 'confirm', action: 'cancel' })
  })

  it('lets no grid key act', () => {
    for (const k of ['x', 'o', '?', 'ArrowLeft']) expect(routeKey(keyLayers(QUIT), key(k), 'other')).toBeNull()
  })

  it('is answered first when a kill confirmation is pending', () => {
    expect(confirmation(BOTH)).toBe('quit')
    expect(confirmation(closeLayer(BOTH, 'quit'))).toBe('kill')
    expect(confirmation(closeLayer(BOTH, 'kill'))).toBe('quit')
    expect(confirmation([])).toBeNull()
  })
})
