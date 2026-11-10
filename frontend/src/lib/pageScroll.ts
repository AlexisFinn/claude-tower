const listeners = new Set<() => void>()

const onScroll = (): void => {
  for (const l of [...listeners]) l()
}

/**
 * Calls listener each time the page or any element in it scrolls. All the
 * listeners share one capturing event listener on the window, which exists
 * while one is subscribed. Returns a function that unsubscribes.
 */
export function onPageScroll(listener: () => void): () => void {
  const entry = (): void => listener()
  if (listeners.size === 0) window.addEventListener('scroll', onScroll, { capture: true, passive: true })
  listeners.add(entry)
  return () => {
    listeners.delete(entry)
    if (listeners.size === 0) window.removeEventListener('scroll', onScroll, { capture: true })
  }
}
