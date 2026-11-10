import { DEFAULT_BG, mixColors } from './termPalette';

/** Fraction of the way to white by which a dark text color is lifted. */
const LIFT = 0.25;
/** Glyph outline width, per pixel of font size. */
const STROKE_PER_PX = 0.045;

const HEX = /^#[0-9a-f]{6}$/i;

/** Relative luminance of a #rrggbb color, from 0 to 1. */
function luminance(hex: string): number {
  const v = parseInt(hex.slice(1), 16);
  const channel = (shift: number): number => {
    const c = ((v >> shift) & 0xff) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * channel(16) + 0.7152 * channel(8) + 0.0722 * channel(0);
}

/**
 * Returns the color a thumbnail draws text in. At a few pixels per glyph, dim
 * text fades into the dark background, so a color darker than mid-gray moves
 * part of the way to white; a light color, or any color on a light
 * background, is kept.
 */
export function textColor(fg: string, bg: string | null): string {
  if (!HEX.test(fg)) return fg;
  const back = bg ?? DEFAULT_BG;
  if (HEX.test(back) && luminance(back) > 0.2) return fg;
  return luminance(fg) < 0.25 ? mixColors(fg, '#ffffff', LIFT) : fg;
}

/**
 * Returns the width of the outline drawn around the glyphs of a font size:
 * it thickens the strokes, which downscaling otherwise thins to faint lines.
 */
export function strokeWidth(fontPx: number): number {
  return fontPx * STROKE_PER_PX;
}
