import { describe, expect, it } from 'vitest'
import { frameClass } from './frame'

describe('frameClass', () => {
  it('maps each state to its class', () => {
    for (const state of ['waiting', 'busy', 'background', 'idle']) {
      expect(frameClass(state, false)).toBe(`frame-${state}`)
    }
  })

  it('falls back to unknown', () => {
    expect(frameClass('unknown', false)).toBe('frame-unknown')
    expect(frameClass('whatever', false)).toBe('frame-unknown')
  })

  it('marks the selected card', () => {
    expect(frameClass('busy', true)).toBe('frame-busy selected')
  })
})
