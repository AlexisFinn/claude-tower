/** A session started by the application that ended before Claude Code was ready. */
export interface StartFailure {
  Name: string
  Cwd: string
  Lines: string[] | null
}

/** Returns the notification text of a failed start. */
export function startFailureText(f: StartFailure): string {
  const where = f.Cwd ? ` (${f.Cwd})` : ''
  const head = `La session « ${f.Name} »${where} s’est arrêtée avant que Claude soit prêt.`
  const lines = (f.Lines ?? []).filter((l) => l.trim() !== '')
  return lines.length === 0 ? head : `${head} Dernières lignes : ${lines.join(' · ')}`
}
