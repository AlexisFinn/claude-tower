<script lang="ts">
  import { tick } from 'svelte'
  import { Prompt } from '../wailsjs/go/app/App'
  import type { core } from '../wailsjs/go/models'
  import { errorMessage } from './lib/errors'
  import {
    beginSend,
    canSend,
    emptyPrompt,
    promptAvailability,
    sendFailed,
    sendSucceeded,
    type PromptState,
  } from './lib/promptState'

  let { card, onSent }: { card: core.Card | null; onSent: () => void } = $props()

  let state = $state<PromptState>({ ...emptyPrompt })
  let input: HTMLTextAreaElement

  const availability = $derived(promptAvailability(card, state))

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

<div class="bar">
  <div class="target">
    {#if card}→ {card.Name}{/if}
    {#if availability.reason !== ''}<span class="reason">{availability.reason}</span>{/if}
    {#if state.error !== ''}<span class="error">{state.error}</span>{/if}
  </div>
  <textarea
    bind:this={input}
    bind:value={state.text}
    onkeydown={onKeydown}
    disabled={availability.disabled}
    rows="2"
    placeholder="Message à envoyer (Entrée : envoyer, Maj+Entrée : nouvelle ligne, Échap : retour à la grille)"
  ></textarea>
</div>

<style>
  .bar {
    flex: none;
    padding: 8px 12px;
    background: var(--panel);
    border-top: 1px solid var(--border);
  }

  .target {
    min-height: 1.4em;
    color: var(--muted);
  }

  .reason {
    margin-left: 12px;
    color: var(--warn);
  }

  .error {
    margin-left: 12px;
    color: var(--danger);
  }

  textarea {
    box-sizing: border-box;
    width: 100%;
    resize: none;
    padding: 6px 8px;
    font: inherit;
    color: var(--text);
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 4px;
  }

  textarea:focus {
    outline: none;
    border-color: var(--info);
  }

  textarea:disabled {
    opacity: 0.6;
  }
</style>
