import { describe, expect, it } from 'vitest'
import { decodeBase64 } from './base64'

const encode = (bytes: Uint8Array): string => btoa(String.fromCharCode(...bytes))

describe('decodeBase64', () => {
  it('decodes ASCII', () => {
    expect([...decodeBase64('aGVsbG8=')]).toEqual([...new TextEncoder().encode('hello')])
  })

  it('decodes bytes that are not valid text', () => {
    expect([...decodeBase64(encode(new Uint8Array([0, 255, 128, 27])))]).toEqual([0, 255, 128, 27])
  })

  it('keeps a multi-byte character split between two packets', () => {
    const bytes = new TextEncoder().encode('a€b')
    const first = decodeBase64(encode(bytes.slice(0, 2)))
    const second = decodeBase64(encode(bytes.slice(2)))
    expect(first).toHaveLength(2)
    expect([...first, ...second]).toEqual([...bytes])
    const decoder = new TextDecoder()
    const text = decoder.decode(first, { stream: true }) + decoder.decode(second, { stream: true })
    expect(text).toBe('a€b')
  })

  it('returns no bytes for an empty string', () => {
    expect(decodeBase64('')).toHaveLength(0)
  })
})
