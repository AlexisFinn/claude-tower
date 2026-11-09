<script lang="ts">
  import Icon from './Icon.svelte'
  import type { Toast } from './lib/toasts'

  let { toasts, onDismiss }: { toasts: Toast[]; onDismiss: (id: number) => void } = $props()
</script>

<div class="stack">
  {#each toasts as toast (toast.id)}
    <div class="toast {toast.kind}" role={toast.kind === 'error' ? 'alert' : 'status'}>
      <span class="kind"><Icon name={toast.kind} /></span>
      <span class="text">{toast.text}</span>
      <button class="icon-btn" aria-label="Fermer la notification" onclick={() => onDismiss(toast.id)}>
        <Icon name="x" />
      </button>
    </div>
  {/each}
</div>

<style>
  .stack {
    position: absolute;
    right: 12px;
    bottom: 8px;
    z-index: 30;
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 8px;
    max-width: min(420px, calc(100% - 24px));
    pointer-events: none;
  }

  .toast {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 10px 6px 10px 12px;
    pointer-events: auto;
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow);
    animation: enter 160ms var(--ease);
  }

  .kind {
    margin-top: 1px;
    color: var(--text-2);
  }

  .warning .kind {
    color: var(--busy);
  }

  .error .kind {
    color: var(--danger);
  }

  .text {
    flex: 1;
    min-width: 0;
    padding-top: 1px;
    overflow-wrap: anywhere;
  }

  @keyframes enter {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
  }
</style>
