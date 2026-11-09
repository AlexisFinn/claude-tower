/**
 * Returns the selected PID after the list of PIDs changed. The selection
 * follows its PID; when that PID is gone, the card now at the same position
 * (or the last one) is selected.
 */
export function reconcileSelection(
  oldPids: number[],
  newPids: number[],
  selected: number | null,
): number | null {
  if (newPids.length === 0) return null
  if (selected !== null && newPids.includes(selected)) return selected
  const position = selected === null ? -1 : oldPids.indexOf(selected)
  if (position < 0) return newPids[0]
  return newPids[Math.min(position, newPids.length - 1)]
}
