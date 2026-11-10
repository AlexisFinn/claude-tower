import { describe, expect, it } from 'vitest'
import { changedRows } from './rowDiff'

describe('changedRows', () => {
  it('returns nothing when no signature changed', () => {
    expect(changedRows(['a', 'b', 'c'], ['a', 'b', 'c'])).toEqual([])
  })

  it('returns only the rows that changed', () => {
    expect(changedRows(['a', 'b', 'c', 'd'], ['a', 'x', 'c', 'y'])).toEqual([1, 3])
  })

  it('returns every row without a previous frame', () => {
    expect(changedRows([], ['a', 'b'])).toEqual([0, 1])
  })

  it('returns every row when the row count changed', () => {
    expect(changedRows(['a', 'b'], ['a', 'b', 'c'])).toEqual([0, 1, 2])
    expect(changedRows(['a', 'b', 'c'], ['a', 'b'])).toEqual([0, 1])
  })

  it('returns nothing for an empty frame', () => {
    expect(changedRows([], [])).toEqual([])
  })
})
