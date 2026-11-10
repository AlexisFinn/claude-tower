import { gridKeyAction, type GridAction, type KeyLike } from './keys'

/** The layers of the window that can be open above the grid. */
export interface KeyLayers {
  fullscreen: boolean
  confirm: boolean
  newSession: boolean
  help: boolean
}

/** What kind of element has the focus when a key is pressed. */
export type KeyTarget = 'text' | 'button' | 'other'

/** The layer that handles a key press and what it does with it. */
export type KeyRoute =
  | { layer: 'confirm'; action: 'accept' | 'cancel' }
  | { layer: 'newSession'; action: 'cancel' }
  | { layer: 'help'; action: 'close' }
  | { layer: 'grid'; action: GridAction }

/**
 * Returns which layer handles a key press, or null when the key is left to
 * the focused element. The top layer is the only one that reacts: grid keys
 * never act under a dialog, in full screen or while typing.
 */
export function routeKey(layers: KeyLayers, e: KeyLike, target: KeyTarget): KeyRoute | null {
  if (layers.fullscreen) return null
  const modified = e.ctrlKey || e.altKey || e.metaKey
  if (layers.confirm) {
    // A modified key never answers a confirmation, which stops a session or quits the application.
    if (modified) return null
    if ((e.key === 'Enter' && target !== 'button') || e.key === 'y') return { layer: 'confirm', action: 'accept' }
    if (e.key === 'Escape' || e.key === 'n') return { layer: 'confirm', action: 'cancel' }
    return null
  }
  if (layers.newSession) return !modified && e.key === 'Escape' ? { layer: 'newSession', action: 'cancel' } : null
  if (layers.help) return !modified && (e.key === 'Escape' || e.key === '?') ? { layer: 'help', action: 'close' } : null
  if (target === 'text') return null
  if (target === 'button' && e.key === 'Enter') return null
  const action = gridKeyAction(e)
  return action === null ? null : { layer: 'grid', action }
}
