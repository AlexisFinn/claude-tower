import { describe, expect, it } from 'vitest'
import { summarizeStates } from './summary'

describe('summarizeStates', () => {
  it('returns nothing without sessions', () => {
    expect(summarizeStates([])).toEqual([])
  })

  it('counts per state, most urgent first, and skips empty states', () => {
    const cards = [
      { State: 'idle' },
      { State: 'busy' },
      { State: 'idle' },
      { State: 'waiting' },
      { State: 'idle' },
      { State: 'busy' },
      { State: 'background' },
    ]
    expect(summarizeStates(cards)).toEqual([
      { state: 'waiting', count: 1, label: 'attend' },
      { state: 'busy', count: 2, label: 'travaillent' },
      { state: 'background', count: 1, label: 'en fond' },
      { state: 'idle', count: 3, label: 'finies' },
    ])
  })

  it('uses the singular for one session and the plural otherwise', () => {
    expect(summarizeStates([{ State: 'waiting' }, { State: 'waiting' }])[0].label).toBe('attendent')
    expect(summarizeStates([{ State: 'idle' }])[0].label).toBe('finie')
  })

  it('groups unrecognised states as unknown', () => {
    expect(summarizeStates([{ State: 'weird' }, { State: '' }])).toEqual([
      { state: 'unknown', count: 2, label: 'inconnues' },
    ])
  })
})
