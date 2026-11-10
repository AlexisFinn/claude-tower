import { DEFAULT_BG, mixColors } from './termPalette';

/** How the thumbnails draw text. */
export interface ThumbnailStyle {
  /** Thickens the glyphs and lifts the dark text colors. */
  heavy: boolean;
  /** Draws at the display resolution, so the font rasterizer smooths the text, instead of downscaling an oversampled image. */
  direct: boolean;
}

/** The styles that F9 cycles through, with the name shown when one is chosen. */
export const THUMBNAIL_STYLES: readonly { name: string; style: ThumbnailStyle }[] = [
  { name: 'réduit (actuel)', style: { heavy: false, direct: false } },
  { name: 'réduit, épais et contrasté', style: { heavy: true, direct: false } },
  { name: 'direct', style: { heavy: false, direct: true } },
  { name: 'direct, épais et contrasté', style: { heavy: true, direct: true } }
];

const STORAGE_KEY = 'claude-tower.thumbnail-style';

/** Returns the index of the saved style, 0 when none is saved or the saved one is unknown. */
export function savedStyleIndex(storage: Pick<Storage, 'getItem'> | undefined): number {
  const value = Number(storage?.getItem(STORAGE_KEY));
  return Number.isInteger(value) && value >= 0 && value < THUMBNAIL_STYLES.length ? value : 0;
}

/** Saves the index of the chosen style; a storage that fails leaves the choice for this run only. */
export function saveStyleIndex(storage: Pick<Storage, 'setItem'> | undefined, index: number): void {
  try {
    storage?.setItem(STORAGE_KEY, String(index));
  } catch {
    // The choice still applies until the application closes.
  }
}

/** Fraction of the way to the default text color by which a heavy style lifts a color. */
const LIFT = 0.25;
/** Glyph outline width of a heavy style, per pixel of font size. */
const STROKE_PER_PX = 0.045;

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
 * Returns the text color a style draws: a heavy style moves a color darker
 * than mid-gray part of the way to white, so dim text stands out from the
 * dark background; a light color, or a color on a light background, is kept.
 */
export function textColor(fg: string, bg: string | null, style: ThumbnailStyle): string {
  if (!style.heavy || !/^#[0-9a-f]{6}$/i.test(fg)) return fg;
  const back = bg ?? DEFAULT_BG;
  if (/^#[0-9a-f]{6}$/i.test(back) && luminance(back) > 0.2) return fg;
  return luminance(fg) < 0.25 ? mixColors(fg, '#ffffff', LIFT) : fg;
}

/** Returns the width of the outline a style adds around glyphs of a font size, 0 for none. */
export function strokeWidth(fontPx: number, style: ThumbnailStyle): number {
  return style.heavy ? fontPx * STROKE_PER_PX : 0;
}
