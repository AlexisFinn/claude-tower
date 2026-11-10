import { describe, expect, it, vi } from 'vitest'
import type { core } from '../../wailsjs/go/models'
import { createFitRequests } from './fitRequests'
import { createScreenFeed } from './screenFeed'
import { FAILURE_THRESHOLD } from './screenFailures'
import type { StartFailure } from './startFailures'

function card(pid: number, extra: Partial<core.Card> = {}): core.Card {
  return { PID: pid, Order: pid, InTmux: true, Attached: false, ...extra } as core.Card
}

function screen(hash: string, width = 80, height = 24): core.Screen {
  return { Lines: [], Width: width, Height: height, Hash: hash }
}

/** Builds a feed over fakes and records what it publishes. */
function setup(initial: core.Card[] = [card(1)]) {
  const state = {
    list: initial,
    cardsError: null as unknown,
    startFailures: [] as StartFailure[],
    startFailuresError: false,
    screens: new Map<number, core.Screen | Error>(),
    fitResult: true as boolean | Error,
    onScreenRead: (_pid: number) => {},
    fullscreen: false,
    hidden: false,
    typing: false,
    selected: null as number | null,
    published: {
      loaded: 0,
      cards: [] as core.Card[],
      screens: {} as Record<number, core.Screen>,
      screenPublications: [] as number[][],
      unavailable: new Set<number>(),
      selected: undefined as number | null | undefined,
      notifications: [] as { kind: string; text: string }[],
    },
  }
  const screenCalls: [number, number][] = []
  const fitCalls: [number, number, number][] = []
  const requestRefresh = vi.fn()
  let now = 0

  const feed = createScreenFeed({
    cards: async () => {
      if (state.cardsError !== null) throw state.cardsError
      return state.list
    },
    screen: async (pid, rows) => {
      screenCalls.push([pid, rows])
      state.onScreenRead(pid)
      const result = state.screens.get(pid) ?? screen('default')
      if (result instanceof Error) throw result
      return result
    },
    fitSession: async (pid, cols, rows) => {
      fitCalls.push([pid, cols, rows])
      if (state.fitResult instanceof Error) throw state.fitResult
      return state.fitResult
    },
    startFailures: async () => {
      if (state.startFailuresError) throw new Error('boom')
      return state.startFailures
    },
    fullscreenOpen: () => state.fullscreen,
    documentHidden: () => state.hidden,
    typing: () => state.typing,
    getSelected: () => state.selected,
    requestRefresh,
    fitRequests: createFitRequests(() => now),
    onLoaded: () => state.published.loaded++,
    onCards: (list) => (state.published.cards = list),
    onScreens: (screens) => {
      state.published.screens = screens
      state.published.screenPublications.push(Object.keys(screens).map(Number))
    },
    onUnavailable: (unavailable) => (state.published.unavailable = unavailable),
    onSelected: (pid) => (state.published.selected = pid),
    notify: (kind, text) => state.published.notifications.push({ kind, text }),
  })
  return {
    feed,
    state,
    screenCalls,
    fitCalls,
    requestRefresh,
    advance: (ms: number) => (now += ms),
  }
}

describe('cycle cards', () => {
  it('publishes the cards sorted by Order and marks the feed as loaded', async () => {
    const t = setup([card(1, { Order: 3 }), card(2, { Order: 1 }), card(3, { Order: 2 })])
    await t.feed.cycle()
    expect(t.state.published.cards.map((c) => c.PID)).toEqual([2, 3, 1])
    expect(t.state.published.loaded).toBe(1)
  })

  it('does not reorder the list it received', async () => {
    const list = [card(1, { Order: 2 }), card(2, { Order: 1 })]
    const t = setup(list)
    await t.feed.cycle()
    expect(list.map((c) => c.PID)).toEqual([1, 2])
  })

  it('notifies a failed read of the cards and publishes nothing', async () => {
    const t = setup()
    t.state.cardsError = new Error('hors service')
    await expect(t.feed.cycle()).resolves.toBeUndefined()
    expect(t.state.published.notifications).toEqual([{ kind: 'error', text: expect.stringContaining('hors service') }])
    expect(t.state.published.loaded).toBe(0)
    expect(t.state.published.selected).toBeUndefined()
  })

  it('runs the next cycle after a failed read', async () => {
    const t = setup()
    t.state.cardsError = new Error('hors service')
    await t.feed.cycle()
    t.state.cardsError = null
    await t.feed.cycle()
    expect(t.state.published.cards.map((c) => c.PID)).toEqual([1])
  })

  it('notifies the failed starts', async () => {
    const t = setup()
    t.state.startFailures = [{ Name: 'api', Cwd: '/srv', Lines: ['boom'] }]
    await t.feed.cycle()
    expect(t.state.published.notifications).toEqual([{ kind: 'error', text: expect.stringContaining('api') }])
    expect(t.state.published.cards.map((c) => c.PID)).toEqual([1])
  })

  it('goes on when the failed starts cannot be read', async () => {
    const t = setup()
    t.state.startFailuresError = true
    await t.feed.cycle()
    expect(t.state.published.notifications).toEqual([])
    expect(t.state.published.cards.map((c) => c.PID)).toEqual([1])
  })

  it('reconciles the selection with the new list', async () => {
    const t = setup([card(1), card(2)])
    t.state.selected = 5
    await t.feed.cycle()
    expect(t.state.published.selected).toBe(1)
    t.state.selected = 2
    await t.feed.cycle()
    expect(t.state.published.selected).toBe(2)
    t.state.list = [card(1)]
    await t.feed.cycle()
    expect(t.state.published.selected).toBe(1)
  })
})

describe('cycle screens', () => {
  it('publishes the screen of a visible and measured card only', async () => {
    const t = setup([card(1), card(2), card(3), card(4, { InTmux: false })])
    t.feed.measure(1, { cols: 80, rows: 20 })
    t.feed.visibility(1, true)
    t.feed.visibility(2, true)
    t.feed.measure(3, { cols: 80, rows: 20 })
    t.feed.measure(4, { cols: 80, rows: 20 })
    t.feed.visibility(4, true)
    t.state.screens.set(1, screen('one'))
    await t.feed.cycle()
    expect(t.screenCalls).toEqual([[1, 20]])
    expect(Object.keys(t.state.published.screens)).toEqual(['1'])
    expect(t.state.published.screens[1].Hash).toBe('one')
  })

  it('stops asking for the screen of a card that is no longer visible', async () => {
    const t = setup()
    t.feed.measure(1, { cols: 80, rows: 20 })
    t.feed.visibility(1, true)
    t.feed.visibility(1, false)
    await t.feed.cycle()
    expect(t.screenCalls).toEqual([])
  })

  it('stops asking for the screen of a card whose measure is gone', async () => {
    const t = setup()
    t.feed.measure(1, { cols: 80, rows: 20 })
    t.feed.visibility(1, true)
    t.feed.measure(1, null)
    await t.feed.cycle()
    expect(t.screenCalls).toEqual([])
  })

  it('publishes the screens one card after the other', async () => {
    const t = setup([card(1), card(2)])
    for (const pid of [1, 2]) {
      t.feed.measure(pid, { cols: 80, rows: 20 })
      t.feed.visibility(pid, true)
    }
    await t.feed.cycle()
    expect(t.state.published.screenPublications).toEqual([[], [1], [1, 2]])
  })

  it('keeps the last screen of a card that is not refreshed', async () => {
    const t = setup([card(1), card(2)])
    for (const pid of [1, 2]) {
      t.feed.measure(pid, { cols: 80, rows: 20 })
      t.feed.visibility(pid, true)
    }
    await t.feed.cycle()
    t.feed.visibility(2, false)
    t.state.screens.set(1, screen('later'))
    await t.feed.cycle()
    expect(Object.keys(t.state.published.screens).sort()).toEqual(['1', '2'])
    expect(t.state.published.screens[1].Hash).toBe('later')
  })

  it('stops the walk when a pause starts in the middle of the loop', async () => {
    const t = setup([card(1), card(2), card(3)])
    for (const pid of [1, 2, 3]) {
      t.feed.measure(pid, { cols: 80, rows: 20 })
      t.feed.visibility(pid, true)
    }
    t.state.onScreenRead = (pid) => {
      if (pid === 1) t.state.typing = true
    }
    await t.feed.cycle()
    expect(t.screenCalls).toEqual([[1, 20]])
    expect(Object.keys(t.state.published.screens)).toEqual(['1'])
  })

  it.each([
    ['the fullscreen view is open', (s: ReturnType<typeof setup>['state']) => (s.fullscreen = true)],
    ['the document is hidden', (s: ReturnType<typeof setup>['state']) => (s.hidden = true)],
    ['the user is typing', (s: ReturnType<typeof setup>['state']) => (s.typing = true)],
  ])('reads no screen while %s, but still publishes the cards', async (_name, pause) => {
    const t = setup([card(1)])
    t.feed.measure(1, { cols: 80, rows: 20 })
    t.feed.visibility(1, true)
    pause(t.state)
    await t.feed.cycle()
    expect(t.screenCalls).toEqual([])
    expect(t.state.published.cards.map((c) => c.PID)).toEqual([1])
  })

  it('forgets the screens of the cards that are gone', async () => {
    const t = setup([card(1), card(2)])
    for (const pid of [1, 2]) {
      t.feed.measure(pid, { cols: 80, rows: 20 })
      t.feed.visibility(pid, true)
    }
    await t.feed.cycle()
    expect(Object.keys(t.state.published.screens).sort()).toEqual(['1', '2'])
    t.state.list = [card(1)]
    await t.feed.cycle()
    expect(Object.keys(t.state.published.screens)).toEqual(['1'])
  })

  it('requests a cycle when a card becomes visible or its measure changes', () => {
    const t = setup()
    t.feed.visibility(1, true)
    expect(t.requestRefresh).toHaveBeenCalledTimes(1)
    t.feed.visibility(1, true)
    expect(t.requestRefresh).toHaveBeenCalledTimes(1)
    t.feed.measure(1, { cols: 80, rows: 20 })
    expect(t.requestRefresh).toHaveBeenCalledTimes(2)
    t.feed.measure(1, { cols: 80, rows: 20 })
    expect(t.requestRefresh).toHaveBeenCalledTimes(2)
    t.feed.measure(1, { cols: 90, rows: 20 })
    expect(t.requestRefresh).toHaveBeenCalledTimes(3)
    t.feed.visibility(1, false)
    t.feed.measure(1, null)
    expect(t.requestRefresh).toHaveBeenCalledTimes(3)
  })
})

describe('unavailable screens', () => {
  async function failing(cycles: number) {
    const t = setup()
    t.feed.measure(1, { cols: 80, rows: 20 })
    t.feed.visibility(1, true)
    t.state.screens.set(1, new Error('nope'))
    for (let i = 0; i < cycles; i++) await t.feed.cycle()
    return t
  }

  it('keeps a card available below the failure threshold', async () => {
    const t = await failing(FAILURE_THRESHOLD - 1)
    expect(t.state.published.unavailable.has(1)).toBe(false)
  })

  it('marks a card unavailable at the failure threshold', async () => {
    const t = await failing(FAILURE_THRESHOLD)
    expect(t.state.published.unavailable.has(1)).toBe(true)
  })

  it('makes a card available again after a success', async () => {
    const t = await failing(FAILURE_THRESHOLD)
    t.state.screens.set(1, screen('back'))
    await t.feed.cycle()
    expect(t.state.published.unavailable.has(1)).toBe(false)
    expect(t.state.published.screens[1].Hash).toBe('back')
  })

  it('counts the failures in a row only', async () => {
    const t = await failing(FAILURE_THRESHOLD - 1)
    t.state.screens.set(1, screen('ok'))
    await t.feed.cycle()
    t.state.screens.set(1, new Error('nope'))
    await t.feed.cycle()
    expect(t.state.published.unavailable.has(1)).toBe(false)
  })

  it('forgets the failures of a card that is gone', async () => {
    const t = await failing(FAILURE_THRESHOLD - 1)
    t.state.list = []
    await t.feed.cycle()
    t.state.list = [card(1)]
    await t.feed.cycle()
    expect(t.state.published.unavailable.has(1)).toBe(false)
  })

  it('does not publish the set again when nothing changed', async () => {
    const t = await failing(FAILURE_THRESHOLD)
    const kept = t.state.published.unavailable
    await t.feed.cycle()
    expect(t.state.published.unavailable).toBe(kept)
  })
})

describe('fit', () => {
  const WANTED = { cols: 100, rows: 30 }

  function ready(extra: Partial<core.Card> = {}) {
    const t = setup([card(1, extra)])
    t.feed.measure(1, { cols: 80, rows: 20 })
    t.feed.visibility(1, true)
    t.feed.fit(1, WANTED)
    return t
  }

  it('asks for the wanted size when the screen has another one', async () => {
    const t = ready()
    await t.feed.cycle()
    expect(t.fitCalls).toEqual([[1, 100, 30]])
  })

  it('asks nothing when the screen already has the wanted size', async () => {
    const t = ready()
    t.state.screens.set(1, screen('same', 100, 30))
    await t.feed.cycle()
    expect(t.fitCalls).toEqual([])
  })

  it('asks nothing for a card that wants no size', async () => {
    const t = ready()
    t.feed.fit(1, null)
    await t.feed.cycle()
    expect(t.fitCalls).toEqual([])
  })

  it('asks nothing for a session a client shows', async () => {
    const t = ready({ Attached: true })
    await t.feed.cycle()
    expect(t.fitCalls).toEqual([])
  })

  it('asks nothing while the fullscreen view is open', async () => {
    const t = ready()
    t.state.fullscreen = true
    await t.feed.cycle()
    expect(t.fitCalls).toEqual([])
  })

  it('asks nothing when the fullscreen view opens during the screen read', async () => {
    const t = ready()
    t.state.onScreenRead = () => (t.state.fullscreen = true)
    await t.feed.cycle()
    expect(t.screenCalls).toEqual([[1, 20]])
    expect(t.fitCalls).toEqual([])
  })

  it('does not ask the same size again once it was applied, until the recheck delay', async () => {
    const t = ready()
    await t.feed.cycle()
    await t.feed.cycle()
    expect(t.fitCalls).toHaveLength(1)
    t.advance(5 * 60_000)
    await t.feed.cycle()
    expect(t.fitCalls).toHaveLength(2)
  })

  it('asks again on the next cycle when the size was kept', async () => {
    const t = ready()
    t.state.fitResult = false
    await t.feed.cycle()
    await t.feed.cycle()
    expect(t.fitCalls).toHaveLength(2)
  })

  it('waits before asking again after a failure', async () => {
    const t = ready()
    t.state.fitResult = new Error('nope')
    await t.feed.cycle()
    await t.feed.cycle()
    expect(t.fitCalls).toHaveLength(1)
    t.advance(30_000)
    await t.feed.cycle()
    expect(t.fitCalls).toHaveLength(2)
  })

  it('does not break the cycle when the fit fails', async () => {
    const t = setup([card(1), card(2)])
    for (const pid of [1, 2]) {
      t.feed.measure(pid, { cols: 80, rows: 20 })
      t.feed.visibility(pid, true)
      t.feed.fit(pid, WANTED)
    }
    t.state.fitResult = new Error('nope')
    await t.feed.cycle()
    expect(t.fitCalls.map((c) => c[0])).toEqual([1, 2])
    expect(Object.keys(t.state.published.screens)).toEqual(['1', '2'])
  })

  it('asks again after the fit of a card was cleared', async () => {
    const t = ready()
    await t.feed.cycle()
    t.feed.fit(1, null)
    t.feed.fit(1, WANTED)
    await t.feed.cycle()
    expect(t.fitCalls).toHaveLength(2)
  })

  it('asks again after the fit requests of a card were forgotten', async () => {
    const t = ready()
    await t.feed.cycle()
    t.feed.forgetFit(1)
    await t.feed.cycle()
    expect(t.fitCalls).toHaveLength(2)
  })

  it('forgets the fit requests of the cards that are gone', async () => {
    const t = ready()
    await t.feed.cycle()
    t.state.list = []
    await t.feed.cycle()
    t.state.list = [card(1)]
    await t.feed.cycle()
    expect(t.fitCalls).toHaveLength(2)
  })
})
