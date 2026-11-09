<script lang="ts">
  let { message }: { message: string } = $props()
</script>

<main>
  <h1>claude-tower ne peut pas démarrer</h1>
  <pre class="message">{message}</pre>
  <p>tmux 3.2 ou plus récent est nécessaire. Pour l'installer :</p>
  <ul>
    <li>Fedora : <code>sudo dnf install tmux</code></li>
    <li>Debian, Ubuntu : <code>sudo apt install tmux</code></li>
    <li>macOS : <code>brew install tmux</code></li>
  </ul>
  <p>Relancez ensuite l'application.</p>
</main>

<style>
  main {
    max-width: 720px;
    margin: 0 auto;
    padding: 48px 24px;
    line-height: 1.6;
  }

  h1 {
    font-size: 18px;
    color: var(--danger);
  }

  .message {
    padding: 12px;
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 4px;
    white-space: pre-wrap;
  }

  code {
    padding: 2px 6px;
    background: var(--panel);
    border-radius: 3px;
  }
</style>
