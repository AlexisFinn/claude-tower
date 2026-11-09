import { describe, expect, it } from 'vitest'
import { gridKeyAction } from './keys'
import { SHORTCUT_GROUPS } from './shortcuts'

const key = (k: string) => ({ key: k, ctrlKey: false, shiftKey: false, altKey: false, metaKey: false })

describe('SHORTCUT_GROUPS', () => {
  it('groups the shortcuts under Navigation, Session and Compositeur', () => {
    expect(SHORTCUT_GROUPS.map((g) => g.title)).toEqual(['Navigation', 'Session', 'Compositeur'])
  })

  it('has a label and at least one key for every shortcut', () => {
    for (const group of SHORTCUT_GROUPS) {
      for (const item of group.items) {
        expect(item.label).not.toBe('')
        expect(item.combos.length).toBeGreaterThan(0)
        for (const combo of item.combos) expect(combo.length).toBeGreaterThan(0)
      }
    }
  })

  it('lists the single letters that the grid actually handles', () => {
    const letters = SHORTCUT_GROUPS.flatMap((g) => g.items.flatMap((i) => i.combos))
      .filter((c) => c.length === 1 && /^[a-z/?]$/.test(c[0]))
      .map((c) => c[0])
    for (const letter of letters) expect(gridKeyAction(key(letter))).not.toBeNull()
  })
})
