<script lang="ts">
  import { icons, type IconName } from '../lib/icons';

  let {
    name,
    size = 20,
    stroke = 2,
    class: cls = '',
  }: { name: IconName; size?: number; stroke?: number; class?: string } = $props();
</script>

<svg
  class={cls}
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width={stroke}
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
  focusable="false"
>
  <!-- static, trusted artwork from lib/icons.ts -->
  {@html icons[name]}
</svg>
