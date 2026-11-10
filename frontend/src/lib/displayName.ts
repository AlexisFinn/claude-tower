/** The part of a card that names its session. */
export interface Named {
  Name: string
  Title?: string
}

function title(card: Named): string {
  return (card.Title ?? '').trim()
}

/** Returns the title of the session when it has one, else its name. */
export function displayName(card: Named): string {
  return title(card) === '' ? card.Name : (card.Title as string)
}

/**
 * Returns the name to show beside the title, which keeps the identifier of
 * the session, or '' when there is no title or the title already is the name,
 * possibly cut short with an ellipsis in the name.
 */
export function nameTag(card: Named): string {
  const t = title(card)
  if (t === '' || t === card.Name) return ''
  if (card.Name.endsWith('…') && t.startsWith(card.Name.slice(0, -1))) return ''
  return card.Name
}
