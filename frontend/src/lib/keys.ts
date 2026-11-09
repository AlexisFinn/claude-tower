import type { Direction } from './navigation'

/** The parts of a keyboard event the key maps read. */
export interface KeyLike {
  type?: string
  key: string
  ctrlKey: boolean
  shiftKey: boolean
  altKey: boolean
  metaKey: boolean
}

/** An action triggered from the grid. */
export type GridAction =
  | { kind: 'move'; dir: Direction }
  | { kind: 'focus' | 'fullscreen' | 'prompt' | 'new' | 'kill' | 'help' }

const MOVES: Record<string, Direction> = {
  ArrowLeft: 'left',
  h: 'left',
  ArrowRight: 'right',
  l: 'right',
  ArrowUp: 'up',
  k: 'up',
  ArrowDown: 'down',
  j: 'down',
}

const SIMPLE: Record<string, GridAction> = {
  Enter: { kind: 'focus' },
  o: { kind: 'fullscreen' },
  i: { kind: 'prompt' },
  '/': { kind: 'prompt' },
  n: { kind: 'new' },
  x: { kind: 'kill' },
  '?': { kind: 'help' },
}

/** Returns the grid action of a key press, or null when the key has none. */
export function gridKeyAction(e: KeyLike): GridAction | null {
  if (e.ctrlKey || e.altKey || e.metaKey) return null
  const dir = MOVES[e.key]
  if (dir) return { kind: 'move', dir }
  return SIMPLE[e.key] ?? null
}

/** An action triggered from the full-screen terminal instead of being sent to the session. */
export type TerminalKeyAction = 'exit' | 'newline' | 'copy' | 'paste'

/** Returns the action of a key press in the full-screen terminal, or null to pass it on. */
export function terminalKeyAction(e: KeyLike): TerminalKeyAction | null {
  if (e.altKey || e.metaKey) return null
  const key = e.key.toLowerCase()
  if (e.ctrlKey && e.shiftKey) {
    if (key === 'q') return 'exit'
    if (key === 'c') return 'copy'
    if (key === 'v') return 'paste'
    return null
  }
  if (!e.ctrlKey && e.shiftKey && e.key === 'Enter') return 'newline'
  return null
}
