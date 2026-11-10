/** The session states as the interface knows them, most urgent first. */
export const STATE_NAMES = ['waiting', 'busy', 'background', 'idle', 'unknown'] as const

/** A session state as the interface knows it. */
export type StateName = (typeof STATE_NAMES)[number]

/** Returns the known state name of a session state, or 'unknown'. */
export function stateName(state: string): StateName {
  return STATE_NAMES.find((name) => name === state) ?? 'unknown'
}

/** Returns the CSS classes of a card frame, from the session state. */
export function frameClass(state: string, selected: boolean): string {
  const name = stateName(state)
  return selected ? `frame-${name} selected` : `frame-${name}`
}
