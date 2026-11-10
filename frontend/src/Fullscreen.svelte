<script lang="ts">
  import { onMount } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { Unicode11Addon } from '@xterm/addon-unicode11'
  import { BrowserClipboardProvider, ClipboardAddon } from '@xterm/addon-clipboard'
  import * as App from '../wailsjs/go/app/App'
  import type { core } from '../wailsjs/go/models'
  import { EventsOn } from '../wailsjs/runtime/runtime'
  import Icon from './Icon.svelte'
  import StatePill from './StatePill.svelte'
  import { errorMessage } from './lib/errors'
  import { terminalKeyAction } from './lib/keys'
  import { DEFAULT_BG, DEFAULT_FG } from './lib/termPalette'
  import { createInputGate, openTerminalSession, type TerminalSession } from './lib/terminalSession'

  let { card, onExit }: { card: core.Card; onExit: () => void } = $props()

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
      theme: { background: DEFAULT_BG, foreground: DEFAULT_FG },
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

    openTerminalSession(bindings, card.PID, term.cols, term.rows, {
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
    <button class="btn back" title="Retour à la grille" onclick={onExit}>
      <Icon name="arrowLeft" />
      Retour à la grille
      <span class="keys"><kbd>Ctrl</kbd><kbd>Maj</kbd><kbd>Q</kbd></span>
    </button>
    <span class="name">{card.Name}</span>
    <StatePill state={card.State} label={card.Label} />
    <span class="cwd"><Icon name="folder" size={14} /><span class="path">{card.Cwd}</span></span>
    {#if error !== ''}<span class="error">{error}</span>{/if}
  </header>
  <div class="stage">
    <div class="host" bind:this={host}></div>
  </div>
</div>

<style>
  .fullscreen {
    position: fixed;
    inset: 0;
    z-index: 10;
    display: flex;
    flex-direction: column;
    background: var(--screen);
    animation: enter 150ms var(--ease);
  }

  header {
    flex: none;
    display: flex;
    align-items: center;
    gap: 12px;
    height: 48px;
    padding: 0 16px 0 12px;
    background: var(--surface-2);
    border-bottom: 1px solid var(--border);
  }

  .back {
    gap: 8px;
  }

  .keys {
    display: inline-flex;
    gap: 3px;
  }

  .name {
    min-width: 0;
    overflow: hidden;
    font-size: 14px;
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cwd {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    min-width: 0;
    color: var(--text-2);
  }

  .path {
    overflow: hidden;
    font-family: var(--mono);
    font-size: 11.5px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .error {
    margin-left: auto;
    color: var(--danger);
  }

  .stage {
    flex: 1;
    display: flex;
    min-height: 0;
    padding: 8px;
  }

  .host {
    flex: 1;
    min-width: 0;
    min-height: 0;
  }

  @keyframes enter {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }
</style>
