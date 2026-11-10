import { realTimers, type Timers } from './timers'

/** The kind of a notification. */
export type ToastKind = 'info' | 'warning' | 'error'

/** A notification shown until it expires or is dismissed. */
export interface Toast {
  id: number
  kind: ToastKind
  text: string
}

/** How long a notification stays, in milliseconds. */
export const TOAST_DURATION_MS: Record<ToastKind, number> = {
  info: 5000,
  warning: 5000,
  error: 8000,
}

/** The most notifications shown at once; the oldest ones are dropped first. */
export const MAX_TOASTS = 4

/** A queue of notifications that expire on their own. */
export interface ToastQueue {
  /** Shows a notification. The same text of the same kind is not duplicated: its expiry restarts. */
  push: (kind: ToastKind, text: string) => number
  dismiss: (id: number) => void
  /** Cancels every pending expiry. */
  dispose: () => void
}

/** Creates a queue that reports the current list of notifications on every change. */
export function createToasts(onChange: (toasts: Toast[]) => void, timers: Timers = realTimers): ToastQueue {
  let toasts: Toast[] = []
  let nextId = 1
  const expiries = new Map<number, unknown>()

  function emit(): void {
    onChange([...toasts])
  }

  function arm(toast: Toast): void {
    const pending = expiries.get(toast.id)
    if (pending !== undefined) timers.clear(pending)
    expiries.set(
      toast.id,
      timers.set(() => dismiss(toast.id), TOAST_DURATION_MS[toast.kind]),
    )
  }

  function dismiss(id: number): void {
    const pending = expiries.get(id)
    if (pending !== undefined) timers.clear(pending)
    expiries.delete(id)
    if (!toasts.some((t) => t.id === id)) return
    toasts = toasts.filter((t) => t.id !== id)
    emit()
  }

  function push(kind: ToastKind, text: string): number {
    const existing = toasts.find((t) => t.kind === kind && t.text === text)
    if (existing) {
      arm(existing)
      return existing.id
    }
    const toast: Toast = { id: nextId++, kind, text }
    const kept = [...toasts, toast]
    for (const dropped of kept.slice(0, Math.max(0, kept.length - MAX_TOASTS))) {
      const pending = expiries.get(dropped.id)
      if (pending !== undefined) timers.clear(pending)
      expiries.delete(dropped.id)
    }
    toasts = kept.slice(-MAX_TOASTS)
    arm(toast)
    emit()
    return toast.id
  }

  function dispose(): void {
    for (const pending of expiries.values()) timers.clear(pending)
    expiries.clear()
  }

  return { push, dismiss, dispose }
}
