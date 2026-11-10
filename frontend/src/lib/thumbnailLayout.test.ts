import { describe, expect, it } from 'vitest'
import { cellBounds } from './thumbnailLayout'

describe('cellBounds', () => {
  it('shares the edges between neighbours', () => {
    for (let i = 0; i < 50; i++) {
      expect(cellBounds(i, 3.37)[1]).toBe(cellBounds(i + 1, 3.37)[0])
    }
  })

  it('spans exactly the rounded grid width', () => {
    expect(cellBounds(0, 3.37)[0]).toBe(0)
    expect(cellBounds(199, 3.37)[1]).toBe(Math.round(200 * 3.37))
  })

  it('gives whole-number cells of that size', () => {
    expect(cellBounds(2, 10)).toEqual([20, 30])
  })
})
