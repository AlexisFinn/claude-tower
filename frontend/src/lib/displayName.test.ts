import { describe, expect, it } from 'vitest'
import { displayName, nameTag } from './displayName'

describe('displayName', () => {
  it('returns the title when there is one', () => {
    expect(displayName({ Name: 'zest-f6', Title: 'Export des objectifs' })).toBe('Export des objectifs')
  })

  it('returns the name without a title', () => {
    expect(displayName({ Name: 'zest-f6', Title: '' })).toBe('zest-f6')
    expect(displayName({ Name: 'zest-f6', Title: '  ' })).toBe('zest-f6')
    expect(displayName({ Name: 'zest-f6' })).toBe('zest-f6')
  })

  it('returns the name when the title is the name', () => {
    expect(displayName({ Name: 'Refonte', Title: 'Refonte' })).toBe('Refonte')
  })
})

describe('nameTag', () => {
  it('keeps the name beside a generated title', () => {
    expect(nameTag({ Name: 'zest-f6', Title: 'Export des objectifs' })).toBe('zest-f6')
  })

  it('is empty without a title', () => {
    expect(nameTag({ Name: 'zest-f6', Title: '' })).toBe('')
    expect(nameTag({ Name: 'zest-f6' })).toBe('')
  })

  it('is empty when the title is the name the user chose', () => {
    expect(nameTag({ Name: 'Refonte', Title: 'Refonte' })).toBe('')
    expect(nameTag({ Name: 'Refonte export de…', Title: 'Refonte export des objectifs' })).toBe('')
  })

  it('keeps a name that only looks like the start of the title', () => {
    expect(nameTag({ Name: 'Refonte…', Title: 'Export des objectifs' })).toBe('Refonte…')
  })
})
