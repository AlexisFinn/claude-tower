import { describe, expect, it } from 'vitest'
import { backgroundLabel, backgroundOf, stateLabel } from './cardText'

describe('stateLabel', () => {
  it('names each known state', () => {
    expect(stateLabel('idle', 'fini')).toBe('Fini')
    expect(stateLabel('background', 'fini · fond')).toBe('Fini · tâches de fond')
    expect(stateLabel('busy', 'travaille')).toBe('Travaille')
    expect(stateLabel('busy', 'shell')).toBe('Shell')
    expect(stateLabel('busy', 'démarrage')).toBe('Démarre')
    expect(stateLabel('waiting', 'attend (démarrage)')).toBe('Bloquée au démarrage')
  })

  it('keeps what a waiting session waits for', () => {
    expect(stateLabel('waiting', 'attend')).toBe('Attend')
    expect(stateLabel('waiting', 'attend (permission)')).toBe('Attend (permission)')
    expect(stateLabel('waiting', 'attend (permission prompt)')).toBe('Attend une autorisation')
    expect(stateLabel('waiting', 'attend (input needed)')).toBe('Attend une réponse')
    expect(stateLabel('waiting', '')).toBe('Attend')
  })

  it('words each known waiting reason', () => {
    expect(stateLabel('waiting', 'attend (permission prompt)')).toBe('Attend une autorisation')
    expect(stateLabel('waiting', 'attend (input needed)')).toBe('Attend une réponse')
    expect(stateLabel('waiting', 'attend (dialog open)')).toBe('Dialogue ouvert')
    expect(stateLabel('waiting', 'attend (sandbox request)')).toBe('Demande d’accès sandbox')
    expect(stateLabel('waiting', 'attend (worker request)')).toBe('Demande d’un agent')
    expect(stateLabel('waiting', 'attend (goal proposal)')).toBe('Propose un objectif')
  })

  it('keeps an unknown waiting reason as is', () => {
    expect(stateLabel('waiting', 'attend (something new)')).toBe('Attend (something new)')
  })

  it('falls back on the raw label for an unknown state', () => {
    expect(stateLabel('weird', 'bizarre')).toBe('Bizarre')
    expect(stateLabel('weird', '?')).toBe('Inconnu')
    expect(stateLabel('', '')).toBe('Inconnu')
  })
})

describe('backgroundLabel', () => {
  it('is empty without background work', () => {
    expect(backgroundLabel(undefined)).toBe('')
    expect(backgroundLabel({ agents: 0, shells: 0 })).toBe('')
  })

  it('counts agents and shells', () => {
    expect(backgroundLabel({ agents: 2, shells: 1 })).toBe('2 agents · 1 shell')
    expect(backgroundLabel({ agents: 1, shells: 0 })).toBe('1 agent')
    expect(backgroundLabel({ agents: 0, shells: 3 })).toBe('3 shells')
  })
})

describe('backgroundOf', () => {
  it('returns the counts when some work runs', () => {
    expect(backgroundOf({ BackgroundAgents: 2, BackgroundShells: 1 })).toEqual({ agents: 2, shells: 1 })
  })

  it('treats an unknown shell count as none', () => {
    expect(backgroundOf({ BackgroundAgents: 1, BackgroundShells: -1 })).toEqual({ agents: 1, shells: 0 })
    expect(backgroundOf({ BackgroundAgents: 0, BackgroundShells: -1 })).toBeUndefined()
  })

  it('returns undefined when nothing runs', () => {
    expect(backgroundOf({ BackgroundAgents: 0, BackgroundShells: 0 })).toBeUndefined()
  })
})
