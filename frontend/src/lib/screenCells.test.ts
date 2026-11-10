import { describe, expect, it } from 'vitest'
import { drawCell, readRow, type CellLike, type LineLike } from './screenCells'
import { DEFAULT_BG, DEFAULT_FG, paletteColor } from './termPalette'

interface Spec {
  ch?: string
  width?: number
  fg?: { palette: number } | { rgb: number }
  bg?: { palette: number } | { rgb: number }
  bold?: boolean
  italic?: boolean
  dim?: boolean
  underline?: boolean
  inverse?: boolean
  invisible?: boolean
}

function color(c: Spec['fg']) {
  if (!c) return { rgb: false, palette: false, value: 0 }
  return 'rgb' in c ? { rgb: true, palette: false, value: c.rgb } : { rgb: false, palette: true, value: c.palette }
}

function cell(spec: Spec): CellLike {
  const fg = color(spec.fg)
  const bg = color(spec.bg)
  return {
    getChars: () => spec.ch ?? '',
    getWidth: () => spec.width ?? 1,
    getFgColor: () => fg.value,
    getBgColor: () => bg.value,
    isFgRGB: () => fg.rgb,
    isBgRGB: () => bg.rgb,
    isFgPalette: () => fg.palette,
    isBgPalette: () => bg.palette,
    isBold: () => (spec.bold ? 1 : 0),
    isItalic: () => (spec.italic ? 1 : 0),
    isDim: () => (spec.dim ? 134217728 : 0),
    isUnderline: () => (spec.underline ? 1 : 0),
    isInverse: () => (spec.inverse ? 67108864 : 0),
    isInvisible: () => (spec.invisible ? 1 : 0),
  }
}

function line(specs: Spec[]): LineLike {
  return { getCell: (x) => (specs[x] ? cell(specs[x]) : undefined) }
}

describe('drawCell', () => {
  it('draws nothing for a blank cell on the default background', () => {
    expect(drawCell(0, cell({}))).toBeNull()
    expect(drawCell(0, cell({ ch: ' ' }))).toBeNull()
  })

  it('draws nothing for the second half of a wide character', () => {
    expect(drawCell(1, cell({ width: 0 }))).toBeNull()
  })

  it('uses the default text color and no background', () => {
    expect(drawCell(3, cell({ ch: 'a' }))).toEqual({
      col: 3,
      ch: 'a',
      wide: false,
      fg: DEFAULT_FG,
      bg: null,
      bold: false,
      italic: false,
      underline: false,
    })
  })

  it('resolves palette and RGB colors', () => {
    const c = drawCell(0, cell({ ch: 'x', fg: { palette: 196 }, bg: { rgb: 0x0a0b0c } }))
    expect(c?.fg).toBe(paletteColor(196))
    expect(c?.bg).toBe('#0a0b0c')
    expect(drawCell(0, cell({ ch: 'x', fg: { rgb: 0x112233 } }))?.fg).toBe('#112233')
    expect(drawCell(0, cell({ ch: 'x', fg: { palette: 4 } }))?.fg).toBe(paletteColor(4))
  })

  it('draws a blank cell that has a background', () => {
    expect(drawCell(0, cell({ bg: { palette: 2 } }))).toMatchObject({ ch: '', bg: paletteColor(2) })
  })

  it('draws a blank underlined cell', () => {
    expect(drawCell(0, cell({ underline: true }))).toMatchObject({ ch: '', underline: true })
  })

  it('swaps the colors of an inverse cell, using the defaults for the missing ones', () => {
    const plain = drawCell(0, cell({ ch: 'x', inverse: true }))
    expect(plain).toMatchObject({ fg: DEFAULT_BG, bg: DEFAULT_FG })
    const colored = drawCell(0, cell({ ch: 'x', inverse: true, fg: { palette: 1 }, bg: { palette: 4 } }))
    expect(colored).toMatchObject({ fg: paletteColor(4), bg: paletteColor(1) })
  })

  it('makes dim text darker than normal text', () => {
    const normal = drawCell(0, cell({ ch: 'x' }))
    const dim = drawCell(0, cell({ ch: 'x', dim: true }))
    expect(dim?.fg).not.toBe(normal?.fg)
    expect(dim?.fg).toBe('#82868b')
  })

  it('keeps the bold, italic and wide flags', () => {
    expect(drawCell(0, cell({ ch: '世', width: 2, bold: true, italic: true }))).toMatchObject({
      wide: true,
      bold: true,
      italic: true,
    })
  })

  it('hides invisible text', () => {
    expect(drawCell(0, cell({ ch: 'x', invisible: true }))).toBeNull()
  })
})

describe('readRow', () => {
  it('collects the drawable cells with their columns', () => {
    const row = readRow(line([{ ch: 'a' }, {}, { ch: 'b' }]), 3)
    expect(row.cells.map((c) => [c.col, c.ch])).toEqual([
      [0, 'a'],
      [2, 'b'],
    ])
  })

  it('stops at the requested column count', () => {
    expect(readRow(line([{ ch: 'a' }, { ch: 'b' }, { ch: 'c' }]), 2).cells).toHaveLength(2)
  })

  it('returns an empty row for a missing line', () => {
    expect(readRow(undefined, 5)).toEqual({ cells: [], signature: '' })
  })

  it('gives equal signatures to rows that look the same', () => {
    const a = readRow(line([{ ch: 'a', fg: { palette: 1 } }, { ch: 'b' }]), 2)
    const b = readRow(line([{ ch: 'a', fg: { palette: 1 } }, { ch: 'b' }]), 2)
    expect(a.signature).toBe(b.signature)
    expect(a.signature).not.toBe('')
  })

  it('changes the signature with the character, the color, the position and the attributes', () => {
    const base = readRow(line([{ ch: 'a' }]), 1).signature
    expect(readRow(line([{ ch: 'b' }]), 1).signature).not.toBe(base)
    expect(readRow(line([{ ch: 'a', fg: { palette: 2 } }]), 1).signature).not.toBe(base)
    expect(readRow(line([{}, { ch: 'a' }]), 2).signature).not.toBe(base)
    expect(readRow(line([{ ch: 'a', bold: true }]), 1).signature).not.toBe(base)
    expect(readRow(line([{ ch: 'a', underline: true }]), 1).signature).not.toBe(base)
    expect(readRow(line([{ ch: 'a', bg: { palette: 2 } }]), 1).signature).not.toBe(base)
  })

  it('gives the same signature to blank rows however they are blank', () => {
    expect(readRow(line([{}, { ch: ' ' }]), 2).signature).toBe(readRow(undefined, 2).signature)
  })
})
