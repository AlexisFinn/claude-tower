import { describe, expect, it } from 'vitest'
import {
  createInputGate,
  createInputQueue,
  openTerminalSession,
  type TerminalBindings,
} from './terminalSession'

const flush = () => new Promise((resolve) => setTimeout(resolve, 0))

function fakeBindings(log: string[]) {
  const listeners = new Map<string, (...data: any[]) => void>()
  const registered = new Map<string, (...data: any[]) => void>()
  const b: TerminalBindings = {
    OpenTerminal: async (pid, cols, rows) => {
      log.push(`open ${pid} ${cols}x${rows}`)
      return 7
    },
    TermReady: async (id) => void log.push(`ready ${id}`),
    TermInput: async (id, data) => void log.push(`input ${id} ${JSON.stringify(data)}`),
    TermResize: async (id, c, r) => void log.push(`resize ${id} ${c}x${r}`),
    CloseTerminal: async (id) => void log.push(`close ${id}`),
    EventsOn: (name, cb) => {
      log.push(`on ${name}`)
      listeners.set(name, cb)
      registered.set(name, cb)
      return () => {
        log.push(`off ${name}`)
        listeners.delete(name)
      }
    },
  }
  return { b, listeners, registered }
}

describe('openTerminalSession', () => {
  it('opens, subscribes, then signals ready, in that order', async () => {
    const log: string[] = []
    const { b } = fakeBindings(log)
    await openTerminalSession(b, 42, 120, 30, { onData: () => {}, onExit: () => {} })
    expect(log).toEqual(['open 42 120x30', 'on term:7:data', 'on term:7:exit', 'ready 7'])
  })

  it('decodes data events into bytes', async () => {
    const log: string[] = []
    const { b, listeners } = fakeBindings(log)
    const got: number[][] = []
    await openTerminalSession(b, 1, 80, 24, {
      onData: (bytes) => got.push([...bytes]),
      onExit: () => {},
    })
    listeners.get('term:7:data')!('4oKs') // "€"
    expect(got).toEqual([[0xe2, 0x82, 0xac]])
  })

  it('reports the exit event', async () => {
    const log: string[] = []
    const { b, listeners } = fakeBindings(log)
    let exits = 0
    await openTerminalSession(b, 1, 80, 24, { onData: () => {}, onExit: () => exits++ })
    listeners.get('term:7:exit')!()
    expect(exits).toBe(1)
  })

  it('closes the terminal, then unsubscribes, once', async () => {
    const log: string[] = []
    const { b } = fakeBindings(log)
    const s = await openTerminalSession(b, 1, 80, 24, { onData: () => {}, onExit: () => {} })
    log.length = 0
    await s.close()
    await s.close()
    expect(log).toEqual(['close 7', 'off term:7:data', 'off term:7:exit'])
  })

  it('cleans up when TermReady fails', async () => {
    const log: string[] = []
    const { b } = fakeBindings(log)
    b.TermReady = async () => {
      throw new Error('nope')
    }
    await expect(
      openTerminalSession(b, 1, 80, 24, { onData: () => {}, onExit: () => {} }),
    ).rejects.toThrow('nope')
    expect(log).toContain('close 7')
    expect(log).toContain('off term:7:data')
  })

  it('sends input and resizes through the bindings', async () => {
    const log: string[] = []
    const { b } = fakeBindings(log)
    const s = await openTerminalSession(b, 1, 80, 24, { onData: () => {}, onExit: () => {} })
    log.length = 0
    s.input('ls\r')
    s.resize(100, 40)
    await flush()
    expect(log).toEqual(['input 7 "ls\\r"', 'resize 7 100x40'])
  })

  it('ignores input and resize after close', async () => {
    const log: string[] = []
    const { b } = fakeBindings(log)
    const s = await openTerminalSession(b, 1, 80, 24, { onData: () => {}, onExit: () => {} })
    await s.close()
    log.length = 0
    s.input('ls\r')
    s.resize(100, 40)
    await flush()
    expect(log).toEqual([])
  })

  it('drops the keystrokes still queued when it closes', async () => {
    const log: string[] = []
    const { b } = fakeBindings(log)
    let release!: () => void
    b.TermInput = (id, data) =>
      new Promise<void>((resolve) => {
        log.push(`input ${id} ${JSON.stringify(data)}`)
        release = resolve
      })
    const s = await openTerminalSession(b, 1, 80, 24, { onData: () => {}, onExit: () => {} })
    log.length = 0
    s.input('a')
    s.input('b')
    await s.close()
    release()
    await flush()
    expect(log.filter((l) => l.startsWith('input'))).toEqual(['input 7 "a"'])
  })

  it('ignores data and exit events once closing has started', async () => {
    const log: string[] = []
    const { b, registered } = fakeBindings(log)
    const got: number[][] = []
    let exits = 0
    const s = await openTerminalSession(b, 1, 80, 24, {
      onData: (bytes) => got.push([...bytes]),
      onExit: () => exits++,
    })
    const closing = s.close()
    registered.get('term:7:data')!('4oKs')
    registered.get('term:7:exit')!()
    await closing
    registered.get('term:7:data')!('4oKs')
    registered.get('term:7:exit')!()
    expect(got).toEqual([])
    expect(exits).toBe(0)
  })

  it('fails when the terminal cannot be opened', async () => {
    const log: string[] = []
    const { b } = fakeBindings(log)
    b.OpenTerminal = async () => {
      throw new Error('no pane')
    }
    await expect(
      openTerminalSession(b, 1, 80, 24, { onData: () => {}, onExit: () => {} }),
    ).rejects.toThrow('no pane')
    expect(log).toEqual([])
  })
})

describe('createInputQueue', () => {
  it('sends one call at a time and joins what arrives meanwhile', async () => {
    const calls: string[] = []
    const releases: Array<() => void> = []
    const q = createInputQueue(
      (data) =>
        new Promise<void>((resolve) => {
          calls.push(data)
          releases.push(resolve)
        }),
    )
    q.push('a')
    q.push('b')
    q.push('c')
    await flush()
    expect(calls).toEqual(['a'])
    releases[0]()
    await flush()
    expect(calls).toEqual(['a', 'bc'])
    releases[1]()
    await flush()
    q.push('d')
    await flush()
    expect(calls).toEqual(['a', 'bc', 'd'])
  })

  it('keeps going after a failed call', async () => {
    const calls: string[] = []
    let fail = true
    const q = createInputQueue(async (data) => {
      calls.push(data)
      if (fail) {
        fail = false
        throw new Error('x')
      }
    })
    q.push('a')
    await flush()
    q.push('b')
    await flush()
    expect(calls).toEqual(['a', 'b'])
  })

  it('drops pending data on clear', async () => {
    const calls: string[] = []
    let release!: () => void
    const q = createInputQueue(
      (data) =>
        new Promise<void>((resolve) => {
          calls.push(data)
          release = resolve
        }),
    )
    q.push('a')
    q.push('b')
    q.clear()
    release()
    await flush()
    expect(calls).toEqual(['a'])
  })
})

describe('createInputGate', () => {
  it('holds keystrokes until opened, then sends them joined and in order', () => {
    const sent: string[] = []
    const gate = createInputGate()
    gate.push('a')
    gate.push('b')
    expect(sent).toEqual([])
    gate.open((d) => sent.push(d))
    expect(sent).toEqual(['ab'])
  })

  it('passes keystrokes straight through once opened', () => {
    const sent: string[] = []
    const gate = createInputGate()
    gate.open((d) => sent.push(d))
    gate.push('c')
    gate.push('d')
    expect(sent).toEqual(['c', 'd'])
  })

  it('sends nothing on open when nothing was typed', () => {
    const sent: string[] = []
    createInputGate().open((d) => sent.push(d))
    expect(sent).toEqual([])
  })

  it('does not resend the held keystrokes', () => {
    const sent: string[] = []
    const gate = createInputGate()
    gate.push('a')
    gate.open((d) => sent.push(d))
    gate.push('b')
    expect(sent).toEqual(['a', 'b'])
  })
})
