import { describe, expect, it } from 'vitest'
import { startFailureText } from './startFailures'

describe('startFailureText', () => {
  it('names the session and its folder', () => {
    expect(startFailureText({ Name: 'proj', Cwd: '~/proj', Lines: [] })).toBe(
      'La session « proj » (~/proj) s’est arrêtée avant que Claude soit prêt.',
    )
  })

  it('adds the last lines of the screen', () => {
    expect(startFailureText({ Name: 'proj', Cwd: '', Lines: ['Welcome', ' ', 'Error: boom'] })).toBe(
      'La session « proj » s’est arrêtée avant que Claude soit prêt. Dernières lignes : Welcome · Error: boom',
    )
  })

  it('accepts missing lines', () => {
    expect(startFailureText({ Name: 'p', Cwd: '', Lines: null })).toBe('La session « p » s’est arrêtée avant que Claude soit prêt.')
  })
})
