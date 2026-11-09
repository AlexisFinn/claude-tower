const KNOWN_STATES = ['waiting', 'busy', 'background', 'idle']

/** A session state as the interface knows it. */
export type StateName = 'waiting' | 'busy' | 'background' | 'idle' | 'unknown'

/** Returns the known state name of a session state, or 'unknown'. */
export function stateName(state: string): StateName {
  return (KNOWN_STATES.includes(state) ? state : 'unknown') as StateName
}

/** Returns the CSS classes of a card frame, from the session state. */
export function frameClass(state: string, selected: boolean): string {
  const name = stateName(state)
  return selected ? `frame-${name} selected` : `frame-${name}`
}
