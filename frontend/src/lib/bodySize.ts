/** The size of one terminal cell, in pixels. */
export interface CellSize {
  width: number
  height: number
}

/** A terminal size, in cells. */
export interface GridSize {
  cols: number
  rows: number
}

/** Returns the number of cells that fill a width x height area, at least 1x1. */
export function bodySize(width: number, height: number, cell: CellSize): GridSize {
  return {
    cols: Math.max(1, Math.floor(width / cell.width + 1e-9)),
    rows: Math.max(1, Math.floor(height / cell.height + 1e-9)),
  }
}

/** Smallest font size, in pixels, at which the terminal of a card body is rendered. */
export const MIN_RENDER_FONT = 10
/** Largest font size, in pixels, at which the terminal of a card body is rendered. */
export const MAX_RENDER_FONT = 28
/** Largest font size, in pixels, at which the text of a card body is displayed. */
export const MAX_DISPLAY_FONT = 16
/** Displayed font size, in pixels, when the pane width is unknown. */
export const DEFAULT_DISPLAY_FONT = 11
/** Ratio between the render font and the displayed font. */
export const OVERSAMPLING = 2
/** Cell size per pixel of font size, used until a real measure is available. */
export const FALLBACK_CELL_RATIO: CellSize = { width: 0.6, height: 1.2 }

/** The scale and the grid of a card body terminal. */
export interface BodyFit {
  /** Scale applied to the terminal rendered at the render font, at most 1. */
  scale: number
  cols: number
  rows: number
}

/**
 * Returns the font size, in whole pixels, at which the terminal is rendered
 * before being scaled down: about OVERSAMPLING times the font size displayed
 * when paneCols columns fill bodyWidth. cellRatio is the cell size per pixel of
 * font size. Without a pane width, the displayed font is DEFAULT_DISPLAY_FONT.
 */
export function renderFont(paneCols: number | null, bodyWidth: number, cellRatio: CellSize): number {
  const displayed =
    paneCols === null
      ? DEFAULT_DISPLAY_FONT
      : Math.min(MAX_DISPLAY_FONT, bodyWidth / (paneCols * cellRatio.width))
  return Math.min(MAX_RENDER_FONT, Math.max(MIN_RENDER_FONT, Math.round(displayed * OVERSAMPLING)))
}

/**
 * Returns the scale that makes a terminal rendered at font, naturalWidth wide
 * for the pane columns, as wide as bodyWidth. The scale never enlarges the
 * terminal and keeps the displayed font at MAX_DISPLAY_FONT or below. Without
 * a natural width, the displayed font is DEFAULT_DISPLAY_FONT.
 */
export function displayScale(font: number, bodyWidth: number, naturalWidth: number | null): number {
  const cap = Math.min(1, MAX_DISPLAY_FONT / font)
  if (naturalWidth === null || naturalWidth <= 0) return Math.min(cap, DEFAULT_DISPLAY_FONT / font)
  return Math.min(cap, Math.max(0, bodyWidth) / naturalWidth)
}

/**
 * Returns the scale and the grid of a card body. cell is the cell size at
 * font, and naturalWidth the measured width of the terminal at font with
 * paneCols columns, or null when it is unknown (the cell width then gives an
 * estimate). With a pane width the grid has
 * as many columns as the pane and the scale makes them fill bodyWidth, up to
 * MAX_DISPLAY_FONT. Without one, the columns are those that fill bodyWidth at
 * DEFAULT_DISPLAY_FONT. Rows are those that fit bodyHeight at that scale, at
 * most paneRows when it is known.
 */
export function fitBody(
  paneCols: number | null,
  bodyWidth: number,
  bodyHeight: number,
  font: number,
  cell: CellSize,
  naturalWidth: number | null,
  paneRows: number | null = null,
): BodyFit {
  const natural = paneCols === null ? null : (naturalWidth ?? paneCols * cell.width)
  const scale = displayScale(font, bodyWidth, natural)
  const grid = bodySize(bodyWidth, bodyHeight, {
    width: cell.width * scale,
    height: cell.height * scale,
  })
  // The pane never has more rows than its height: capping them bounds the
  // rendered layer when the scale is tiny.
  const rows = paneRows !== null && paneRows > 0 ? Math.min(grid.rows, paneRows) : grid.rows
  return { scale, cols: paneCols ?? grid.cols, rows }
}
