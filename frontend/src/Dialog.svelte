<script lang="ts">
  import { onMount, type Snippet } from 'svelte'

  interface Props {
    title: string
    role?: 'dialog' | 'alertdialog'
    width?: number
    onClose: () => void
    children: Snippet
  }

  let { title, role = 'dialog', width = 420, onClose, children }: Props = $props()

  const uid = $props.id()
  const titleId = `dialog-title-${uid}`
  const FOCUSABLE = 'button:not(:disabled), [href], input:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])'

  let panel: HTMLDivElement

  onMount(() => {
    const previous = document.activeElement
    const first = panel.querySelector<HTMLElement>('[data-autofocus]') ?? panel.querySelector<HTMLElement>(FOCUSABLE)
    ;(first ?? panel).focus()
    return () => {
      if (previous instanceof HTMLElement) previous.focus()
    }
  })

  function trapTab(e: KeyboardEvent): void {
    if (e.key !== 'Tab') return
    const items = [...panel.querySelectorAll<HTMLElement>(FOCUSABLE)]
    if (items.length === 0) {
      e.preventDefault()
      return
    }
    const first = items[0]
    const last = items[items.length - 1]
    if (document.activeElement === panel) {
      e.preventDefault()
      ;(e.shiftKey ? last : first).focus()
    } else if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }
</script>

<div
  class="overlay"
  role="presentation"
  onmousedown={(e) => {
    if (e.target === e.currentTarget) onClose()
  }}
>
  <div
    bind:this={panel}
    class="panel"
    {role}
    aria-modal="true"
    aria-labelledby={titleId}
    tabindex="-1"
    style:width="{width}px"
    onkeydown={trapTab}
  >
    <h2 id={titleId}>{title}</h2>
    {@render children()}
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 20;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 16px;
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(4px);
    animation: fade 150ms var(--ease);
  }

  .panel {
    max-width: 100%;
    max-height: 100%;
    padding: 20px;
    overflow-y: auto;
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow);
    animation: pop 150ms var(--ease);
  }

  .panel:focus {
    outline: none;
  }

  h2 {
    margin: 0 0 8px;
    font-size: 15px;
    font-weight: 600;
  }

  @keyframes fade {
    from {
      opacity: 0;
    }
  }

  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(6px) scale(0.98);
    }
  }
</style>
