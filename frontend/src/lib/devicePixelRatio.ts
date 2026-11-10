const listeners = new Set<(dpr: number) => void>()
let stopWindowWatch: (() => void) | null = null

/** Watches the window itself: calls onChange with the new device pixel ratio each time it changes. */
function watchWindow(onChange: (dpr: number) => void): () => void {
  let query: MediaQueryList | null = null
  let stopped = false

  const listen = (): void => {
    query = window.matchMedia(`(resolution: ${window.devicePixelRatio}dppx)`)
    query.addEventListener('change', changed, { once: true })
  }

  const changed = (): void => {
    if (stopped) return
    onChange(window.devicePixelRatio)
    listen()
  }

  listen()
  return () => {
    stopped = true
    query?.removeEventListener('change', changed)
  }
}

/**
 * Calls onChange with the new device pixel ratio each time it changes, for
 * example when the window moves to another screen or the page is zoomed.
 * All the watchers share one media query, which exists while one is active.
 * Returns a function that stops watching.
 */
export function watchDevicePixelRatio(onChange: (dpr: number) => void): () => void {
  const listener = (dpr: number): void => onChange(dpr)
  listeners.add(listener)
  stopWindowWatch ??= watchWindow((dpr) => {
    for (const l of [...listeners]) l(dpr)
  })
  return () => {
    listeners.delete(listener)
    if (listeners.size > 0) return
    stopWindowWatch?.()
    stopWindowWatch = null
  }
}
