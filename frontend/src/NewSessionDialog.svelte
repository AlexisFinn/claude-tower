<script lang="ts">
  import { ChooseDirectory } from '../wailsjs/go/app/App'
  import Dialog from './Dialog.svelte'
  import { errorMessage } from './lib/errors'

  const MAX_NAME_LENGTH = 60

  interface Props {
    busy: boolean
    error: string
    onSubmit: (name: string, dir: string) => Promise<boolean>
    onEdit: () => void
    onClose: () => void
  }

  let { busy, error, onSubmit, onEdit, onClose }: Props = $props()

  let name = $state('')
  let dir = $state('~')
  let choosing = $state(false)
  let chooseError = $state('')
  let dirInput: HTMLInputElement | null = $state(null)

  const message = $derived(error || chooseError)

  function edited(): void {
    chooseError = ''
    onEdit()
  }

  // A button that becomes disabled loses the focus, so it goes back to the field.
  async function submit(e: SubmitEvent): Promise<void> {
    e.preventDefault()
    if (busy) return
    chooseError = ''
    if (await onSubmit(name, dir)) return
    dirInput?.focus()
    dirInput?.select()
  }

  async function browse(): Promise<void> {
    if (choosing) return
    choosing = true
    chooseError = ''
    try {
      const picked = await ChooseDirectory()
      if (picked) {
        dir = picked
        edited()
      }
    } catch (e) {
      chooseError = errorMessage(e)
    } finally {
      choosing = false
      dirInput?.focus()
    }
  }
</script>

<Dialog title="Nouvelle session" {onClose}>
  <form onsubmit={submit}>
    <label>
      <span>Nom (facultatif)</span>
      <input
        type="text"
        data-autofocus
        maxlength={MAX_NAME_LENGTH}
        autocomplete="off"
        spellcheck="false"
        placeholder="Laissez vide pour le nom par défaut"
        bind:value={name}
      />
    </label>
    <div class="field">
      <label for="new-session-dir">Dossier</label>
      <div class="row">
        <input
          id="new-session-dir"
          class="mono"
          type="text"
          autocomplete="off"
          spellcheck="false"
          placeholder="~ ou chemin absolu"
          aria-invalid={error !== ''}
          bind:this={dirInput}
          bind:value={dir}
          oninput={edited}
        />
        <button type="button" class="btn" disabled={choosing} onclick={() => void browse()}>Parcourir…</button>
      </div>
      {#if message}
        <p class="error" role="alert">{message}</p>
      {/if}
    </div>
    <div class="actions">
      <button type="button" class="btn" title="Échap" onclick={onClose}>Annuler</button>
      <button type="submit" class="btn btn-primary" title="Entrée" disabled={busy}>Créer</button>
    </div>
  </form>
</Dialog>

<style>
  label {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 20px;
    color: var(--text-2);
  }

  .field {
    margin-bottom: 20px;
  }

  .field label {
    margin-bottom: 6px;
  }

  .row {
    display: flex;
    gap: 8px;
  }

  input {
    height: 32px;
    padding: 0 10px;
    font: inherit;
    color: var(--text);
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
  }

  .row input {
    flex: 1;
    min-width: 0;
  }

  .row .btn {
    height: 32px;
  }

  input.mono {
    font-family: var(--mono);
    font-size: 12.5px;
  }

  input[aria-invalid='true'] {
    border-color: var(--danger);
  }

  input::placeholder {
    color: var(--text-3);
  }

  .error {
    margin: 8px 0 0;
    color: var(--danger);
    overflow-wrap: anywhere;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
