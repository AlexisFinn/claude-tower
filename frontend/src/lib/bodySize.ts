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
    cols: Math.max(1, Math.floor(width / cell.width)),
    rows: Math.max(1, Math.floor(height / cell.height)),
  }
}

/** Smallest font size, in pixels, of a card body. */
export const MIN_FONT = 6
/** Largest font size, in pixels, of a card body. */
export const MAX_FONT = 11

/** The font size and the grid of a card body. */
export interface BodyFit {
  fontSize: number
  cols: number
  rows: number
}

/**
 * Returns the largest font size, in half-pixel steps between min and max, at
 * which paneCols cells fit in bodyWidth. The cell width scales linearly with
 * the font size from cellWidthAtRef, measured at refFont.
 */
export function fitFont(
  paneCols: number,
  bodyWidth: number,
  cellWidthAtRef: number,
  refFont: number,
  min = MIN_FONT,
  max = MAX_FONT,
): number {
  const fitting = (bodyWidth * refFont) / (paneCols * cellWidthAtRef)
  const stepped = Math.floor(fitting * 2 + 1e-9) / 2
  return Math.min(max, Math.max(min, stepped))
}

/**
 * Returns the font size and the grid of a card body. With a known pane width
 * the grid has as many columns as the pane and the font shrinks to fit them,
 * down to min, below which the right side is cut. Without one, the grid is
 * the whole body at max. Rows are those that fit the height at that font.
 */
export function fitBody(
  paneCols: number | null,
  bodyWidth: number,
  bodyHeight: number,
  refCell: CellSize,
  refFont: number,
  min = MIN_FONT,
  max = MAX_FONT,
): BodyFit {
  const fontSize =
    paneCols === null ? max : fitFont(paneCols, bodyWidth, refCell.width, refFont, min, max)
  const scale = fontSize / refFont
  const grid = bodySize(bodyWidth, bodyHeight, {
    width: refCell.width * scale,
    height: refCell.height * scale,
  })
  return { fontSize, cols: paneCols ?? grid.cols, rows: grid.rows }
}
