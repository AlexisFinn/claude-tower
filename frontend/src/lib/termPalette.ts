/** Default background of a terminal, thumbnail or fullscreen; the same as --screen in style.css. */
export const DEFAULT_BG = '#07090c'
/** Default text color of a terminal, thumbnail or fullscreen; the same as --text in style.css. */
export const DEFAULT_FG = '#e7ecf3'

const ANSI_16 = [
  '#2d3440',
  '#ef6b73',
  '#8fcf7a',
  '#e5c07b',
  '#6aa8f5',
  '#c792ea',
  '#56c8d8',
  '#c5cdd9',
  '#5c6674',
  '#ff8a90',
  '#a9e69a',
  '#f5d98f',
  '#8dbcff',
  '#dcaeff',
  '#7fe0ec',
  '#f2f5fa',
]

const CUBE_LEVELS = [0, 95, 135, 175, 215, 255]

/** Formats a 0xRRGGBB number as a lowercase #rrggbb string. */
export function rgbHex(value: number): string {
  return `#${(value & 0xffffff).toString(16).padStart(6, '0')}`
}

function buildPalette(): string[] {
  const palette = [...ANSI_16]
  for (let i = 0; i < 216; i++) {
    const r = CUBE_LEVELS[Math.floor(i / 36)]
    const g = CUBE_LEVELS[Math.floor(i / 6) % 6]
    const b = CUBE_LEVELS[i % 6]
    palette.push(rgbHex((r << 16) | (g << 8) | b))
  }
  for (let i = 0; i < 24; i++) {
    const level = 8 + 10 * i
    palette.push(rgbHex((level << 16) | (level << 8) | level))
  }
  return palette
}

const PALETTE = buildPalette()

/** Returns the color of a palette index: 16 ANSI colors, the 6x6x6 cube, then 24 grays. */
export function paletteColor(index: number): string {
  return PALETTE[index] ?? DEFAULT_FG
}

/** Returns the color at fraction t (0 to 1) of the way from color a to color b. */
export function mixColors(a: string, b: string, t: number): string {
  const av = parseInt(a.slice(1), 16)
  const bv = parseInt(b.slice(1), 16)
  const channel = (shift: number): number => {
    const from = (av >> shift) & 0xff
    const to = (bv >> shift) & 0xff
    return Math.round(from + (to - from) * t)
  }
  return rgbHex((channel(16) << 16) | (channel(8) << 8) | channel(0))
}
