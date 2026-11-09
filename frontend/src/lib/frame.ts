const KNOWN_STATES = ['waiting', 'busy', 'background', 'idle']

/** Returns the CSS classes of a card frame, from the session state. */
export function frameClass(state: string, selected: boolean): string {
  const name = KNOWN_STATES.includes(state) ? state : 'unknown'
  return selected ? `frame-${name} selected` : `frame-${name}`
}
