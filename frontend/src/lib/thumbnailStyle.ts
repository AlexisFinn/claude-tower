import { THUMBNAIL_FONT_FAMILY } from './thumbnail'
import type { Ink } from './thumbnailInk'

/** A monospace font that stays legible at small sizes, with the default stack as fallback. */
export const LEGIBLE_FAMILY = `"JetBrainsMono Nerd Font Mono", "JetBrains Mono", ${THUMBNAIL_FONT_FAMILY}`

/** How the thumbnails draw text. */
export interface ThumbnailStyle {
  family: string
  subpixel: boolean
  ink: Ink
}

const style = (ink: Ink): ThumbnailStyle => ({ family: LEGIBLE_FAMILY, subpixel: true, ink })

/** The styles that F9 cycles through, with the name shown when one is chosen. */
export const THUMBNAIL_STYLES: readonly { name: string; style: ThumbnailStyle }[] = [
  { name: 'sans épaississement ni éclaircissement', style: style({ lift: 0, stroke: 0 }) },
  { name: 'éclaircissement léger seul', style: style({ lift: 0.12, stroke: 0 }) },
  { name: 'épaississement et éclaircissement légers', style: style({ lift: 0.12, stroke: 0.02 }) },
  { name: 'épaississement et éclaircissement forts (actuel)', style: style({ lift: 0.25, stroke: 0.045 }) },
]

const STORAGE_KEY = 'claude-tower.thumbnail-ink'

/** Returns the index of the saved style, 0 when none is saved or the saved one is unknown. */
export function savedStyleIndex(storage: Pick<Storage, 'getItem'> | undefined): number {
  const value = Number(storage?.getItem(STORAGE_KEY))
  return Number.isInteger(value) && value >= 0 && value < THUMBNAIL_STYLES.length ? value : THUMBNAIL_STYLES.length - 1
}

/** Saves the index of the chosen style; a storage that fails leaves the choice for this run only. */
export function saveStyleIndex(storage: Pick<Storage, 'setItem'> | undefined, index: number): void {
  try {
    storage?.setItem(STORAGE_KEY, String(index))
  } catch {
    // The choice still applies until the application closes.
  }
}
