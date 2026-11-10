import { describe, expect, it } from 'vitest'
import { bodySize, displayScale, fitBody, renderFont } from './bodySize'

describe('bodySize', () => {
  const cell = { width: 6.6, height: 13 }

  it('fits whole cells into the body', () => {
    expect(bodySize(660, 130, cell)).toEqual({ cols: 100, rows: 10 })
    expect(bodySize(666, 142, cell)).toEqual({ cols: 100, rows: 10 })
    expect(bodySize(0.3 * 3, 0.1 * 3, { width: 0.3, height: 0.1 })).toEqual({ cols: 3, rows: 3 })
  })

  it('never returns less than one cell', () => {
    expect(bodySize(2, 3, cell)).toEqual({ cols: 1, rows: 1 })
    expect(bodySize(0, 0, cell)).toEqual({ cols: 1, rows: 1 })
  })
})

describe('renderFont', () => {
  const ratio = { width: 0.6, height: 1.2 }

  it('renders at twice the displayed font', () => {
    expect(renderFont(100, 600, ratio)).toBe(20) // displayed 10 px
    expect(renderFont(100, 480, ratio)).toBe(16) // displayed 8 px
  })

  it('rounds to whole pixels', () => {
    expect(renderFont(100, 500, ratio)).toBe(17)
    expect(renderFont(100, 490, ratio)).toBe(16)
  })

  it('stays at the minimum for tiny text', () => {
    expect(renderFont(212, 400, ratio)).toBe(10)
    expect(renderFont(80, 1, ratio)).toBe(10)
    expect(renderFont(80, 0, ratio)).toBe(10)
  })

  it('stays at the maximum when the display font is capped', () => {
    expect(renderFont(40, 2000, ratio)).toBe(28)
    expect(renderFont(80, 600, ratio)).toBe(25)
  })

  it('uses the default display font without a pane width', () => {
    expect(renderFont(null, 1000, ratio)).toBe(22)
    expect(renderFont(null, 10, ratio)).toBe(22)
  })
})

describe('displayScale', () => {
  it('makes the terminal as wide as the body', () => {
    expect(displayScale(20, 500, 1000)).toBe(0.5)
    expect(displayScale(20, 300, 1000)).toBe(0.3)
  })

  it('never enlarges the terminal', () => {
    expect(displayScale(10, 2000, 1000)).toBe(1)
  })

  it('caps the displayed font at 16 px', () => {
    expect(displayScale(28, 5000, 1000)).toBeCloseTo(16 / 28, 10)
    expect(displayScale(20, 5000, 1000)).toBe(0.8)
  })

  it('shows the default 11 px font without a natural width', () => {
    expect(displayScale(22, 800, null)).toBe(0.5)
    expect(displayScale(22, 800, 0)).toBe(0.5)
  })

  it('never returns a negative scale', () => {
    expect(displayScale(20, -5, 1000)).toBe(0)
  })
})

describe('fitBody', () => {
  const cell = { width: 12, height: 24 } // at 20 px

  it('fills the body width with the pane columns and takes the rows from the height', () => {
    expect(fitBody(100, 600, 300, 20, cell, 1200)).toEqual({ scale: 0.5, cols: 100, rows: 25 })
  })

  it('uses the measured natural width rather than the cell width', () => {
    expect(fitBody(100, 600, 300, 20, cell, 1250)).toEqual({ scale: 0.48, cols: 100, rows: 26 })
  })

  it('estimates the natural width from the cell when it could not be measured', () => {
    expect(fitBody(100, 600, 300, 20, cell, null)).toEqual({ scale: 0.5, cols: 100, rows: 25 })
  })

  it('stops at a 16 px displayed font and leaves the right side empty', () => {
    expect(fitBody(40, 2000, 300, 28, { width: 16.8, height: 33.6 }, 672)).toEqual({
      scale: 16 / 28,
      cols: 40,
      rows: 15,
    })
  })

  it('fills the body at the default font when the pane width is unknown', () => {
    expect(fitBody(null, 660, 130, 22, { width: 13.2, height: 26 }, null)).toEqual({
      scale: 0.5,
      cols: 100,
      rows: 10,
    })
  })

  it('ignores the natural width when the pane width is unknown', () => {
    expect(fitBody(null, 660, 130, 22, { width: 13.2, height: 26 }, 5000).cols).toBe(100)
  })

  it('keeps the pane columns on a very small card', () => {
    const fit = fitBody(212, 40, 20, 10, { width: 6, height: 12 }, 1272)
    expect(fit.cols).toBe(212)
    expect(fit.scale).toBeCloseTo(40 / 1272, 10)
    expect(fit.rows).toBe(53)
  })

  it('never returns less than one row or one column', () => {
    expect(fitBody(80, 528, 3, 20, cell, 960).rows).toBe(1)
    expect(fitBody(null, 1, 3, 22, cell, null)).toMatchObject({ cols: 1, rows: 1 })
  })
})
