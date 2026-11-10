import { describe, expect, it } from 'vitest'
import {
  beginSend,
  canSend,
  emptyPrompt,
  promptAvailability,
  sendFailed,
  sendSucceeded,
} from './promptState'

describe('prompt state', () => {
  const typed = { ...emptyPrompt, text: 'fais ceci' }

  it('only sends non-blank text, once at a time', () => {
    expect(canSend(emptyPrompt)).toBe(false)
    expect(canSend({ ...emptyPrompt, text: '  \n' })).toBe(false)
    expect(canSend(typed)).toBe(true)
    expect(canSend(beginSend(typed))).toBe(false)
  })

  it('sends files without text', () => {
    expect(canSend({ ...emptyPrompt, files: ['/a.txt'] })).toBe(true)
    expect(canSend(beginSend({ ...emptyPrompt, files: ['/a.txt'] }))).toBe(false)
    expect(canSend({ ...emptyPrompt, text: ' ', files: [] })).toBe(false)
  })

  it('disables the bar while sending', () => {
    const sending = beginSend(typed)
    expect(sending.sending).toBe(true)
    expect(promptAvailability({ CanPrompt: true, PromptReason: '' }, sending)).toEqual({
      disabled: true,
      reason: '',
    })
  })

  it('clears the text and the files after a success', () => {
    expect(sendSucceeded()).toEqual({ text: '', files: [], sending: false, error: '' })
  })

  it('keeps the text and shows the error after a failure', () => {
    const failed = sendFailed(beginSend(typed), 'session morte')
    expect(failed).toEqual({ text: 'fais ceci', files: [], sending: false, error: 'session morte' })
    expect(canSend(failed)).toBe(true)
  })

  it('keeps the files after a failure', () => {
    const withFiles = { ...typed, files: ['/a.txt', '/b.txt'] }
    expect(sendFailed(beginSend(withFiles), 'oups').files).toEqual(['/a.txt', '/b.txt'])
    expect(sendSucceeded().files).toEqual([])
  })

  it('clears a previous error when a new send starts', () => {
    expect(beginSend(sendFailed(typed, 'oups')).error).toBe('')
  })

  it('explains why a session cannot take a prompt', () => {
    expect(
      promptAvailability({ CanPrompt: false, PromptReason: 'session hors tmux' }, typed),
    ).toEqual({ disabled: true, reason: 'session hors tmux' })
    expect(promptAvailability(null, typed).disabled).toBe(true)
    expect(promptAvailability({ CanPrompt: true, PromptReason: '' }, typed)).toEqual({
      disabled: false,
      reason: '',
    })
  })
})
