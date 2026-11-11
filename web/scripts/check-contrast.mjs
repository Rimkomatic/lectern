// Checks that the colours in src/styles/tokens.css are legible (WCAG 2.2 contrast).
//   npm run contrast
// Run it after changing a colour. It reads the real tokens, so there is nothing to keep in sync.
// Exits with 1 if any pair falls short.

import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const file = join(dirname(fileURLToPath(import.meta.url)), '..', 'src', 'styles', 'tokens.css');
const css = readFileSync(file, 'utf8');

/** Custom properties declared in the first block whose selector contains `selector`. */
function block(selector) {
  const start = css.indexOf(selector);
  if (start < 0) throw new Error(`tokens.css has no block for ${selector}`);
  const open = css.indexOf('{', start);
  const close = css.indexOf('}', open);
  const out = {};
  for (const m of css.slice(open + 1, close).matchAll(/--([\w-]+)\s*:\s*(#[0-9a-fA-F]{6})\s*;/g)) out[m[1]] = m[2];
  return out;
}

const themes = {
  light: block(":root[data-theme='light']"),
  dark: block(":root[data-theme='dark']"),
};

const channel = (v) => {
  const c = v / 255;
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
};
function luminance(hex) {
  const n = parseInt(hex.slice(1), 16);
  return 0.2126 * channel((n >> 16) & 255) + 0.7152 * channel((n >> 8) & 255) + 0.0722 * channel(n & 255);
}
function ratio(a, b) {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

let failures = 0;
function check(theme, label, fg, bg, need) {
  const r = ratio(fg, bg);
  const ok = r >= need;
  if (!ok) failures++;
  if (!ok || process.argv.includes('--all')) {
    console.log(`${ok ? 'ok  ' : 'FAIL'} ${r.toFixed(2).padStart(5)} (need ${need})  [${theme}] ${label}: ${fg} on ${bg}`);
  }
}

for (const [name, t] of Object.entries(themes)) {
  const need = ['bg', 'surface', 'surface-2', 'surface-3', 'accent', 'on-accent', 'accent-text', 'accent-soft', 'ok', 'ok-soft', 'danger', 'danger-soft', 'focus', 'border-input'];
  for (const k of need) if (!t[k]) throw new Error(`[${name}] --${k} is missing or not a #rrggbb colour`);

  // Text needs 4.5:1 on every surface it can appear on.
  for (const s of ['bg', 'surface', 'surface-2', 'surface-3']) {
    for (const fg of ['text', 'text-2', 'text-3', 'accent-text', 'ok', 'danger']) check(name, `--${fg} on --${s}`, t[fg], t[s], 4.5);
  }
  // Coloured chips and banners.
  for (const [fg, bg] of [['text', 'accent-soft'], ['accent-text', 'accent-soft'], ['text-2', 'accent-soft'], ['ok', 'ok-soft'], ['danger', 'danger-soft'], ['text', 'ok-soft'], ['text', 'danger-soft']]) {
    check(name, `--${fg} on --${bg}`, t[fg], t[bg], 4.5);
  }
  check(name, 'button label on the accent fill', t['on-accent'], t.accent, 4.5);
  // Non-text parts of the interface need 3:1 against what is behind them.
  for (const s of ['bg', 'surface']) {
    check(name, `focus ring on --${s}`, t.focus, t[s], 3);
    check(name, `field border on --${s}`, t['border-input'], t[s], 3);
  }
}

if (failures) {
  console.log(`\n${failures} pair(s) below the target. Adjust the colours in src/styles/tokens.css.`);
  process.exit(1);
}
console.log('Contrast: every checked pair meets WCAG AA (text 4.5:1, interface parts 3:1).');
