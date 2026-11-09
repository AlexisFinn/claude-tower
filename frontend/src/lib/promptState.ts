/** The state of the prompt bar. */
export interface PromptState {
  text: string
  sending: boolean
  error: string
}

/** What the prompt bar knows about the selected card. */
export interface PromptTarget {
  CanPrompt: boolean
  PromptReason: string
}

/** The state of an empty prompt bar. Frozen: copy it before using it as mutable state. */
export const emptyPrompt: Readonly<PromptState> = Object.freeze({ text: '', sending: false, error: '' })

/** Reports whether the text can be sent now. */
export function canSend(s: PromptState): boolean {
  return !s.sending && s.text.trim() !== ''
}

/** Returns the state while a message is being sent: the bar is disabled. */
export function beginSend(s: PromptState): PromptState {
  return { ...s, sending: true, error: '' }
}

/** Returns the state after a successful send: the text is cleared. */
export function sendSucceeded(): PromptState {
  return { ...emptyPrompt }
}

/** Returns the state after a failed send: the text is kept, the error shown. */
export function sendFailed(s: PromptState, message: string): PromptState {
  return { ...s, sending: false, error: message }
}

/** Tells whether the bar is disabled, and why when the session cannot take a prompt. */
export function promptAvailability(
  target: PromptTarget | null,
  s: PromptState,
): { disabled: boolean; reason: string } {
  if (target === null) return { disabled: true, reason: 'aucune session sélectionnée' }
  if (!target.CanPrompt) return { disabled: true, reason: target.PromptReason }
  return { disabled: s.sending, reason: '' }
}
