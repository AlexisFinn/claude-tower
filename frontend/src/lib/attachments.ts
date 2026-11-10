const PLAIN_PATH = /^[\p{L}\p{N}_@%+=:,./~-]+$/u

/** Returns the `@` mention that makes Claude Code include a file: quoted when the path has a space or a character special to a shell. */
export function mentionOf(path: string): string {
  return PLAIN_PATH.test(path) ? `@${path}` : `@"${path}"`
}

/** Returns the attached paths with the new ones appended, without duplicates and without empty paths. */
export function addFiles(files: readonly string[], added: readonly string[]): string[] {
  const seen = new Set(files)
  const out = [...files]
  for (const path of added) {
    if (path === '' || seen.has(path)) continue
    seen.add(path)
    out.push(path)
  }
  return out
}

/** Returns the attached paths without the given one. */
export function removeFile(files: readonly string[], path: string): string[] {
  return files.filter((f) => f !== path)
}

/** Returns the last component of a path, which is the whole path when it has no separator. */
export function fileName(path: string): string {
  const parts = path.split(/[\\/]/).filter((p) => p !== '')
  return parts.length > 0 ? parts[parts.length - 1] : path
}

/**
 * Returns the message to send: the text, then a blank line and the mentions of the files separated by spaces. Without text, only the mentions.
 * A space follows the last mention: a message ending with a mention leaves the file suggestions of Claude Code open, and the Enter
 * that submits it would pick a suggestion instead.
 */
export function composeMessage(text: string, files: readonly string[]): string {
  if (files.length === 0) return text
  const mentions = files.map(mentionOf).join(' ') + ' '
  return text.trim() === '' ? mentions : `${text.trimEnd()}\n\n${mentions}`
}
