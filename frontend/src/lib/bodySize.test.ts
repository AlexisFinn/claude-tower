import { describe, expect, it } from 'vitest'
import { bodyGeometry, FIT_FONT, fitGrid, MAX_FIT, MIN_FIT, sameGeometry, sameSize } from './bodySize'

const ratio = { width: 0.6, height: 1.2 }

describe('bodyGeometry', () => {
  it('fills the body width with the pane columns and takes the rows from the height', () => {
    const g = bodyGeometry(100, null, 600, 300, ratio)
    expect(g.cols).toBe(100)
    expect(g.font).toBeCloseTo(10, 10)
    expect(g.cellWidth).toBeCloseTo(6, 10)
    expect(g.cellHeight).toBeCloseTo(12, 10)
    expect(g.rows).toBe(25)
  })

  it('stops at a 16 px font and leaves the right side empty', () => {
    const g = bodyGeometry(40, null, 2000, 300, ratio)
    expect(g.font).toBe(16)
    expect(g.cols).toBe(40)
    expect(g.cellWidth * g.cols).toBeLessThan(2000)
    expect(g.rows).toBe(15)
  })

  it('shows an 11 px font filling the width when the pane width is unknown', () => {
    const g = bodyGeometry(null, null, 660, 130, ratio)
    expect(g.font).toBe(11)
    expect(g.cols).toBe(100)
    expect(g.rows).toBe(9)
  })

  it('ignores the pane rows when they are not positive', () => {
    expect(bodyGeometry(100, 0, 600, 300, ratio).rows).toBe(25)
  })

  it('caps the rows at the pane height', () => {
    expect(bodyGeometry(212, 50, 300, 1000, ratio).rows).toBe(50)
    expect(bodyGeometry(212, 50, 300, 100, ratio).rows).toBeLessThan(50)
    expect(bodyGeometry(212, null, 300, 1000, ratio).rows).toBeGreaterThan(50)
  })

  it('keeps the pane columns on a very small card', () => {
    const g = bodyGeometry(212, null, 40, 20, ratio)
    expect(g.cols).toBe(212)
    expect(g.cellWidth * g.cols).toBeCloseTo(40, 10)
  })

  it('never returns less than one row or one column', () => {
    expect(bodyGeometry(80, null, 528, 3, ratio).rows).toBe(1)
    expect(bodyGeometry(null, null, 1, 3, ratio)).toMatchObject({ cols: 1, rows: 1 })
    expect(bodyGeometry(null, null, 0, 0, ratio)).toMatchObject({ cols: 1, rows: 1 })
  })

  it('never returns a negative font for a negative width', () => {
    expect(bodyGeometry(80, null, -5, 100, ratio).font).toBe(0)
  })
})

describe('sameGeometry', () => {
  it('compares every field', () => {
    const g = bodyGeometry(100, 40, 600, 300, ratio)
    expect(sameGeometry(g, { ...g })).toBe(true)
    expect(sameGeometry(g, { ...g, rows: g.rows - 1 })).toBe(false)
    expect(sameGeometry(g, { ...g, font: g.font + 1 })).toBe(false)
    expect(sameGeometry(g, { ...g, cellHeight: g.cellHeight + 1 })).toBe(false)
  })

  it('handles null', () => {
    const g = bodyGeometry(100, 40, 600, 300, ratio)
    expect(sameGeometry(null, null)).toBe(true)
    expect(sameGeometry(g, null)).toBe(false)
    expect(sameGeometry(null, g)).toBe(false)
  })
})

describe('fitGrid', () => {
  const ratio = { width: 0.6, height: 1.2 }

  it('fills the body at the fit font', () => {
    // A cell is 8.4 x 16.8 px at 14 px.
    expect(fitGrid(1400, 432, ratio)).toEqual({ cols: 166, rows: 25 })
  })

  it('gives the bodyGeometry font back for that many columns', () => {
    const size = fitGrid(1400, 432, ratio)
    const g = bodyGeometry(size.cols, size.rows, 1400, 432, ratio)
    expect(g.font).toBeCloseTo(FIT_FONT, 0)
    expect(g.rows).toBe(size.rows)
  })

  it('never goes below the smallest usable terminal nor above the largest', () => {
    expect(fitGrid(100, 50, ratio).cols).toBe(MIN_FIT.cols)
    // A 900 px body gives 107 columns at 14 px: it gets the 144 that dock the tasks pane, with a smaller font.
    expect(fitGrid(900, 432, ratio)).toEqual({ cols: 144, rows: 34 })
    expect(fitGrid(0, -5, ratio)).toEqual(MIN_FIT)
    expect(fitGrid(1e6, 1e6, ratio)).toEqual({ cols: MAX_FIT, rows: MAX_FIT })
  })

  it('asks for the rows bodyGeometry shows at the columns asked', () => {
    for (const [w, h] of [[900, 432], [1200, 432], [1913, 700], [640, 300]]) {
      const size = fitGrid(w, h, ratio)
      expect(bodyGeometry(size.cols, size.rows, w, h, ratio).rows).toBe(size.rows)
    }
  })
})

describe('sameSize', () => {
  it('compares the columns and the rows', () => {
    expect(sameSize({ cols: 80, rows: 24 }, { cols: 80, rows: 24 })).toBe(true)
    expect(sameSize({ cols: 80, rows: 24 }, { cols: 81, rows: 24 })).toBe(false)
    expect(sameSize({ cols: 80, rows: 24 }, { cols: 80, rows: 25 })).toBe(false)
  })

  it('is false for an unknown size', () => {
    expect(sameSize(undefined, { cols: 80, rows: 24 })).toBe(false)
  })
})
