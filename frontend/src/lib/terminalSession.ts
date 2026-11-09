import { decodeBase64 } from './base64'

/** The bound methods and runtime events a full-screen terminal needs. */
export interface TerminalBindings {
  OpenTerminal(pid: number, cols: number, rows: number): Promise<number>
  TermReady(id: number): Promise<void>
  TermInput(id: number, data: string): Promise<void>
  TermResize(id: number, cols: number, rows: number): Promise<void>
  CloseTerminal(id: number): Promise<void>
  EventsOn(name: string, callback: (...data: any[]) => void): () => void
}

/** Receives what the session writes. */
export interface TerminalHandlers {
  onData(bytes: Uint8Array): void
  onExit(): void
}

/** An open full-screen terminal. */
export interface TerminalSession {
  readonly id: number
  input(data: string): void
  resize(cols: number, rows: number): void
  close(): Promise<void>
}

/** A serial sender: one call at a time, with the keystrokes that arrive meanwhile joined. */
export interface InputQueue {
  push(data: string): void
  clear(): void
}

/**
 * Creates a queue that calls send with one string at a time. Bound calls may
 * run in any order, so the next call starts only after the previous one
 * returned; what was pushed in between is sent in a single call.
 */
export function createInputQueue(send: (data: string) => Promise<void>): InputQueue {
  let pending = ''
  let busy = false

  async function pump(): Promise<void> {
    busy = true
    try {
      while (pending !== '') {
        const data = pending
        pending = ''
        try {
          await send(data)
        } catch {
          // the exit event reports a dead session
        }
      }
    } finally {
      busy = false
    }
  }

  return {
    push(data) {
      pending += data
      if (!busy) void pump()
    },
    clear() {
      pending = ''
    },
  }
}

/** Holds keystrokes until the session is ready, then forwards them in order. */
export interface InputGate {
  push(data: string): void
  open(send: (data: string) => void): void
}

/**
 * Creates a gate that joins the keystrokes pushed before open into a single
 * send, then passes the later ones straight through.
 */
export function createInputGate(): InputGate {
  let pending = ''
  let send: ((data: string) => void) | null = null
  return {
    push(data) {
      if (send) send(data)
      else pending += data
    },
    open(target) {
      send = target
      if (pending === '') return
      const data = pending
      pending = ''
      target(data)
    },
  }
}

/**
 * Opens a terminal on the session of pid. The listeners are registered
 * before TermReady, which releases the output the backend holds back.
 */
export async function openTerminalSession(
  b: TerminalBindings,
  pid: number,
  cols: number,
  rows: number,
  handlers: TerminalHandlers,
): Promise<TerminalSession> {
  const id = await b.OpenTerminal(pid, cols, rows)
  let closed = false
  const offData = b.EventsOn(`term:${id}:data`, (data: string) => {
    if (!closed) handlers.onData(decodeBase64(data))
  })
  const offExit = b.EventsOn(`term:${id}:exit`, () => {
    if (!closed) handlers.onExit()
  })
  const queue = createInputQueue((data) => b.TermInput(id, data))

  const close = async (): Promise<void> => {
    if (closed) return
    closed = true
    queue.clear()
    try {
      await b.CloseTerminal(id)
    } catch {
      // the view may already be gone
    }
    offData()
    offExit()
  }

  try {
    await b.TermReady(id)
  } catch (e) {
    await close()
    throw e
  }
  return {
    id,
    input: (data) => {
      if (!closed) queue.push(data)
    },
    resize: (c, r) => {
      if (!closed) void b.TermResize(id, c, r).catch(() => {})
    },
    close,
  }
}
