import { describe, expect, it } from 'vitest'
import { addFiles, composeMessage, fileName, mentionOf, removeFile } from './attachments'

describe('mentionOf', () => {
  it('writes a plain absolute path after an @', () => {
    expect(mentionOf('/home/alex/notes.md')).toBe('@/home/alex/notes.md')
    expect(mentionOf('/home/alex/été-2026_v1.2+final.md')).toBe('@/home/alex/été-2026_v1.2+final.md')
  })

  it('quotes a path with a space or a shell-special character', () => {
    expect(mentionOf('/home/alex/mes notes.md')).toBe('@"/home/alex/mes notes.md"')
    expect(mentionOf('/tmp/a(1).txt')).toBe('@"/tmp/a(1).txt"')
    expect(mentionOf('/tmp/a&b.txt')).toBe('@"/tmp/a&b.txt"')
    expect(mentionOf("/tmp/it's.txt")).toBe('@"/tmp/it\'s.txt"')
    expect(mentionOf('/tmp/$HOME.txt')).toBe('@"/tmp/$HOME.txt"')
  })
})

describe('addFiles', () => {
  it('appends the new paths in order', () => {
    expect(addFiles(['/a'], ['/b', '/c'])).toEqual(['/a', '/b', '/c'])
  })

  it('ignores paths already attached and repeated in the same batch', () => {
    expect(addFiles(['/a', '/b'], ['/b', '/c', '/c', '/a'])).toEqual(['/a', '/b', '/c'])
  })

  it('ignores empty paths', () => {
    expect(addFiles([], ['', '/a'])).toEqual(['/a'])
  })

  it('does not change the attached list it receives', () => {
    const files = ['/a']
    addFiles(files, ['/b'])
    expect(files).toEqual(['/a'])
  })
})

describe('removeFile', () => {
  it('removes only the given path', () => {
    expect(removeFile(['/a', '/b', '/c'], '/b')).toEqual(['/a', '/c'])
    expect(removeFile(['/a'], '/x')).toEqual(['/a'])
  })
})

describe('fileName', () => {
  it('returns the last component of the path', () => {
    expect(fileName('/home/alex/notes.md')).toBe('notes.md')
    expect(fileName('C:\\Users\\alex\\notes.md')).toBe('notes.md')
    expect(fileName('/home/alex/dossier/')).toBe('dossier')
    expect(fileName('notes.md')).toBe('notes.md')
    expect(fileName('/')).toBe('/')
  })
})

describe('composeMessage', () => {
  it('returns the text untouched without files', () => {
    expect(composeMessage('fais ceci\n', [])).toBe('fais ceci\n')
  })

  it('puts the mentions after a blank line, separated by spaces', () => {
    expect(composeMessage('lis ça', ['/a.txt', '/b c.txt'])).toBe('lis ça\n\n@/a.txt @"/b c.txt" ')
  })

  it('does not stack blank lines when the text ends with newlines', () => {
    expect(composeMessage('lis ça\n\n', ['/a.txt'])).toBe('lis ça\n\n@/a.txt ')
  })

  it('ends with a space, so that the file suggestions of Claude Code are closed when Enter submits', () => {
    expect(composeMessage('lis ça', ['/a.txt']).endsWith(' ')).toBe(true)
  })

  it('sends only the mentions when there is no text', () => {
    expect(composeMessage('', ['/a.txt'])).toBe('@/a.txt ')
    expect(composeMessage(' \n ', ['/a.txt', '/b.txt'])).toBe('@/a.txt @/b.txt ')
  })
})
