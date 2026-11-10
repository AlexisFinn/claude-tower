import { describe, expect, it } from 'vitest'
import { DEFAULT_BG, DEFAULT_FG } from './termPalette'
import { outlines, strokeWidth, textColor } from './thumbnailInk'

describe('textColor', () => {
  it('lifts a dark color a quarter of the way to white on the default background', () => {
    expect(textColor('#202020', null)).toBe('#585858')
    expect(textColor('#202020', DEFAULT_BG)).toBe('#585858')
    expect(textColor('#202020', '#2d3440')).toBe('#585858')
  })

  it('keeps a color lighter than mid-gray', () => {
    expect(textColor(DEFAULT_FG, null)).toBe(DEFAULT_FG)
    expect(textColor('#a0a0a0', null)).toBe('#a0a0a0')
  })

  it('keeps any color on a colored or light background', () => {
    expect(textColor('#202020', '#cc0000')).toBe('#202020')
    expect(textColor('#202020', '#2d5a2d')).toBe('#202020')
    expect(textColor('#202020', DEFAULT_FG)).toBe('#202020')
  })

  it('keeps a color it cannot read', () => {
    expect(textColor('red', null)).toBe('red')
    expect(textColor('#202020', 'black')).toBe('#202020')
  })
})

describe('strokeWidth', () => {
  it('is a small fraction of the font size', () => {
    expect(strokeWidth(20)).toBeCloseTo(0.9)
  })
})

describe('outlines', () => {
  it('outlines narrow characters of the Basic Multilingual Plane only', () => {
    expect(outlines('a', false)).toBe(true)
    expect(outlines('é', false)).toBe(true)
    expect(outlines('世', true)).toBe(false)
    expect(outlines('😀', true)).toBe(false)
    expect(outlines('😀', false)).toBe(false)
  })
})
