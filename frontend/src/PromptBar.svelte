<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { ChooseFiles, Prompt } from '../wailsjs/go/app/App'
  import { OnFileDrop, OnFileDropOff } from '../wailsjs/runtime/runtime'
  import type { core } from '../wailsjs/go/models'
  import Icon from './Icon.svelte'
  import { addFiles, composeMessage, fileName, removeFile } from './lib/attachments'
  import { errorMessage } from './lib/errors'
  import { INITIAL_HEIGHT_STATE, heightStateOf, needsHeightMeasure } from './lib/composerHeight'
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
    dropBlocked = false,
    onSent,
    onLocate,
    onTyping,
  }: {
    card: core.Card | null
    dropBlocked?: boolean
    onSent: () => void
    onLocate: () => void
    onTyping: () => void
  } = $props()

  const MAX_HEIGHT = 172

  let dragging = $state(false)
  let prompt = $state<PromptState>({ ...emptyPrompt })
  let input: HTMLTextAreaElement
  let dragTimer: ReturnType<typeof setTimeout> | undefined
  let heightState = INITIAL_HEIGHT_STATE

  const availability = $derived(promptAvailability(card, prompt))
  const locked = $derived(availability.disabled && availability.reason !== '')
  const placeholder = $derived(card ? `Message pour ${card.Name}…` : 'Aucune session sélectionnée')

  // The height is measured at most once per frame, and only when the text may have changed it.
  $effect(() => {
    void prompt.text
    if (!input) return
    const frame = requestAnimationFrame(fitHeight)
    return () => cancelAnimationFrame(frame)
  })

  function fitHeight(): void {
    const text = prompt.text
    const overflows = (): boolean => input.scrollHeight > input.clientHeight && input.clientHeight < MAX_HEIGHT
    const measure = needsHeightMeasure(heightState, text, overflows)
    heightState = heightStateOf(text)
    if (!measure) return
    input.style.height = 'auto'
    input.style.height = `${Math.min(input.scrollHeight, MAX_HEIGHT)}px`
  }

  /** Puts the cursor in the text area. */
  export function focus(): void {
    input?.focus()
  }

  /** Adds files to the message, unless the bar cannot take a message. */
  function attach(paths: readonly string[]): void {
    if (availability.disabled || paths.length === 0) return
    prompt = { ...prompt, files: addFiles(prompt.files, paths), error: '' }
  }

  async function chooseFiles(): Promise<void> {
    if (availability.disabled) return
    try {
      attach((await ChooseFiles()) ?? [])
    } catch (e) {
      prompt = { ...prompt, error: errorMessage(e) }
    }
  }

  function detach(path: string): void {
    prompt = { ...prompt, files: removeFile(prompt.files, path) }
  }

  function carriesFiles(e: DragEvent): boolean {
    return e.dataTransfer?.types.includes('Files') ?? false
  }

  function stopDragging(): void {
    clearTimeout(dragTimer)
    dragging = false
  }

  function onDragOver(e: DragEvent): void {
    if (!carriesFiles(e) || availability.disabled || dropBlocked) return
    dragging = true
    clearTimeout(dragTimer)
    dragTimer = setTimeout(stopDragging, 400)
  }

  function onDragLeave(e: DragEvent): void {
    if (e.relatedTarget === null) stopDragging()
  }

  onMount(() => {
    // A drop while a dialog or the fullscreen terminal covers the bar would add
    // files the user cannot see.
    OnFileDrop((_x, _y, paths) => {
      if (!dropBlocked) attach(paths)
    }, false)
    window.addEventListener('dragover', onDragOver)
    window.addEventListener('dragleave', onDragLeave)
    window.addEventListener('drop', stopDragging)
    return () => {
      OnFileDropOff()
      window.removeEventListener('dragover', onDragOver)
      window.removeEventListener('dragleave', onDragLeave)
      window.removeEventListener('drop', stopDragging)
      clearTimeout(dragTimer)
    }
  })

  async function send(): Promise<void> {
    if (!card || !canSend(prompt) || availability.disabled) return
    const pid = card.PID
    prompt = beginSend(prompt)
    try {
      await Prompt(pid, composeMessage(prompt.text, prompt.files))
      prompt = sendSucceeded()
      onSent()
    } catch (e) {
      prompt = sendFailed(prompt, errorMessage(e))
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
  {#if prompt.files.length > 0}
    <ul class="files" aria-label="Fichiers joints">
      {#each prompt.files as path (path)}
        <li class="chip" title={path}>
          <Icon name="file" size={14} />
          <span class="file-name">{fileName(path)}</span>
          <button
            class="remove"
            aria-label="Retirer {fileName(path)}"
            title="Retirer"
            disabled={prompt.sending}
            onclick={() => detach(path)}
          >
            <Icon name="x" size={12} />
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  <div class="bar" class:dropping={dragging}>
    {#if card}
      <button class="target {frameClass(card.State, false)}" title="Afficher la carte de {card.Name}" onclick={onLocate}>
        <span class="dot"></span>
        <span class="name">{card.Name}</span>
      </button>
    {:else}
      <span class="target none">Aucune session</span>
    {/if}
    <button
      class="attach"
      aria-label="Joindre des fichiers"
      title="Joindre des fichiers"
      disabled={availability.disabled}
      onclick={() => void chooseFiles()}
    >
      <Icon name="plus" />
    </button>
    {#if locked}
      <span class="reason" role="status"><Icon name="lock" /><span>{availability.reason}</span></span>
    {/if}
    <textarea
      bind:this={input}
      bind:value={prompt.text}
      onkeydown={onKeydown}
      oninput={onTyping}
      disabled={availability.disabled}
      hidden={locked}
      rows="1"
      style:--max-height="{MAX_HEIGHT}px"
      aria-label="Message à envoyer"
      {placeholder}
    ></textarea>
    <button
      class="send"
      class:sending={prompt.sending}
      aria-label="Envoyer le message"
      title="Envoyer (Entrée)"
      disabled={availability.disabled || !canSend(prompt)}
      onclick={() => void send()}
    >
      {#if prompt.sending}<span class="spin"><Icon name="loader" /></span>{:else}<Icon name="send" />{/if}
    </button>
  </div>
  <div class="hint">
    {#if dragging}
      <span class="drop-hint">Déposer pour joindre les fichiers</span>
    {:else if prompt.error !== ''}
      <span class="error"><Icon name="error" size={14} />{prompt.error}</span>
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

  .bar.dropping {
    border-color: var(--accent);
    border-style: dashed;
    background: color-mix(in srgb, var(--accent) 8%, var(--surface-2));
  }

  .files {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin: 0 0 6px;
    padding: 0;
    list-style: none;
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    max-width: 260px;
    height: 26px;
    padding: 0 4px 0 8px;
    font-size: 12px;
    color: var(--text-2);
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: 999px;
  }

  .file-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .remove,
  .attach {
    flex: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    color: var(--text-2);
    background: transparent;
    border: none;
    transition:
      background 120ms var(--ease),
      color 120ms var(--ease);
  }

  .remove {
    width: 18px;
    height: 18px;
    border-radius: 50%;
  }

  .attach {
    width: 28px;
    height: 28px;
    border-radius: var(--radius-md);
  }

  .remove:hover:not(:disabled),
  .attach:hover:not(:disabled) {
    color: var(--text);
    background: var(--surface);
  }

  .attach:disabled,
  .remove:disabled {
    opacity: 0.5;
  }

  .drop-hint {
    color: var(--accent);
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
    max-height: var(--max-height);
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
