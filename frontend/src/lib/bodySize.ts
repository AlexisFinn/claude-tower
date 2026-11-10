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

/** Largest font size, in pixels, at which the text of a card body is displayed. */
export const MAX_DISPLAY_FONT = 16
/** Displayed font size, in pixels, when the pane width is unknown. */
export const DEFAULT_DISPLAY_FONT = 11
/** Cell size per pixel of font size, used until a real measure is available. */
export const FALLBACK_CELL_RATIO: CellSize = { width: 0.6, height: 1.2 }

/** The grid and the displayed cell size of a card body. */
export interface BodyGeometry extends GridSize {
  /** Displayed font size, in CSS pixels. */
  font: number
  /** Displayed cell size, in CSS pixels. */
  cellWidth: number
  cellHeight: number
}

/**
 * Returns the grid and the displayed cell size of a card body. cellRatio is the
 * cell size per pixel of font size. With a pane width the grid has as many
 * columns as the pane and they fill bodyWidth, up to MAX_DISPLAY_FONT. Without
 * one, the font is DEFAULT_DISPLAY_FONT and the columns are those that fill
 * bodyWidth. Rows are those that fit bodyHeight, at most paneRows when it is
 * known, and there is always at least one row and one column.
 */
export function bodyGeometry(
  paneCols: number | null,
  paneRows: number | null,
  bodyWidth: number,
  bodyHeight: number,
  cellRatio: CellSize,
): BodyGeometry {
  const width = Math.max(0, bodyWidth)
  const font =
    paneCols === null
      ? DEFAULT_DISPLAY_FONT
      : Math.min(MAX_DISPLAY_FONT, width / (paneCols * cellRatio.width))
  const cellWidth = font * cellRatio.width
  const cellHeight = font * cellRatio.height
  const cols = paneCols ?? Math.max(1, Math.floor(width / cellWidth + 1e-9))
  const fitting = Math.max(1, Math.floor(Math.max(0, bodyHeight) / cellHeight + 1e-9))
  const rows = paneRows !== null && paneRows > 0 ? Math.min(fitting, paneRows) : fitting
  return { cols, rows, font, cellWidth, cellHeight }
}

/** Reports whether two geometries are the same. */
export function sameGeometry(a: BodyGeometry | null, b: BodyGeometry | null): boolean {
  if (a === null || b === null) return a === b
  return (
    a.cols === b.cols &&
    a.rows === b.rows &&
    a.font === b.font &&
    a.cellWidth === b.cellWidth &&
    a.cellHeight === b.cellHeight
  )
}

/** Font size, in pixels, that a session sized to its card displays its text at. */
export const FIT_FONT = 12
/** Smallest terminal a session is sized to: below it Claude Code cannot lay out its screen. */
export const MIN_FIT: GridSize = { cols: 60, rows: 12 }
/** Largest terminal a session is sized to. */
export const MAX_FIT = 1000

/**
 * Returns the terminal size that fills a card body at FIT_FONT, within MIN_FIT
 * and MAX_FIT: a body too small for MIN_FIT shows it with a smaller font.
 */
export function fitGrid(bodyWidth: number, bodyHeight: number, cellRatio: CellSize): GridSize {
  const fit = (length: number, cell: number, min: number): number =>
    Math.min(MAX_FIT, Math.max(min, Math.floor(Math.max(0, length) / (cell * FIT_FONT) + 1e-9)))
  return { cols: fit(bodyWidth, cellRatio.width, MIN_FIT.cols), rows: fit(bodyHeight, cellRatio.height, MIN_FIT.rows) }
}
