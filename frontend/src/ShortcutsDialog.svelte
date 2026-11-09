<script lang="ts">
  import Dialog from './Dialog.svelte'
  import { SHORTCUT_GROUPS } from './lib/shortcuts'

  let { onClose }: { onClose: () => void } = $props()
</script>

<Dialog title="Raccourcis clavier" width={520} {onClose}>
  {#each SHORTCUT_GROUPS as group (group.title)}
    <section>
      <h3>{group.title}</h3>
      <dl>
        {#each group.items as item (item.label + item.combos.join())}
          <dt>{item.label}</dt>
          <dd>
            {#each item.combos as combo, i (i)}
              {#if i > 0}<span class="or">ou</span>{/if}
              <span class="combo">
                {#each combo as k, j (j)}
                  {#if j > 0}<span class="plus">+</span>{/if}<kbd>{k}</kbd>
                {/each}
              </span>
            {/each}
          </dd>
        {/each}
      </dl>
    </section>
  {/each}
  <div class="footer">
    <button class="btn" data-autofocus onclick={onClose}>Fermer</button>
  </div>
</Dialog>

<style>
  section {
    margin-top: 14px;
  }

  h3 {
    margin: 0 0 6px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-3);
  }

  dl {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 6px 16px;
    margin: 0;
  }

  dt {
    color: var(--text-2);
  }

  dd {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 4px;
    margin: 0;
  }

  .combo {
    display: inline-flex;
    align-items: center;
    gap: 3px;
  }

  .plus,
  .or {
    font-size: 11px;
    color: var(--text-3);
  }

  .footer {
    display: flex;
    justify-content: flex-end;
    margin-top: 18px;
  }
</style>
