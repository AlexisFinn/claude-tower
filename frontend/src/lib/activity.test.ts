import { describe, expect, it } from 'vitest'
import { parseActivity } from './activity'

describe('parseActivity', () => {
  it('recognises a user prompt and drops its arrow', () => {
    expect(parseActivity('› Peux-tu corriger le test ?')).toEqual({
      kind: 'prompt',
      text: 'Peux-tu corriger le test ?',
    })
  })

  it('recognises a tool call', () => {
    expect(parseActivity('Bash: npm test')).toEqual({ kind: 'tool', text: 'Bash: npm test' })
    expect(parseActivity('Read: /home/a/b.ts').kind).toBe('tool')
    expect(parseActivity('mcp__linear__get_issue: DEV-1').kind).toBe('tool')
  })

  it('recognises a tool name alone', () => {
    expect(parseActivity('Bash')).toEqual({ kind: 'tool', text: 'Bash' })
    expect(parseActivity('TodoWrite').kind).toBe('tool')
    expect(parseActivity('mcp__linear__get_issue').kind).toBe('tool')
  })

  it('recognises a CamelCase identifier as a tool', () => {
    expect(parseActivity('FooBar: x').kind).toBe('tool')
    expect(parseActivity('FooBar').kind).toBe('tool')
    expect(parseActivity('Foo').kind).toBe('text')
    expect(parseActivity('OK').kind).toBe('text')
  })

  it('treats anything else as the text of Claude', () => {
    expect(parseActivity('Les tests passent.')).toEqual({ kind: 'text', text: 'Les tests passent.' })
    expect(parseActivity('Note: x')).toEqual({ kind: 'text', text: 'Note: x' })
    expect(parseActivity('Attention: le test échoue').kind).toBe('text')
    expect(parseActivity('Bash est lent').kind).toBe('text')
    expect(parseActivity('note: voici la suite').kind).toBe('text')
  })

  it('reports no activity for an empty text', () => {
    expect(parseActivity('')).toEqual({ kind: 'none', text: '' })
    expect(parseActivity('   ').kind).toBe('none')
  })
})
