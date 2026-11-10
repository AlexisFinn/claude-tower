/** Keys of a keyboard event that the quit shortcut looks at. */
export type QuitKey = Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'shiftKey' | 'altKey' | 'metaKey'>

/**
 * Reports whether a key asks to quit the application: Ctrl+Q, when neither
 * the fullscreen view, where it belongs to the session, nor a dialog is open.
 */
export function isQuitShortcut(e: QuitKey, blocked: boolean): boolean {
  return !blocked && e.ctrlKey && !e.shiftKey && !e.altKey && !e.metaKey && e.key.toLowerCase() === 'q'
}
