import { describe, expect, it } from 'vitest'
import { DEFAULT_FG, mixColors, paletteColor, rgbHex } from './termPalette'

describe('rgbHex', () => {
  it('pads and lowercases', () => {
    expect(rgbHex(0x000a0b)).toBe('#000a0b')
    expect(rgbHex(0xff00ff)).toBe('#ff00ff')
    expect(rgbHex(0)).toBe('#000000')
  })

  it('ignores bits above 24', () => {
    expect(rgbHex(0x1ff0000)).toBe('#ff0000')
  })
})

describe('paletteColor', () => {
  it('has 16 distinct readable ANSI colors', () => {
    const colors = Array.from({ length: 16 }, (_, i) => paletteColor(i))
    expect(new Set(colors).size).toBe(16)
    expect(colors[15]).toBe('#f2f5fa')
  })

  it('computes the 6x6x6 cube', () => {
    expect(paletteColor(16)).toBe('#000000')
    expect(paletteColor(21)).toBe('#0000ff')
    expect(paletteColor(196)).toBe('#ff0000')
    expect(paletteColor(46)).toBe('#00ff00')
    expect(paletteColor(231)).toBe('#ffffff')
    expect(paletteColor(16 + 36 * 1 + 6 * 2 + 3)).toBe('#5f87af')
  })

  it('computes the grays', () => {
    expect(paletteColor(232)).toBe('#080808')
    expect(paletteColor(244)).toBe('#808080')
    expect(paletteColor(255)).toBe('#eeeeee')
  })

  it('falls back to the default text color outside the palette', () => {
    expect(paletteColor(256)).toBe(DEFAULT_FG)
    expect(paletteColor(-1)).toBe(DEFAULT_FG)
  })
})

describe('mixColors', () => {
  it('returns the ends and the middle', () => {
    expect(mixColors('#000000', '#ffffff', 0)).toBe('#000000')
    expect(mixColors('#000000', '#ffffff', 1)).toBe('#ffffff')
    expect(mixColors('#000000', '#ffffff', 0.5)).toBe('#808080')
    expect(mixColors('#ff0000', '#0000ff', 0.5)).toBe('#800080')
  })
})
