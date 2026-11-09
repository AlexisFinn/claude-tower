/** What the last activity of a session is. */
export type ActivityKind = 'prompt' | 'tool' | 'text' | 'none'

/** The last activity of a session, classified, with the text to show. */
export interface ParsedActivity {
  kind: ActivityKind
  text: string
}

const PROMPT_PREFIX = '› '
const KNOWN_TOOLS = new Set([
  'Bash',
  'Read',
  'Edit',
  'Write',
  'MultiEdit',
  'Grep',
  'Glob',
  'Agent',
  'Task',
  'TodoWrite',
  'WebFetch',
  'WebSearch',
  'NotebookEdit',
  'Skill',
  'ToolSearch',
  'Monitor',
  'SendMessage',
  'TaskStop',
  'AskUserQuestion',
  'ExitPlanMode',
  'EnterPlanMode',
])
const IDENTIFIER = /^[A-Z][A-Za-z0-9]*$/

function isToolName(name: string): boolean {
  if (KNOWN_TOOLS.has(name) || /^mcp__\S+$/.test(name)) return true
  return IDENTIFIER.test(name) && /[a-z]/.test(name) && (name.match(/[A-Z]/g) ?? []).length >= 2
}

function isTool(text: string): boolean {
  const colon = text.indexOf(':')
  return isToolName(colon < 0 ? text : text.slice(0, colon))
}

/**
 * Classifies the activity text of a card: a user prompt starts with the
 * prompt arrow, a tool call is a known tool name, an mcp__ name or a CamelCase
 * identifier, alone or followed by ": argument", anything else is the text of
 * Claude. The arrow of a prompt is dropped from the text.
 */
export function parseActivity(activity: string): ParsedActivity {
  const text = activity.trim()
  if (text === '') return { kind: 'none', text: '' }
  if (text.startsWith(PROMPT_PREFIX)) return { kind: 'prompt', text: text.slice(PROMPT_PREFIX.length).trim() }
  if (isTool(text)) return { kind: 'tool', text }
  return { kind: 'text', text }
}
