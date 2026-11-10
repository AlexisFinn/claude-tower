import type { core } from '../../wailsjs/go/models'
import type { KeyLayers } from './keyRouting'

/** A layer of the window that is open above the grid. */
export type Layer =
  | { kind: 'fullscreen'; card: core.Card }
  | { kind: 'help' }
  | { kind: 'newSession' }
  | { kind: 'kill'; card: core.Card }
  | { kind: 'quit' }

export type LayerKind = Layer['kind']

/**
 * The layers that are open, at most one of each kind. Several kinds can be
 * open together, for instance the quit confirmation above a pending kill
 * confirmation.
 */
export type Layers = readonly Layer[]

/** Returns the layer of a kind, or undefined when it is not open. */
export function layerOf<K extends LayerKind>(layers: Layers, kind: K): Extract<Layer, { kind: K }> | undefined {
  return layers.find((l): l is Extract<Layer, { kind: K }> => l.kind === kind)
}

/** Opens a layer, replacing the one of the same kind. */
export function openLayer(layers: Layers, layer: Layer): Layers {
  return [...layers.filter((l) => l.kind !== layer.kind), layer]
}

/** Closes the layer of a kind. */
export function closeLayer(layers: Layers, kind: LayerKind): Layers {
  return layers.some((l) => l.kind === kind) ? layers.filter((l) => l.kind !== kind) : layers
}

/** The confirmation a key answers: the quit confirmation shows above a kill confirmation. */
export function confirmation(layers: Layers): 'quit' | 'kill' | null {
  if (layerOf(layers, 'quit')) return 'quit'
  return layerOf(layers, 'kill') ? 'kill' : null
}

/** Describes the open layers to the key routing. */
export function keyLayers(layers: Layers): KeyLayers {
  return {
    fullscreen: layerOf(layers, 'fullscreen') !== undefined,
    confirm: confirmation(layers) !== null,
    newSession: layerOf(layers, 'newSession') !== undefined,
    help: layerOf(layers, 'help') !== undefined,
  }
}
