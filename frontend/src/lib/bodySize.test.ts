import { describe, expect, it } from 'vitest'
import { bodySize, fitBody, fitFont } from './bodySize'

describe('bodySize', () => {
  const cell = { width: 6.6, height: 13 }

  it('fits whole cells into the body', () => {
    expect(bodySize(660, 130, cell)).toEqual({ cols: 100, rows: 10 })
    expect(bodySize(666, 142, cell)).toEqual({ cols: 100, rows: 10 })
  })

  it('never returns less than one cell', () => {
    expect(bodySize(2, 3, cell)).toEqual({ cols: 1, rows: 1 })
    expect(bodySize(0, 0, cell)).toEqual({ cols: 1, rows: 1 })
  })
})

describe('fitFont', () => {
  const ref = 6.6 // cell width at 11 px

  it('keeps 11 px when the pane fits exactly', () => {
    expect(fitFont(80, 528, ref, 11)).toBe(11)
  })

  it('never goes above the maximum for a narrow pane', () => {
    expect(fitFont(40, 528, ref, 11)).toBe(11)
  })

  it('shrinks in half-pixel steps between 11 and 6 px', () => {
    expect(fitFont(212, 1000, ref, 11)).toBe(7.5)
    expect(fitFont(80, 527, ref, 11)).toBe(10.5)
    expect(fitFont(100, 500, ref, 11)).toBe(8)
  })

  it('stops at 6 px when the pane does not fit', () => {
    expect(fitFont(212, 600, ref, 11)).toBe(6)
    expect(fitFont(100, 360, ref, 11)).toBe(6)
  })

  it('reaches the minimum exactly', () => {
    expect(fitFont(100, 360 + 36, ref, 11)).toBe(6.5)
    expect(fitFont(100, 360, ref, 11, 6)).toBe(6)
  })

  it('is not thrown off by floating-point error on an exact fit', () => {
    expect(fitFont(21, 147, 7.7, 11)).toBe(10)
  })

  it('honours custom bounds', () => {
    expect(fitFont(10, 1000, ref, 11, 6, 9)).toBe(9)
    expect(fitFont(100, 100, ref, 11, 8)).toBe(8)
  })
})

describe('fitBody', () => {
  const cell = { width: 6.6, height: 13 } // at 11 px

  it('takes the pane width as columns and rows from the height at that font', () => {
    expect(fitBody(212, 1000, 300, cell, 11)).toEqual({ fontSize: 7.5, cols: 212, rows: 33 })
  })

  it('keeps 11 px and the pane columns when the pane is narrower than the card', () => {
    expect(fitBody(40, 528, 130, cell, 11)).toEqual({ fontSize: 11, cols: 40, rows: 10 })
  })

  it('keeps the pane columns at 6 px even when they do not fit', () => {
    expect(fitBody(212, 600, 130, cell, 11)).toEqual({ fontSize: 6, cols: 212, rows: 18 })
  })

  it('fills the body at 11 px when the pane width is unknown', () => {
    expect(fitBody(null, 660, 130, cell, 11)).toEqual({ fontSize: 11, cols: 100, rows: 10 })
  })

  it('never returns less than one row', () => {
    expect(fitBody(80, 528, 3, cell, 11).rows).toBe(1)
  })
})
