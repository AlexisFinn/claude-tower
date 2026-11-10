import { THUMBNAIL_FONT_FAMILY } from './thumbnail'

/** How the thumbnails draw text. */
export interface ThumbnailStyle {
  /** Font stack of the text. */
  family: string
  /** Draws with subpixel rendering at the display resolution, instead of downscaling an oversampled image. */
  subpixel: boolean
}

/** A monospace font that stays legible at small sizes, with the default stack as fallback. */
const LEGIBLE_FAMILY = `"JetBrainsMono Nerd Font Mono", "JetBrains Mono", ${THUMBNAIL_FONT_FAMILY}`

/** The styles that F9 cycles through, with the name shown when one is chosen. */
export const THUMBNAIL_STYLES: readonly { name: string; style: ThumbnailStyle }[] = [
  { name: 'actuel', style: { family: THUMBNAIL_FONT_FAMILY, subpixel: false } },
  { name: 'police JetBrains Mono', style: { family: LEGIBLE_FAMILY, subpixel: false } },
  { name: 'ClearType', style: { family: THUMBNAIL_FONT_FAMILY, subpixel: true } },
  { name: 'ClearType + JetBrains Mono', style: { family: LEGIBLE_FAMILY, subpixel: true } },
]

const STORAGE_KEY = 'claude-tower.thumbnail-style'

/** Returns the index of the saved style, 0 when none is saved or the saved one is unknown. */
export function savedStyleIndex(storage: Pick<Storage, 'getItem'> | undefined): number {
  const value = Number(storage?.getItem(STORAGE_KEY))
  return Number.isInteger(value) && value >= 0 && value < THUMBNAIL_STYLES.length ? value : 0
}

/** Saves the index of the chosen style; a storage that fails leaves the choice for this run only. */
export function saveStyleIndex(storage: Pick<Storage, 'setItem'> | undefined, index: number): void {
  try {
    storage?.setItem(STORAGE_KEY, String(index))
  } catch {
    // The choice still applies until the application closes.
  }
}
