import { describe, expect, it } from 'vitest'
import { emptyPrompt, sendSucceeded, type PromptState } from './promptState'

/** Holds the state the way the prompt bar does: reactive state that the text area writes into. */
function barState() {
  let state = $state<PromptState>({ ...emptyPrompt })
  return {
    get: () => state,
    type: (text: string) => (state.text = text),
    succeed: () => (state = sendSucceeded()),
  }
}

describe('prompt state as reactive state', () => {
  it('clears the text after a send even though the bar wrote into its state', () => {
    const bar = barState()
    bar.type('fais ceci')
    expect(bar.get().text).toBe('fais ceci')
    bar.succeed()
    expect(bar.get().text).toBe('')
  })

  it('keeps the shared empty prompt untouched when the bar state is written', () => {
    barState().type('fais ceci')
    expect(emptyPrompt.text).toBe('')
    expect(sendSucceeded().text).toBe('')
  })

  it('returns a new object on each success', () => {
    expect(sendSucceeded()).not.toBe(sendSucceeded())
    expect(sendSucceeded()).not.toBe(emptyPrompt)
  })
})
