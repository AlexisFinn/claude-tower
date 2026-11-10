import type { core } from '../../wailsjs/go/models'
import { sameSize, type GridSize } from './bodySize'
import { errorMessage } from './errors'
import { createFitRequests, type FitRequests } from './fitRequests'
import { createFailureTracker, type FailureTracker } from './screenFailures'
import { reconcileSelection } from './selection'
import { startFailureText, type StartFailure } from './startFailures'
import type { ToastKind } from './toasts'

/** The backend calls the feed makes. */
export interface ScreenFeedBackend {
  cards: () => Promise<core.Card[]>
  screen: (pid: number, rows: number) => Promise<core.Screen>
  /** Resolves to whether the session got the size. */
  fitSession: (pid: number, cols: number, rows: number) => Promise<boolean>
  startFailures: () => Promise<StartFailure[]>
}

/** What the feed publishes to the view. */
export interface ScreenFeedOutputs {
  onLoaded: () => void
  onCards: (cards: core.Card[]) => void
  onScreens: (screens: Record<number, core.Screen>) => void
  onUnavailable: (unavailable: Set<number>) => void
  onSelected: (pid: number | null) => void
  notify: (kind: ToastKind, text: string) => void
}

/** Options of createScreenFeed. */
export interface ScreenFeedOptions extends ScreenFeedBackend, ScreenFeedOutputs {
  /** Reports whether the fullscreen view is open: the screens are not read then. */
  fullscreenOpen: () => boolean
  documentHidden: () => boolean
  typing: () => boolean
  getSelected: () => number | null
  /** Asks for a cycle now. */
  requestRefresh: () => void
  failures?: FailureTracker
  fitRequests?: FitRequests
}

/** Loads the cards and the screens of the visible ones, and what the cards report about themselves. */
export interface ScreenFeed {
  /** Reads the cards, then the screen of each one that is visible and measured. */
  cycle(): Promise<void>
  /** Records the size of the body of a card, or null once it is gone. */
  measure(pid: number, size: GridSize | null): void
  /** Records the size a card wants for its session, or null when it does not want one. */
  fit(pid: number, size: GridSize | null): void
  visibility(pid: number, visible: boolean): void
  /** Forgets the sizes already asked of a session, so that its size is asked again. */
  forgetFit(pid: number): void
}

/** Creates a feed: it owns the screens, the failures and the sizes, and publishes the result. */
export function createScreenFeed(options: ScreenFeedOptions): ScreenFeed {
  const {
    fullscreenOpen,
    documentHidden,
    typing,
    getSelected,
    requestRefresh,
    failures = createFailureTracker(),
    fitRequests = createFitRequests(),
  } = options
  const sizes = new Map<number, GridSize>()
  const fits = new Map<number, GridSize>()
  const visible = new Set<number>()
  let pids: number[] = []
  let screens: Record<number, core.Screen> = {}
  let unavailable = new Set<number>()

  function syncUnavailable(list: core.Card[]): void {
    const next = new Set(list.filter((c) => failures.unavailable(c.PID)).map((c) => c.PID))
    const same = next.size === unavailable.size && [...next].every((pid) => unavailable.has(pid))
    if (!same) {
      unavailable = next
      options.onUnavailable(unavailable)
    }
  }

  function setUnavailable(pid: number, value: boolean): void {
    if (unavailable.has(pid) === value) return
    const next = new Set(unavailable)
    if (value) next.add(pid)
    else next.delete(pid)
    unavailable = next
    options.onUnavailable(unavailable)
  }

  /**
   * Resizes a session the application launched to the size its card wants,
   * when its last screen has another size. The engine leaves alone a session
   * that a client shows, such as the fullscreen view.
   */
  async function fitCard(card: core.Card, screen: core.Screen): Promise<void> {
    const size = fits.get(card.PID)
    if (!size || !fitRequests.shouldAsk(card.PID, size, { cols: screen.Width, rows: screen.Height }, card.Attached))
      return
    const ticket = fitRequests.ticket(card.PID)
    try {
      const applied = await options.fitSession(card.PID, size.cols, size.rows)
      fitRequests.record(card.PID, size, applied ? 'applied' : 'kept', ticket)
    } catch {
      fitRequests.record(card.PID, size, 'failed', ticket)
    }
  }

  async function loadScreens(list: core.Card[]): Promise<void> {
    const next: Record<number, core.Screen> = {}
    for (const card of list) {
      const known = screens[card.PID]
      if (known) next[card.PID] = known
    }
    screens = next
    options.onScreens(screens)
    failures.prune(list.map((c) => c.PID))
    fitRequests.prune(list.map((c) => c.PID))
    syncUnavailable(list)
    for (const card of list) {
      const size = sizes.get(card.PID)
      if (!card.InTmux || !size || !visible.has(card.PID)) continue
      if (fullscreenOpen() || documentHidden() || typing()) return
      try {
        const screen = await options.screen(card.PID, size.rows)
        screens = { ...screens, [card.PID]: screen }
        options.onScreens(screens)
        failures.succeed(card.PID)
        setUnavailable(card.PID, false)
        if (!fullscreenOpen()) await fitCard(card, screen)
      } catch {
        setUnavailable(card.PID, failures.fail(card.PID))
      }
    }
  }

  return {
    async cycle() {
      let list: core.Card[]
      try {
        list = [...(await options.cards())].sort((a, b) => a.Order - b.Order)
      } catch (e) {
        options.notify('error', errorMessage(e))
        return
      }
      options.onLoaded()
      try {
        for (const f of await options.startFailures()) options.notify('error', startFailureText(f))
      } catch {
        // A failed read leaves the failures for the next cycle.
      }
      const oldPids = pids
      pids = list.map((c) => c.PID)
      options.onCards(list)
      options.onSelected(reconcileSelection(oldPids, pids, getSelected()))
      await loadScreens(list)
    },
    measure(pid, size) {
      if (size === null) {
        sizes.delete(pid)
        return
      }
      const known = sizes.get(pid)
      sizes.set(pid, size)
      if (!sameSize(known, size)) requestRefresh()
    },
    fit(pid, size) {
      if (size === null) {
        fits.delete(pid)
        fitRequests.forget(pid)
      } else {
        fits.set(pid, size)
      }
    },
    visibility(pid, isVisible) {
      if (!isVisible) {
        visible.delete(pid)
        return
      }
      if (visible.has(pid)) return
      visible.add(pid)
      requestRefresh()
    },
    forgetFit(pid) {
      fitRequests.forget(pid)
    },
  }
}
