import { DEFAULT_BG, mixColors } from './termPalette'

/** How much a thumbnail thickens and lightens its text. */
export interface Ink {
  /** Fraction of the way to white by which a dark text color is lifted, 0 for none. */
  lift: number
  /** Glyph outline width per pixel of font size, 0 for none. */
  stroke: number
}
/** Background luminance under which dim text is lifted: the default background and near-black ones, not colored ones. */
const DARK_BACKGROUND = 0.05
/** Text luminance under which a color is lifted, about mid-gray. */
const DARK_TEXT = 0.25

const HEX = /^#[0-9a-f]{6}$/i

/** Relative luminance of a #rrggbb color, from 0 to 1. */
function luminance(hex: string): number {
  const v = parseInt(hex.slice(1), 16)
  const channel = (shift: number): number => {
    const c = ((v >> shift) & 0xff) / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * channel(16) + 0.7152 * channel(8) + 0.0722 * channel(0)
}

/** Largest number of color pairs kept; truecolor output can produce many, so the cache starts over beyond it. */
const CACHE_LIMIT = 4096
const colorCache = new Map<string, string>()

/**
 * Returns the color a thumbnail draws text in. At a few pixels per glyph, dim
 * text fades into a dark background, so on the default background or a
 * near-black one a color darker than mid-gray moves part of the way to white.
 * Other colors, and text on any other background, are kept.
 */
export function textColor(fg: string, bg: string | null, lift: number): string {
  if (lift <= 0) return fg
  const key = `${fg}|${bg ?? ''}|${lift}`
  let out = colorCache.get(key)
  if (out === undefined) {
    const back = bg ?? DEFAULT_BG
    const dark = HEX.test(fg) && HEX.test(back) && luminance(back) < DARK_BACKGROUND && luminance(fg) < DARK_TEXT
    out = dark ? mixColors(fg, '#ffffff', lift) : fg
    if (colorCache.size >= CACHE_LIMIT) colorCache.clear()
    colorCache.set(key, out)
  }
  return out
}

/**
 * Returns the width of the outline drawn around the glyphs of a font size:
 * it thickens the strokes, which downscaling otherwise thins to faint lines.
 */
export function strokeWidth(fontPx: number, stroke: number): number {
  return fontPx * stroke
}

/**
 * Reports whether a glyph gets the outline: wide glyphs (CJK, emoji) and
 * glyphs outside the Basic Multilingual Plane are often drawn in color or
 * already dense, and an outline would only blur them.
 */
export function outlines(ch: string, wide: boolean): boolean {
  return !wide && ch.length === 1
}
