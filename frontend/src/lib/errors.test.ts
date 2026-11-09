import { describe, expect, it } from 'vitest'
import { errorMessage } from './errors'

describe('errorMessage', () => {
  it('reads strings, errors and other values', () => {
    expect(errorMessage('boom')).toBe('boom')
    expect(errorMessage(new Error('bang'))).toBe('bang')
    expect(errorMessage(42)).toBe('42')
  })
})
