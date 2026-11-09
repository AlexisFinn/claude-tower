import type { Rect } from './layout'

/** A move direction in the grid. */
export type Direction = 'left' | 'right' | 'up' | 'down'

const centerX = (r: Rect): number => r.x + r.width / 2

/**
 * Returns the index of the card reached from index in the given direction,
 * following the geometry of rects. At an edge, index is returned unchanged.
 * Vertical moves land on the card of the adjacent row whose center is the
 * closest to the center of the current card.
 */
export function move(rects: Rect[], index: number, dir: Direction): number {
  const cur = rects[index]
  if (!cur) return rects.length > 0 ? 0 : index
  if (dir === 'left' || dir === 'right') return moveInRow(rects, index, dir)
  return moveAcrossRows(rects, index, dir)
}

function moveInRow(rects: Rect[], index: number, dir: 'left' | 'right'): number {
  const cur = rects[index]
  const sign = dir === 'left' ? -1 : 1
  let best = index
  let bestGap = Infinity
  rects.forEach((r, i) => {
    if (r.y !== cur.y) return
    const gap = (centerX(r) - centerX(cur)) * sign
    if (gap > 0 && gap < bestGap) {
      best = i
      bestGap = gap
    }
  })
  return best
}

function moveAcrossRows(rects: Rect[], index: number, dir: 'up' | 'down'): number {
  const cur = rects[index]
  const rowY = adjacentRowY(rects, cur, dir)
  if (rowY === null) return index
  let best = index
  let bestGap = Infinity
  rects.forEach((r, i) => {
    if (r.y !== rowY) return
    const gap = Math.abs(centerX(r) - centerX(cur))
    if (gap < bestGap) {
      best = i
      bestGap = gap
    }
  })
  return best
}

function adjacentRowY(rects: Rect[], cur: Rect, dir: 'up' | 'down'): number | null {
  let rowY: number | null = null
  for (const r of rects) {
    if (dir === 'down' && r.y >= cur.y + cur.height) {
      if (rowY === null || r.y < rowY) rowY = r.y
    }
    if (dir === 'up' && r.y + r.height <= cur.y) {
      if (rowY === null || r.y > rowY) rowY = r.y
    }
  }
  return rowY
}
