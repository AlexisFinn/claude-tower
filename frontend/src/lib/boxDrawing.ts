/** A filled rectangle of a glyph, in cell pixels, painted with the text color at the given opacity. */
export interface RectShape {
  kind: 'rect'
  x: number
  y: number
  w: number
  h: number
  alpha: number
}

/** A stroked line of a glyph, in cell pixels. */
export interface LineShape {
  kind: 'line'
  x1: number
  y1: number
  x2: number
  y2: number
  width: number
}

export type Shape = RectShape | LineShape

const FIRST = 0x2500
const LAST = 0x259f

/**
 * Weights of the left, right, up and down arms of U+2500 to U+257F, in that
 * order: 0 none, 1 light, 2 heavy, 3 double. "diag" marks the diagonals.
 * Dashed lines are drawn solid and arcs as square corners.
 */
const BOX = [
  '1100', '2200', '0011', '0022', '1100', '2200', '0011', '0022',
  '1100', '2200', '0011', '0022', '0101', '0201', '0102', '0202',
  '1001', '2001', '1002', '2002', '0110', '0210', '0120', '0220',
  '1010', '2010', '1020', '2020', '0111', '0211', '0121', '0112',
  '0122', '0221', '0212', '0222', '1011', '2011', '1021', '1012',
  '1022', '2021', '2012', '2022', '1101', '2101', '1201', '2201',
  '1102', '2102', '1202', '2202', '1110', '2110', '1210', '2210',
  '1120', '2120', '1220', '2220', '1111', '2111', '1211', '2211',
  '1121', '1112', '1122', '2121', '1221', '2112', '1212', '2221',
  '2212', '2122', '1222', '2222', '1100', '2200', '0011', '0022',
  '3300', '0033', '0301', '0103', '0303', '3001', '1003', '3003',
  '0310', '0130', '0330', '3010', '1030', '3030', '0311', '0133',
  '0333', '3011', '1033', '3033', '3301', '1103', '3303', '3310',
  '1130', '3330', '3311', '1133', '3333', '0101', '1001', '1010',
  '0110', 'diag', 'diag', 'diag', '1000', '0010', '0100', '0001',
  '2000', '0020', '0200', '0002', '1200', '0012', '2100', '0021',
]

/** Reports whether a code point is a box-drawing or block-element character. */
export function isBoxGlyph(codePoint: number): boolean {
  return codePoint >= FIRST && codePoint <= LAST
}

/** Returns half of the band that an arm of the given weight occupies across its axis. */
function halfBand(weight: number, t: number): number {
  return weight === 0 ? 0 : (weight === 3 ? 3 * t : weight * t) / 2
}

interface Arm {
  /** Weight of the arm and of the two perpendicular arms, on its negative and positive side. */
  weight: number
  negative: number
  positive: number
}

/**
 * Returns the cross-axis bands of an arm as [from, to] offsets from the center,
 * each with the distance from the center at which the band starts along the
 * axis, measured outwards.
 */
function armBands(arm: Arm, t: number): { from: number; to: number; start: number }[] {
  const { weight, negative, positive } = arm
  if (weight === 3) {
    return [
      { from: -1.5 * t, to: -0.5 * t, start: lineStart(negative, positive, t) },
      { from: 0.5 * t, to: 1.5 * t, start: lineStart(positive, negative, t) },
    ]
  }
  const half = (weight * t) / 2
  return [{ from: -half, to: half, start: -Math.max(halfBand(negative, t), halfBand(positive, t)) }]
}

/** Returns where a line of a double arm starts, given the perpendicular arms on its own side and on the other. */
function lineStart(own: number, other: number, t: number): number {
  if (own > 0) return own === 3 ? 0.5 * t : 0
  return -halfBand(other, t)
}

function armShapes(
  horizontal: boolean,
  outward: 1 | -1,
  arm: Arm,
  cx: number,
  cy: number,
  w: number,
  h: number,
  t: number,
): RectShape[] {
  const shapes: RectShape[] = []
  for (const band of armBands(arm, t)) {
    const a0 = Math.round((horizontal ? cx : cy) + outward * band.start)
    const a1 = outward === 1 ? (horizontal ? w : h) : 0
    const c0 = Math.round((horizontal ? cy : cx) + band.from)
    const c1 = Math.round((horizontal ? cy : cx) + band.to)
    const lo = Math.min(a0, a1)
    const along = Math.abs(a1 - a0)
    const across = Math.max(1, c1 - c0)
    shapes.push(
      horizontal
        ? { kind: 'rect', x: lo, y: c0, w: along, h: across, alpha: 1 }
        : { kind: 'rect', x: c0, y: lo, w: across, h: along, alpha: 1 },
    )
  }
  return shapes
}

function boxShapes(index: number, w: number, h: number): Shape[] {
  const t = Math.max(1, Math.round(h / 16))
  const spec = BOX[index]
  if (spec === 'diag') {
    const lines: LineShape[] = []
    if (index !== 0x72) lines.push({ kind: 'line', x1: 0, y1: h, x2: w, y2: 0, width: t })
    if (index !== 0x71) lines.push({ kind: 'line', x1: 0, y1: 0, x2: w, y2: h, width: t })
    return lines
  }
  const [l, r, u, d] = [...spec].map(Number)
  const cx = Math.round(w / 2)
  const cy = Math.round(h / 2)
  return [
    ...(l ? armShapes(true, -1, { weight: l, negative: u, positive: d }, cx, cy, w, h, t) : []),
    ...(r ? armShapes(true, 1, { weight: r, negative: u, positive: d }, cx, cy, w, h, t) : []),
    ...(u ? armShapes(false, -1, { weight: u, negative: l, positive: r }, cx, cy, w, h, t) : []),
    ...(d ? armShapes(false, 1, { weight: d, negative: l, positive: r }, cx, cy, w, h, t) : []),
  ]
}

const QUADRANTS: Record<number, string> = {
  0x2596: 'c',
  0x2597: 'd',
  0x2598: 'a',
  0x2599: 'acd',
  0x259a: 'ad',
  0x259b: 'abc',
  0x259c: 'abd',
  0x259d: 'b',
  0x259e: 'bc',
  0x259f: 'bcd',
}

function rect(x: number, y: number, w: number, h: number, alpha = 1): RectShape {
  return { kind: 'rect', x, y, w, h, alpha }
}

function blockShapes(code: number, w: number, h: number): Shape[] {
  const eighthH = (n: number): number => Math.round((h * n) / 8)
  const eighthW = (n: number): number => Math.round((w * n) / 8)
  const halfW = Math.round(w / 2)
  const halfH = Math.round(h / 2)
  if (code === 0x2580) return [rect(0, 0, w, halfH)]
  if (code >= 0x2581 && code <= 0x2588) return [rect(0, h - eighthH(code - 0x2580), w, eighthH(code - 0x2580))]
  if (code >= 0x2589 && code <= 0x258f) return [rect(0, 0, eighthW(0x2590 - code), h)]
  if (code === 0x2590) return [rect(halfW, 0, w - halfW, h)]
  if (code >= 0x2591 && code <= 0x2593) return [rect(0, 0, w, h, (code - 0x2590) / 4)]
  if (code === 0x2594) return [rect(0, 0, w, eighthH(1))]
  if (code === 0x2595) return [rect(w - eighthW(1), 0, eighthW(1), h)]
  const parts = QUADRANTS[code] ?? ''
  return [...parts].map((q) => {
    const x = q === 'b' || q === 'd' ? halfW : 0
    const y = q === 'c' || q === 'd' ? halfH : 0
    return rect(x, y, x === 0 ? halfW : w - halfW, y === 0 ? halfH : h - halfH)
  })
}

/**
 * Returns the shapes that draw a box-drawing or block-element character in a
 * w x h cell whose origin is (0, 0), so that neighbouring cells join without
 * gaps. Returns an empty list for a code point outside U+2500 to U+259F.
 */
export function glyphShapes(codePoint: number, w: number, h: number): Shape[] {
  if (!isBoxGlyph(codePoint)) return []
  return codePoint <= 0x257f ? boxShapes(codePoint - FIRST, w, h) : blockShapes(codePoint, w, h)
}

/** Returns the shapes of a glyph in a w x h cell, the same list for the same arguments. */
export type GlyphCache = (codePoint: number, w: number, h: number) => Shape[]

/**
 * Creates a memoized glyphShapes. The cache grows with the distinct glyph and
 * cell sizes asked, so a geometry change calls for a new one.
 */
export function createGlyphCache(): GlyphCache {
  const shapes = new Map<string, Shape[]>()
  return (codePoint, w, h) => {
    const key = `${codePoint}|${w}|${h}`
    let known = shapes.get(key)
    if (!known) {
      known = glyphShapes(codePoint, w, h)
      shapes.set(key, known)
    }
    return known
  }
}
