import { describe, expect, it } from 'vitest'
import { reconcileSelection } from './selection'

describe('reconcileSelection', () => {
  it('keeps the selected PID when it is still there, wherever it moved', () => {
    expect(reconcileSelection([1, 2, 3], [3, 1, 2], 2)).toBe(2)
  })

  it('selects the first card when nothing was selected', () => {
    expect(reconcileSelection([], [7, 8], null)).toBe(7)
  })

  it('takes the neighbour at the same position when the selected card disappears', () => {
    expect(reconcileSelection([1, 2, 3], [1, 3], 2)).toBe(3)
    expect(reconcileSelection([1, 2, 3], [1, 2], 3)).toBe(2)
    expect(reconcileSelection([1, 2, 3], [2, 3], 1)).toBe(2)
  })

  it('selects nothing when no card is left', () => {
    expect(reconcileSelection([1], [], 1)).toBeNull()
  })

  it('selects the first card when the selected PID was unknown', () => {
    expect(reconcileSelection([1], [5, 6], 9)).toBe(5)
  })
})
