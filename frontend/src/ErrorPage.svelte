<script lang="ts">
  import { onDestroy } from 'svelte'
  import Icon from './Icon.svelte'

  let { message }: { message: string } = $props()

  const COMMANDS = [
    { system: 'Fedora', command: 'sudo dnf install tmux' },
    { system: 'Debian, Ubuntu', command: 'sudo apt install tmux' },
    { system: 'macOS', command: 'brew install tmux' },
  ]

  let copied = $state<string | null>(null)
  let timer: ReturnType<typeof setTimeout> | undefined

  async function copy(command: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(command)
    } catch {
      return
    }
    copied = command
    clearTimeout(timer)
    timer = setTimeout(() => (copied = null), 1500)
  }

  onDestroy(() => clearTimeout(timer))
</script>

<main>
  <div class="card">
    <div class="head">
      <span class="alert"><Icon name="warning" size={22} /></span>
      <div>
        <h1>claude-tower ne peut pas démarrer</h1>
        <p class="lead">tmux 3.2 ou plus récent est nécessaire.</p>
      </div>
    </div>

    <pre class="message">{message}</pre>

    <p class="lead">Pour l'installer :</p>
    <ul>
      {#each COMMANDS as item (item.system)}
        <li>
          <span class="system">{item.system}</span>
          <code>{item.command}</code>
          <button class="btn" onclick={() => void copy(item.command)}>
            <Icon name={copied === item.command ? 'check' : 'copy'} />
            {copied === item.command ? 'Copié' : 'Copier'}
          </button>
        </li>
      {/each}
    </ul>

    <p class="lead">Relancez ensuite l'application.</p>
  </div>
</main>

<style>
  main {
    display: flex;
    justify-content: center;
    padding: 56px 24px;
  }

  .card {
    width: 100%;
    max-width: 640px;
    padding: 24px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow);
  }

  .head {
    display: flex;
    align-items: center;
    gap: 14px;
  }

  .alert {
    display: flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 44px;
    height: 44px;
    color: var(--danger);
    background: color-mix(in srgb, var(--danger) 14%, transparent);
    border-radius: var(--radius-md);
  }

  h1 {
    margin: 0;
    font-size: 17px;
    font-weight: 600;
  }

  .lead {
    margin: 2px 0 0;
    color: var(--text-2);
  }

  .message {
    margin: 18px 0;
    padding: 12px;
    font-family: var(--mono);
    font-size: 12px;
    white-space: pre-wrap;
    background: var(--screen);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  ul {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin: 10px 0 18px;
    padding: 0;
    list-style: none;
  }

  li {
    display: grid;
    grid-template-columns: 120px 1fr auto;
    align-items: center;
    gap: 12px;
  }

  .system {
    color: var(--text-2);
  }

  code {
    padding: 6px 10px;
    font-size: 12px;
    background: var(--screen);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
  }
</style>
