<script lang="ts">
  import Icon from './Icon.svelte'
  import logo from './assets/logo.svg'
  import { frameClass } from './lib/frame'
  import type { StateCount } from './lib/summary'

  let { summary, onHelp, onNew }: { summary: StateCount[]; onHelp: () => void; onNew: () => void } = $props()
</script>

<header class="appbar">
  <div class="brand">
    <img src={logo} alt="" width="20" height="20" />
    <span class="title">Claude Tower</span>
  </div>

  <ul class="summary" aria-label="Résumé des sessions">
    {#each summary as item (item.state)}
      <li class="chip {frameClass(item.state, false)}" class:pulse={item.state === 'waiting'}>
        <span class="dot"></span>
        <strong>{item.count}</strong>
        <span>{item.label}</span>
      </li>
    {/each}
  </ul>

  <div class="actions">
    <button class="icon-btn" aria-label="Raccourcis clavier" title="Raccourcis clavier (?)" onclick={onHelp}>
      <Icon name="help" />
    </button>
    <button class="btn btn-primary" title="Nouvelle session (n)" onclick={onNew}>
      <Icon name="plus" />
      Nouvelle session
    </button>
  </div>
</header>

<style>
  .appbar {
    flex: none;
    display: flex;
    align-items: center;
    gap: 20px;
    height: 48px;
    padding: 0 12px 0 16px;
    background: var(--surface-2);
    border-bottom: 1px solid var(--border);
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 9px;
  }

  .title {
    font-size: 14px;
    font-weight: 600;
    letter-spacing: 0.1px;
  }

  .summary {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    margin: 0;
    padding: 0;
    list-style: none;
    overflow: hidden;
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 3px 10px 3px 8px;
    font-size: 12px;
    color: var(--text-2);
    white-space: nowrap;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 999px;
  }

  .chip strong {
    font-weight: 600;
    color: var(--text);
  }

  .dot {
    width: 7px;
    height: 7px;
    background: var(--state);
    border-radius: 50%;
  }

  .pulse {
    border-color: color-mix(in srgb, var(--state) 45%, transparent);
  }

  .pulse .dot {
    animation: ripple 2.4s ease-in-out infinite;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-left: auto;
  }

  @keyframes ripple {
    0% {
      box-shadow: 0 0 0 0 color-mix(in srgb, var(--state) 55%, transparent);
    }
    70%,
    100% {
      box-shadow: 0 0 0 6px transparent;
    }
  }
</style>
