<script lang="ts">
  import Dialog from './Dialog.svelte'

  const MAX_NAME_LENGTH = 60

  let { onSubmit, onClose }: { onSubmit: (name: string) => void; onClose: () => void } = $props()

  let name = $state('')

  function submit(e: SubmitEvent): void {
    e.preventDefault()
    onSubmit(name)
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
    <div class="actions">
      <button type="button" class="btn" title="Échap" onclick={onClose}>Annuler</button>
      <button type="submit" class="btn btn-primary" title="Entrée">Choisir le dossier…</button>
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

  input {
    height: 32px;
    padding: 0 10px;
    font: inherit;
    color: var(--text);
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
  }

  input::placeholder {
    color: var(--text-3);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
