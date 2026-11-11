<script lang="ts">
  let { size = 32 }: { size?: number } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" focusable="false">
  <rect width="32" height="32" rx="10" fill="var(--accent)" />
  <path
    d="M12.2 8.6c0-1.3 1.4-2 2.5-1.4l9 5.5c.9.6.9 1.9 0 2.5l-9 5.5c-1.1.7-2.5-.1-2.5-1.4z"
    fill="var(--on-accent)"
  />
  <rect x="8.5" y="23.2" width="15" height="2.6" rx="1.3" fill="var(--on-accent)" opacity=".62" />
</svg>
