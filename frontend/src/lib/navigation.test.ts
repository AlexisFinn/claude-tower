import { describe, expect, it } from 'vitest'
import { layout } from './layout'
import { move } from './navigation'

describe('move', () => {
  const five = layout(5, 1800, 900).rects // 3 on top, 2 below

  it('moves within a row', () => {
    expect(move(five, 0, 'right')).toBe(1)
    expect(move(five, 2, 'left')).toBe(1)
  })

  it('stays at the edges', () => {
    expect(move(five, 0, 'left')).toBe(0)
    expect(move(five, 2, 'right')).toBe(2)
    expect(move(five, 0, 'up')).toBe(0)
    expect(move(five, 4, 'down')).toBe(4)
    expect(move(five, 4, 'right')).toBe(4)
  })

  it('goes down to the card below, then back up', () => {
    expect(move(five, 0, 'down')).toBe(3)
    expect(move(five, 1, 'down')).toBe(3)
    expect(move(five, 2, 'down')).toBe(4)
    expect(move(five, 3, 'up')).toBe(0)
    expect(move(five, 4, 'up')).toBe(2)
  })

  it('crosses an incomplete last row by position', () => {
    const three = layout(3, 1600, 900).rects // 2 on top, 1 full-width below
    expect(move(three, 0, 'down')).toBe(2)
    expect(move(three, 1, 'down')).toBe(2)
    expect(move(three, 2, 'up')).toBe(0)
    expect(move(three, 2, 'left')).toBe(2)
  })

  it('follows the geometry after a resize that changes the columns', () => {
    const wide = layout(4, 2000, 1000).rects // 2x2
    expect(move(wide, 0, 'right')).toBe(1)
    expect(move(wide, 1, 'down')).toBe(3)
    const narrow = layout(4, 600, 1000).rects // one column of four rows
    expect(move(narrow, 0, 'right')).toBe(0)
    expect(move(narrow, 1, 'down')).toBe(2)
    expect(move(narrow, 3, 'up')).toBe(2)
  })

  it('returns the index when there is no card', () => {
    expect(move([], 0, 'down')).toBe(0)
  })

  it('falls back to the first card for an out-of-range index', () => {
    expect(move(five, 99, 'down')).toBe(0)
  })
})
