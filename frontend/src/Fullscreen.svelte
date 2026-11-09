<script lang="ts">
  import { onMount } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { Unicode11Addon } from '@xterm/addon-unicode11'
  import { BrowserClipboardProvider, ClipboardAddon } from '@xterm/addon-clipboard'
  import * as App from '../wailsjs/go/app/App'
  import { EventsOn } from '../wailsjs/runtime/runtime'
  import { errorMessage } from './lib/errors'
  import { terminalKeyAction } from './lib/keys'
  import { createInputGate, openTerminalSession, type TerminalSession } from './lib/terminalSession'

  let { pid, name, onExit }: { pid: number; name: string; onExit: () => void } = $props()

  let host: HTMLDivElement
  let error = $state('')

  const bindings = {
    OpenTerminal: App.OpenTerminal,
    TermReady: App.TermReady,
    TermInput: App.TermInput,
    TermResize: App.TermResize,
    CloseTerminal: App.CloseTerminal,
    EventsOn,
  }

  const browserClipboard = new BrowserClipboardProvider()
  // The session cannot read the local clipboard: reads are empty, writes are allowed.
  const writeOnlyClipboard = {
    readText: () => '',
    writeText: (selection: Parameters<BrowserClipboardProvider['writeText']>[0], text: string) =>
      browserClipboard.writeText(selection, text),
  }

  onMount(() => {
    const term = new Terminal({
      fontSize: 13,
      fontFamily: 'ui-monospace, "SFMono-Regular", Menlo, "DejaVu Sans Mono", monospace',
      cursorBlink: true,
      allowProposedApi: true,
      theme: { background: '#0d1117', foreground: '#e6edf3' },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.loadAddon(new Unicode11Addon())
    term.unicode.activeVersion = '11'
    term.loadAddon(new ClipboardAddon(undefined, writeOnlyClipboard))
    term.open(host)
    fit.fit()
    term.focus()

    let session: TerminalSession | null = null
    let disposed = false
    const gate = createInputGate()

    openTerminalSession(bindings, pid, term.cols, term.rows, {
      onData: (bytes) => {
        if (!disposed) term.write(bytes)
      },
      onExit: () => onExit(),
    })
      .then((s) => {
        if (disposed) {
          void s.close()
          return
        }
        session = s
        s.resize(term.cols, term.rows)
        gate.open((data) => s.input(data))
      })
      .catch((e) => {
        error = errorMessage(e)
      })

    term.onData((data) => gate.push(data))
    term.onResize(({ cols, rows }) => session?.resize(cols, rows))

    const copySelection = (): void => {
      const text = term.getSelection()
      if (text !== '') void navigator.clipboard?.writeText(text).catch(() => {})
    }
    const paste = (): void => {
      void navigator.clipboard
        ?.readText()
        .then((text) => term.paste(text))
        .catch(() => {})
    }

    term.attachCustomKeyEventHandler((e) => {
      const action = terminalKeyAction(e)
      if (action === null) return true
      e.preventDefault()
      if (e.type === 'keydown') {
        if (action === 'newline') gate.push('\x1b\r')
        else if (action === 'copy') copySelection()
        else if (action === 'paste') paste()
      }
      return false
    })

    const onWindowKey = (e: KeyboardEvent): void => {
      if (terminalKeyAction(e) !== 'exit') return
      e.preventDefault()
      e.stopPropagation()
      if (e.type === 'keydown') onExit()
    }
    window.addEventListener('keydown', onWindowKey, true)
    window.addEventListener('keyup', onWindowKey, true)
    window.addEventListener('keypress', onWindowKey, true)

    let pendingFit = 0
    const observer = new ResizeObserver(() => {
      cancelAnimationFrame(pendingFit)
      pendingFit = requestAnimationFrame(() => fit.fit())
    })
    observer.observe(host)

    return () => {
      disposed = true
      window.removeEventListener('keydown', onWindowKey, true)
      window.removeEventListener('keyup', onWindowKey, true)
      window.removeEventListener('keypress', onWindowKey, true)
      observer.disconnect()
      cancelAnimationFrame(pendingFit)
      if (session) void session.close().finally(() => term.dispose())
      else term.dispose()
    }
  })
</script>

<div class="fullscreen">
  <header>
    <span class="name">{name}</span>
    {#if error !== ''}<span class="error">{error}</span>{/if}
    <button onclick={onExit}>Retour à la grille (Ctrl+Maj+Q)</button>
  </header>
  <div class="host" bind:this={host}></div>
</div>

<style>
  .fullscreen {
    position: fixed;
    inset: 0;
    z-index: 10;
    display: flex;
    flex-direction: column;
    background: var(--bg);
  }

  header {
    flex: none;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 6px 12px;
    background: var(--panel);
    border-bottom: 1px solid var(--border);
  }

  .name {
    font-weight: bold;
  }

  .error {
    color: var(--danger);
  }

  button {
    margin-left: auto;
  }

  .host {
    flex: 1;
    min-height: 0;
  }
</style>
