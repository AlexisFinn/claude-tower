import { describe, expect, it } from 'vitest'
import { screenSequence } from './screenSequence'

describe('screenSequence', () => {
  it('disables wrap, homes the cursor and clears each line and the rest', () => {
    expect(screenSequence(['one', 'two', 'three'], 3)).toBe(
      '\x1b[?7l\x1b[Hone\x1b[K\r\ntwo\x1b[K\r\nthree\x1b[K\x1b[J',
    )
  })

  it('does not end the last line with a line break', () => {
    expect(screenSequence(['a'], 5)).toBe('\x1b[?7l\x1b[Ha\x1b[K\x1b[J')
  })

  it('keeps the last lines when there are more than rows', () => {
    expect(screenSequence(['a', 'b', 'c'], 2)).toBe('\x1b[?7l\x1b[Hb\x1b[K\r\nc\x1b[K\x1b[J')
  })

  it('only clears the screen without lines', () => {
    expect(screenSequence([], 4)).toBe('\x1b[?7l\x1b[H\x1b[J')
  })
})
