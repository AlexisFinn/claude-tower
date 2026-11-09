<script lang="ts">
  import Icon from './Icon.svelte'

  let { onNew }: { onNew: () => void } = $props()
</script>

<div class="empty">
  <svg class="art" width="160" height="160" viewBox="0 0 160 160" fill="none" aria-hidden="true" focusable="false">
    <rect x="6" y="104" width="44" height="30" rx="6" stroke="var(--border-strong)" stroke-width="1.5" stroke-dasharray="4 4" />
    <rect x="110" y="104" width="44" height="30" rx="6" stroke="var(--border-strong)" stroke-width="1.5" stroke-dasharray="4 4" />
    <path d="M52 140V30H62V46H72V30H88V46H98V30H108V140" stroke="var(--accent)" stroke-width="1.5" stroke-linejoin="round" />
    <path d="M30 140H130" stroke="var(--accent)" stroke-width="1.5" stroke-linecap="round" />
    <rect x="72" y="74" width="16" height="26" rx="3" stroke="var(--accent)" stroke-width="1.5" />
    <path d="M62 16a24 24 0 0 1 36 0" stroke="var(--text-3)" stroke-width="1.5" stroke-linecap="round" />
    <path d="M70 22a13 13 0 0 1 20 0" stroke="var(--text-3)" stroke-width="1.5" stroke-linecap="round" />
  </svg>
  <h1>Aucune session Claude en cours</h1>
  <p>Lancez une session ici, ou utilisez <code>claude-tower run</code> dans un terminal.</p>
  <button class="btn btn-primary" onclick={onNew}>
    <Icon name="plus" />
    Nouvelle session
  </button>
</div>

<style>
  .empty {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    padding: 24px;
    text-align: center;
  }

  h1 {
    margin: 8px 0 0;
    font-size: 18px;
    font-weight: 600;
  }

  p {
    max-width: 420px;
    margin: 0 0 8px;
    color: var(--text-2);
    line-height: 1.6;
  }

  code {
    padding: 1px 6px;
    font-size: 12px;
    color: var(--text);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: 4px;
  }
</style>
