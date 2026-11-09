import { stateName } from './frame'

/** The background work of a session. */
export interface BackgroundTasks {
  agents: number
  shells: number
}

function capitalize(text: string): string {
  return text === '' ? text : text[0].toUpperCase() + text.slice(1)
}

/** Returns the text of a state capsule, from the session state and its raw label. */
export function stateLabel(state: string, label: string): string {
  switch (stateName(state)) {
    case 'background':
      return 'Fini · tâches de fond'
    case 'idle':
      return 'Fini'
    case 'busy':
      return label === 'shell' ? 'Shell' : 'Travaille'
    case 'waiting':
      return waitingLabel(label)
    default:
      return label === '' || label === '?' ? 'Inconnu' : capitalize(label)
  }
}

const waitingReasons: Record<string, string> = {
  'permission prompt': 'Attend une autorisation',
  'input needed': 'Attend une réponse',
  'dialog open': 'Dialogue ouvert',
  'sandbox request': 'Demande d’accès sandbox',
  'worker request': 'Demande d’un agent',
  'goal proposal': 'Propose un objectif',
}

// The backend label of a waiting session is "attend" or "attend (<reason>)",
// where the reason is the raw value reported by Claude Code.
function waitingLabel(label: string): string {
  const reason = /\((.*)\)\s*$/.exec(label)?.[1]?.trim() ?? ''
  if (reason === '') return 'Attend'
  return waitingReasons[reason] ?? `Attend (${reason})`
}

function plural(count: number, word: string): string {
  return `${count} ${word}${count > 1 ? 's' : ''}`
}

/** Returns the badge text of the background work, or '' when there is none. */
export function backgroundLabel(tasks: BackgroundTasks | undefined): string {
  if (!tasks) return ''
  const parts: string[] = []
  if (tasks.agents > 0) parts.push(plural(tasks.agents, 'agent'))
  if (tasks.shells > 0) parts.push(plural(tasks.shells, 'shell'))
  return parts.join(' · ')
}

/** Returns the background work reported for a card, or undefined when there is none. */
export function backgroundOf(card: { BackgroundAgents: number; BackgroundShells: number }): BackgroundTasks | undefined {
  const agents = Math.max(0, card.BackgroundAgents)
  const shells = Math.max(0, card.BackgroundShells)
  return agents + shells > 0 ? { agents, shells } : undefined
}
