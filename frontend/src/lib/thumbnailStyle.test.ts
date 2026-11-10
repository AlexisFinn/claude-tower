import { describe, expect, it } from 'vitest'
import { DEFAULT_FG } from './termPalette'
import { saveStyleIndex, savedStyleIndex, strokeWidth, textColor, THUMBNAIL_STYLES } from './thumbnailStyle'

const heavy = { heavy: true, direct: false }
const plain = { heavy: false, direct: false }

describe('textColor', () => {
  it('keeps every color without the heavy style', () => {
    expect(textColor('#30343c', null, plain)).toBe('#30343c')
  })

  it('lifts a dark color on the dark background', () => {
    const lifted = textColor('#30343c', null, heavy)
    expect(lifted).not.toBe('#30343c')
    expect(parseInt(lifted.slice(1, 3), 16)).toBeGreaterThan(0x30)
  })

  it('keeps a light color, and any color on a light background', () => {
    expect(textColor(DEFAULT_FG, null, heavy)).toBe(DEFAULT_FG)
    expect(textColor('#30343c', '#e7ecf3', heavy)).toBe('#30343c')
  })
})

describe('strokeWidth', () => {
  it('outlines only with the heavy style, in proportion to the font size', () => {
    expect(strokeWidth(12, plain)).toBe(0)
    expect(strokeWidth(24, heavy)).toBeCloseTo(2 * strokeWidth(12, heavy))
    expect(strokeWidth(12, heavy)).toBeGreaterThan(0)
  })
})

describe('saved style', () => {
  it('reads back a saved index and falls back to the first style', () => {
    const store = new Map<string, string>()
    const storage = { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v) }
    expect(savedStyleIndex(storage)).toBe(0)
    saveStyleIndex(storage, 2)
    expect(savedStyleIndex(storage)).toBe(2)
    saveStyleIndex(storage, THUMBNAIL_STYLES.length)
    expect(savedStyleIndex(storage)).toBe(0)
    expect(savedStyleIndex(undefined)).toBe(0)
  })

  it('ignores a storage that refuses to save', () => {
    expect(() => saveStyleIndex({ setItem: () => { throw new Error('full') } }, 1)).not.toThrow()
  })
})
