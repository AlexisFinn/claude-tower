import { describe, expect, it } from 'vitest'
import { countLines, heightStateOf, INITIAL_HEIGHT_STATE, needsHeightMeasure } from './composerHeight'

describe('countLines', () => {
  it('counts logical lines', () => {
    expect(countLines('')).toBe(1)
    expect(countLines('abc')).toBe(1)
    expect(countLines('a\nb')).toBe(2)
    expect(countLines('a\n\nb\n')).toBe(4)
  })
})

describe('needsHeightMeasure', () => {
  const never = () => false
  const always = () => true

  it('measures before the first measure', () => {
    expect(needsHeightMeasure(INITIAL_HEIGHT_STATE, '', never)).toBe(true)
  })

  it('does nothing when text grows on the same line and fits', () => {
    expect(needsHeightMeasure(heightStateOf('abc'), 'abcd', never)).toBe(false)
  })

  it('measures when the number of lines changed', () => {
    expect(needsHeightMeasure(heightStateOf('abc'), 'abc\n', never)).toBe(true)
    expect(needsHeightMeasure(heightStateOf('a\nb'), 'a', never)).toBe(true)
  })

  it('measures when the text got shorter', () => {
    expect(needsHeightMeasure(heightStateOf('abcdef'), 'abcde', never)).toBe(true)
  })

  it('measures when the content overflows', () => {
    expect(needsHeightMeasure(heightStateOf('abc'), 'abcd', always)).toBe(true)
  })

  it('does not read the overflow when the lines or the length already require a measure', () => {
    let reads = 0
    const overflows = () => {
      reads++
      return false
    }
    needsHeightMeasure(heightStateOf('abc'), 'abc\n', overflows)
    needsHeightMeasure(heightStateOf('abc'), 'ab', overflows)
    expect(reads).toBe(0)
  })
})
