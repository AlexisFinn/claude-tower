/**
 * Calls onChange with the new device pixel ratio each time it changes, for
 * example when the window moves to another screen or the page is zoomed.
 * Returns a function that stops watching.
 */
export function watchDevicePixelRatio(onChange: (dpr: number) => void): () => void {
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
