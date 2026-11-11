<script lang="ts">
  import Icon from './Icon.svelte';

  let {
    label,
    name,
    value = $bindable(''),
    autocomplete,
    required = false,
    minlength,
  }: {
    label: string;
    name: string;
    value?: string;
    autocomplete: 'current-password' | 'new-password';
    required?: boolean;
    minlength?: number;
  } = $props();

  const id = $props.id();
  let shown = $state(false);
</script>

<div class="field">
  <label for={id}><span>{label}</span></label>
  <div class="wrap">
    <input
      {id}
      class="input"
      type={shown ? 'text' : 'password'}
      {name}
      {autocomplete}
      {required}
      {minlength}
      bind:value
    />
    <button
      type="button"
      class="btn btn-ghost btn-icon btn-sm eye"
      aria-pressed={shown}
      title={shown ? 'Hide password' : 'Show password'}
      onclick={() => (shown = !shown)}
    >
      <Icon name={shown ? 'eye-off' : 'eye'} size={18} />
      <span class="sr-only">Show password</span>
    </button>
  </div>
</div>

<style>
  label span {
    font-size: 0.8125rem;
    font-weight: 600;
    color: var(--text-2);
  }
  .wrap {
    position: relative;
  }
  .wrap .input {
    padding-right: 3rem;
  }
  .eye {
    position: absolute;
    top: 50%;
    right: 6px;
    margin-top: -18px;
  }
</style>
