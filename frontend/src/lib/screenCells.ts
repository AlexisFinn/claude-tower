import type { IBufferCell } from '@xterm/xterm'
import { DEFAULT_BG, DEFAULT_FG, mixColors, paletteColor, rgbHex } from './termPalette'

/** Share of the background mixed into the text color of a dim cell. */
const DIM_MIX = 0.45

/** The part of an xterm buffer cell that a thumbnail reads. */
export type CellLike = Pick<
  IBufferCell,
  | 'getChars'
  | 'getWidth'
  | 'getFgColor'
  | 'getBgColor'
  | 'isFgRGB'
  | 'isBgRGB'
  | 'isFgPalette'
  | 'isBgPalette'
  | 'isBold'
  | 'isItalic'
  | 'isDim'
  | 'isUnderline'
  | 'isInverse'
  | 'isInvisible'
>

/** The part of an xterm buffer line that a thumbnail reads. */
export interface LineLike {
  getCell(x: number, cell?: IBufferCell): CellLike | undefined
}

/** A cell to draw, with its colors resolved and its inverse and dim attributes applied. */
export interface DrawCell {
  col: number
  ch: string
  /** True for a double-width character, which spans this column and the next. */
  wide: boolean
  fg: string
  /** Null for the default background. */
  bg: string | null
  bold: boolean
  italic: boolean
  underline: boolean
}

/** The drawable cells of one row, and a string that is equal for two rows only when they look the same. */
export interface Row {
  cells: DrawCell[]
  signature: string
}

function foreground(cell: CellLike): string | null {
  if (cell.isFgRGB()) return rgbHex(cell.getFgColor())
  return cell.isFgPalette() ? paletteColor(cell.getFgColor()) : null
}

function background(cell: CellLike): string | null {
  if (cell.isBgRGB()) return rgbHex(cell.getBgColor())
  return cell.isBgPalette() ? paletteColor(cell.getBgColor()) : null
}

/**
 * Resolves a buffer cell into a draw cell, or returns null when it draws
 * nothing: the second half of a wide character, or a blank cell on the default
 * background without underline.
 */
export function drawCell(col: number, cell: CellLike): DrawCell | null {
  const width = cell.getWidth()
  if (width === 0) return null
  let fg = foreground(cell)
  let bg = background(cell)
  if (cell.isInverse() !== 0) {
    const swapped = fg
    fg = bg ?? DEFAULT_BG
    bg = swapped ?? DEFAULT_FG
  }
  let text = fg ?? DEFAULT_FG
  if (cell.isDim() !== 0) text = mixColors(text, bg ?? DEFAULT_BG, DIM_MIX)
  const chars = cell.getChars()
  const ch = cell.isInvisible() !== 0 || chars === ' ' ? '' : chars
  const underline = cell.isUnderline() !== 0
  if (ch === '' && bg === null && !underline) return null
  return {
    col,
    ch,
    wide: width === 2,
    fg: text,
    bg,
    bold: cell.isBold() !== 0,
    italic: cell.isItalic() !== 0,
    underline,
  }
}

function signatureOf(cells: DrawCell[]): string {
  let out = ''
  for (const c of cells) {
    const flags = (c.wide ? 1 : 0) | (c.bold ? 2 : 0) | (c.italic ? 4 : 0) | (c.underline ? 8 : 0)
    out += `${c.col},${c.ch},${c.fg},${c.bg ?? ''},${flags};`
  }
  return out
}

/** Reads the first cols cells of a buffer line, which may be missing. cell is reused between calls. */
export function readRow(line: LineLike | undefined, cols: number, cell?: IBufferCell): Row {
  const cells: DrawCell[] = []
  if (line) {
    for (let x = 0; x < cols; x++) {
      const raw = line.getCell(x, cell)
      if (!raw) break
      const resolved = drawCell(x, raw)
      if (resolved) cells.push(resolved)
    }
  }
  return { cells, signature: signatureOf(cells) }
}

/** The part of an xterm buffer that a thumbnail reads. */
export interface BufferLike {
  getLine(y: number): LineLike | undefined
  getNullCell(): IBufferCell
}
