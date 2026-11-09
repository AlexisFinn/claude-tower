<script lang="ts">
  import { frameClass } from './lib/frame'
  import { stateLabel } from './lib/cardText'

  let { state, label }: { state: string; label: string } = $props()
</script>

<span class="pill {frameClass(state, false)}">
  <span class="dot"></span>
  <span class="text">{stateLabel(state, label)}</span>
</span>

<style>
  .pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    padding: 2px 9px 2px 7px;
    font-size: 11.5px;
    font-weight: 500;
    line-height: 1.5;
    color: var(--state);
    background: var(--state-soft);
    border-radius: 999px;
    white-space: nowrap;
  }

  .text {
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .dot {
    flex: none;
    width: 7px;
    height: 7px;
    background: var(--state);
    border-radius: 50%;
  }
</style>
