/** A card position, in pixels, inside the grid area. */
export interface Rect {
  x: number
  y: number
  width: number
  height: number
}

/** The positions of the cards and the height of the area they cover. */
export interface Layout {
  rects: Rect[]
  contentHeight: number
  scrolls: boolean
}

/** Smallest card width, in pixels, before the grid scrolls. */
export const MIN_CARD_WIDTH = 320
/** Smallest card height, in pixels, before the grid scrolls. */
export const MIN_CARD_HEIGHT = 230

/**
 * Tiles n cards over a width x height area, splitting vertically first: rows
 * share the height and the cards of a row share the width, so the cards cover
 * the whole area. An incomplete last row spreads its width over its cards.
 * When cards would get smaller than the minimum size, rows are
 * MIN_CARD_HEIGHT tall and the content scrolls vertically.
 */
export function layout(n: number, width: number, height: number): Layout {
  if (n <= 0) return { rects: [], contentHeight: height, scrolls: false }
  const cols = Math.ceil(Math.sqrt(n))
  const rows = Math.ceil(n / cols)
  if (width / cols >= MIN_CARD_WIDTH && height / rows >= MIN_CARD_HEIGHT) {
    return { rects: tile(n, cols, width, height), contentHeight: height, scrolls: false }
  }
  const fitCols = Math.min(cols, Math.max(1, Math.floor(width / MIN_CARD_WIDTH)))
  const fitRows = Math.ceil(n / fitCols)
  const rowHeight = Math.max(MIN_CARD_HEIGHT, height / fitRows)
  const contentHeight = Math.round(rowHeight * fitRows)
  return {
    rects: tile(n, fitCols, width, contentHeight),
    contentHeight,
    scrolls: contentHeight > height,
  }
}

function tile(n: number, cols: number, width: number, height: number): Rect[] {
  const rows = Math.ceil(n / cols)
  const rects: Rect[] = []
  for (let r = 0; r < rows; r++) {
    const count = Math.min(cols, n - r * cols)
    const y0 = Math.round((r * height) / rows)
    const y1 = Math.round(((r + 1) * height) / rows)
    for (let i = 0; i < count; i++) {
      const x0 = Math.round((i * width) / count)
      const x1 = Math.round(((i + 1) * width) / count)
      rects.push({ x: x0, y: y0, width: x1 - x0, height: y1 - y0 })
    }
  }
  return rects
}

/** Returns the rect shrunk by the given amount on every side, never below zero size. */
export function insetRect(rect: Rect, by: number): Rect {
  return {
    x: rect.x + by,
    y: rect.y + by,
    width: Math.max(0, rect.width - 2 * by),
    height: Math.max(0, rect.height - 2 * by),
  }
}
