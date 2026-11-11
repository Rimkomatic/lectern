<script lang="ts">
  import { coverIndex, initials } from '../lib/cover';
  import { url } from '../lib/api';

  let {
    course,
    eager = false,
  }: {
    course: { id: number; title: string; has_cover: boolean; cover_v?: number };
    eager?: boolean;
  } = $props();

  // A new upload has a new version and so a new address: no stale picture, and
  // an address that once failed does not hide the artwork for the next one.
  const src = $derived(url.cover(course.id, course.cover_v ?? 0));
  let brokenSrc = $state('');
  const showImage = $derived(course.has_cover && brokenSrc !== src);
</script>

<div class="cover" style:--bg-art="var(--cover-{coverIndex(course.title)})">
  {#if showImage}
    <img
      {src}
      alt=""
      loading={eager ? 'eager' : 'lazy'}
      decoding="async"
      onerror={() => (brokenSrc = src)}
    />
  {:else}
    <span class="art" aria-hidden="true">
      <span class="mono">{initials(course.title)}</span>
    </span>
  {/if}
</div>

<style>
  .cover {
    position: relative;
    width: 100%;
    height: 100%;
    overflow: hidden;
    background: var(--surface-3);
    container-type: inline-size;
  }
  img {
    display: block;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .art {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    background:
      radial-gradient(circle at 18% 115%, rgb(255 255 255 / 0.34), transparent 52%),
      radial-gradient(circle at 92% -10%, rgb(255 255 255 / 0.22), transparent 46%),
      var(--bg-art);
  }
  /* a fine dot grid for texture */
  .art::before {
    content: '';
    position: absolute;
    inset: 0;
    background: radial-gradient(rgb(255 255 255 / 0.22) 1px, transparent 1.6px) 0 0 / 16px 16px;
    mask-image: linear-gradient(135deg, transparent 20%, #000 100%);
    -webkit-mask-image: linear-gradient(135deg, transparent 20%, #000 100%);
  }
  .mono {
    position: relative;
    font-size: clamp(2rem, 24cqi, 4.75rem);
    font-weight: 800;
    letter-spacing: -0.04em;
    color: #fff;
    text-shadow: 0 2px 18px rgb(0 0 0 / 0.28);
    user-select: none;
  }
</style>
