import { THUMBNAIL_FONT_FAMILY } from './thumbnail'

const FALLBACK = 'ui-monospace, "DejaVu Sans Mono", "Liberation Mono", monospace'

/** The fonts that F9 cycles through, with the name shown when one is chosen. */
export const THUMBNAIL_FONTS: readonly { name: string; family: string }[] = [
  { name: 'JetBrains Mono', family: THUMBNAIL_FONT_FAMILY },
  { name: 'Source Code Pro', family: `"Source Code Pro", "SauceCodePro Nerd Font Mono", ${FALLBACK}` },
  { name: 'Fira Code', family: `"Fira Code", "FiraCode Nerd Font Mono", ${FALLBACK}` },
  { name: 'Cascadia Code', family: `"Cascadia Code", "Cascadia Code NF", "CaskaydiaCove Nerd Font Mono", ${FALLBACK}` },
]

const STORAGE_KEY = 'claude-tower.thumbnail-font'

/** Returns the index of the saved font, 0 when none is saved or the saved one is unknown. */
export function savedFontIndex(storage: Pick<Storage, 'getItem'> | undefined): number {
  const value = Number(storage?.getItem(STORAGE_KEY))
  return Number.isInteger(value) && value >= 0 && value < THUMBNAIL_FONTS.length ? value : 0
}

/** Saves the index of the chosen font; a storage that fails leaves the choice for this run only. */
export function saveFontIndex(storage: Pick<Storage, 'setItem'> | undefined, index: number): void {
  try {
    storage?.setItem(STORAGE_KEY, String(index))
  } catch {
    // The choice still applies until the application closes.
  }
}
