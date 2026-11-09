<script lang="ts">
  import { onMount } from 'svelte'
  import { StartupError, StartupWarning } from '../wailsjs/go/app/App'
  import { errorMessage } from './lib/errors'
  import ErrorPage from './ErrorPage.svelte'
  import Tower from './Tower.svelte'

  let status = $state<{ error: string; warning: string } | null>(null)

  onMount(async () => {
    try {
      const [error, warning] = await Promise.all([StartupError(), StartupWarning()])
      status = { error, warning }
    } catch (e) {
      status = { error: errorMessage(e), warning: '' }
    }
  })
</script>

{#if status === null}
  <p class="loading">Démarrage…</p>
{:else if status.error !== ''}
  <ErrorPage message={status.error} />
{:else}
  <Tower warning={status.warning} />
{/if}

<style>
  .loading {
    margin: 0;
    padding: 24px;
    color: var(--text-2);
  }
</style>
