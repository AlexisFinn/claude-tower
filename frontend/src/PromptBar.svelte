<script lang="ts">
  import { tick } from 'svelte'
  import { Prompt } from '../wailsjs/go/app/App'
  import type { core } from '../wailsjs/go/models'
  import Icon from './Icon.svelte'
  import { errorMessage } from './lib/errors'
  import { frameClass } from './lib/frame'
  import {
    beginSend,
    canSend,
    emptyPrompt,
    promptAvailability,
    sendFailed,
    sendSucceeded,
    type PromptState,
  } from './lib/promptState'

  let {
    card,
    onSent,
    onLocate,
  }: { card: core.Card | null; onSent: () => void; onLocate: () => void } = $props()

  const MAX_HEIGHT = 172

  let state = $state<PromptState>({ ...emptyPrompt })
  let input: HTMLTextAreaElement

  const availability = $derived(promptAvailability(card, state))
  const locked = $derived(availability.disabled && availability.reason !== '')
  const placeholder = $derived(card ? `Message pour ${card.Name}…` : 'Aucune session sélectionnée')

  $effect(() => {
    void state.text
    if (!input) return
    input.style.height = 'auto'
    input.style.height = `${Math.min(input.scrollHeight, MAX_HEIGHT)}px`
  })

  /** Puts the cursor in the text area. */
  export function focus(): void {
    input?.focus()
  }

  async function send(): Promise<void> {
    if (!card || !canSend(state) || availability.disabled) return
    const pid = card.PID
    state = beginSend(state)
    try {
      await Prompt(pid, state.text)
      state = sendSucceeded()
      onSent()
    } catch (e) {
      state = sendFailed(state, errorMessage(e))
    }
    await tick()
    input?.focus()
  }

  function onKeydown(e: KeyboardEvent): void {
    if (e.key === 'Enter' && !e.shiftKey && !e.isComposing && e.keyCode !== 229) {
      e.preventDefault()
      void send()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      input.blur()
    }
  }
</script>

<div class="composer">
  <div class="bar">
    {#if card}
      <button class="target {frameClass(card.State, false)}" title="Afficher la carte de {card.Name}" onclick={onLocate}>
        <span class="dot"></span>
        <span class="name">{card.Name}</span>
      </button>
    {:else}
      <span class="target none">Aucune session</span>
    {/if}
    {#if locked}
      <span class="reason" role="status"><Icon name="lock" /><span>{availability.reason}</span></span>
    {/if}
    <textarea
      bind:this={input}
      bind:value={state.text}
      onkeydown={onKeydown}
      disabled={availability.disabled}
      hidden={locked}
      rows="1"
      aria-label="Message à envoyer"
      {placeholder}
    ></textarea>
    <button
      class="send"
      class:sending={state.sending}
      aria-label="Envoyer le message"
      title="Envoyer (Entrée)"
      disabled={availability.disabled || !canSend(state)}
      onclick={() => void send()}
    >
      {#if state.sending}<span class="spin"><Icon name="loader" /></span>{:else}<Icon name="send" />{/if}
    </button>
  </div>
  <div class="hint">
    {#if state.error !== ''}
      <span class="error"><Icon name="error" size={14} />{state.error}</span>
    {:else}
      <span><kbd>Entrée</kbd> envoyer</span>
      <span class="sep">·</span>
      <span><kbd>Maj</kbd>+<kbd>Entrée</kbd> nouvelle ligne</span>
      <span class="sep">·</span>
      <span><kbd>/</kbd> pour écrire</span>
      <span class="sep">·</span>
      <span><kbd>Échap</kbd> retour à la grille</span>
      <span class="sep">·</span>
      <span><kbd>?</kbd> raccourcis</span>
    {/if}
  </div>
</div>

<style>
  .composer {
    flex: none;
    padding: 0 12px 6px;
  }

  .bar {
    display: flex;
    align-items: flex-end;
    gap: 8px;
    padding: 8px;
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow);
    transition:
      border-color 150ms var(--ease),
      box-shadow 150ms var(--ease);
  }

  .bar:focus-within {
    border-color: var(--accent);
    box-shadow:
      var(--shadow),
      0 0 0 2px color-mix(in srgb, var(--accent) 40%, transparent);
  }

  .target {
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 7px;
    max-width: 200px;
    height: 28px;
    padding: 0 10px 0 9px;
    font-size: 12px;
    font-weight: 500;
    color: var(--text);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 999px;
    transition: border-color 120ms var(--ease);
  }

  button.target:hover {
    border-color: var(--border-strong);
  }

  .target.none {
    color: var(--text-3);
    font-weight: 400;
    cursor: default;
  }

  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .dot {
    flex: none;
    width: 8px;
    height: 8px;
    background: var(--state);
    border-radius: 50%;
  }

  .reason {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 7px;
    min-width: 0;
    min-height: 28px;
    color: var(--text-2);
  }

  textarea {
    flex: 1;
    min-width: 0;
    min-height: 28px;
    max-height: 172px;
    margin: 0;
    padding: 4px 2px;
    font: inherit;
    line-height: 20px;
    color: var(--text);
    resize: none;
    background: transparent;
    border: none;
  }

  textarea:focus,
  textarea:focus-visible {
    outline: none;
  }

  textarea::placeholder {
    color: var(--text-3);
  }

  textarea:disabled {
    opacity: 0.6;
  }

  .send {
    flex: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 28px;
    padding: 0;
    color: #1a0f0a;
    background: var(--accent);
    border: none;
    border-radius: var(--radius-md);
    transition:
      background 120ms var(--ease),
      opacity 120ms var(--ease);
  }

  .send:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .send:disabled:not(.sending) {
    color: var(--text-3);
    background: var(--surface);
  }

  .hint {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 6px;
    min-height: 20px;
    padding: 5px 6px 0;
    font-size: 11px;
    color: var(--text-3);
  }

  .hint .sep {
    opacity: 0.6;
  }

  .error {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    color: var(--danger);
  }
</style>
