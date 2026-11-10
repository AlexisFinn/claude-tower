import { STATE_NAMES, stateName, type StateName } from './frame'

/** The number of sessions in one state, with the word that follows the number. */
export interface StateCount {
  state: StateName
  count: number
  label: string
}

const LABELS: Record<StateName, [one: string, many: string]> = {
  waiting: ['attend', 'attendent'],
  busy: ['travaille', 'travaillent'],
  background: ['en fond', 'en fond'],
  idle: ['finie', 'finies'],
  unknown: ['inconnue', 'inconnues'],
}

/** Counts the sessions per state, most urgent state first, leaving out the states with no session. */
export function summarizeStates(cards: { State: string }[]): StateCount[] {
  const counts = new Map<StateName, number>()
  for (const card of cards) {
    const name = stateName(card.State)
    counts.set(name, (counts.get(name) ?? 0) + 1)
  }
  return STATE_NAMES.filter((state) => counts.has(state)).map((state) => {
    const count = counts.get(state) ?? 0
    return { state, count, label: LABELS[state][count === 1 ? 0 : 1] }
  })
}
