import { describe, expect, it } from 'vitest';
import {
  SKIP_SECONDS,
  VOLUME_STEP,
  actionFor,
  keyboardDriven,
  skipTarget,
  stepVolume,
  targetFacts,
  watchInputModality,
  type KeyFacts,
  type TargetFacts,
} from './shortcuts';

const key = (k: string, extra: Partial<KeyFacts> = {}): KeyFacts => ({
  key: k,
  ctrlKey: false,
  metaKey: false,
  altKey: false,
  shiftKey: false,
  ...extra,
});
const target = (tag: string, extra: Partial<TargetFacts> = {}): TargetFacts => ({
  tag,
  type: '',
  role: '',
  editable: false,
  ...extra,
});
const body = target('body');

describe('actionFor', () => {
  it('maps Space and the four arrows', () => {
    expect(actionFor(key(' '), body, true)).toEqual({ kind: 'toggle' });
    expect(actionFor(key('ArrowLeft'), body, true)).toEqual({ kind: 'skip', seconds: -SKIP_SECONDS });
    expect(actionFor(key('ArrowRight'), body, true)).toEqual({ kind: 'skip', seconds: SKIP_SECONDS });
    expect(actionFor(key('ArrowUp'), body, true)).toEqual({ kind: 'volume', delta: VOLUME_STEP });
    expect(actionFor(key('ArrowDown'), body, true)).toEqual({ kind: 'volume', delta: -VOLUME_STEP });
  });

  it('ignores every other key', () => {
    for (const k of ['Enter', 'a', 'm', 'f', 'k', 'Tab', 'Escape', 'PageDown', 'Home']) {
      expect(actionFor(key(k), body, true)).toBeNull();
    }
  });

  it('leaves browser and system shortcuts alone', () => {
    for (const mod of ['ctrlKey', 'metaKey', 'altKey', 'shiftKey'] as const) {
      expect(actionFor(key('ArrowLeft', { [mod]: true }), body, true)).toBeNull();
      expect(actionFor(key(' ', { [mod]: true }), body, true)).toBeNull();
    }
    expect(actionFor(key(' ', { isComposing: true }), body, true)).toBeNull();
  });

  it('works from anywhere on the page: body, video, links, panels', () => {
    for (const t of [body, target(''), target('video'), target('audio'), target('a'), target('aside'), target('main'), target('div')]) {
      for (const k of [' ', 'ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown']) {
        expect(actionFor(key(k), t, true), `${t.tag} ${k}`).not.toBeNull();
        expect(actionFor(key(k), t, false), `${t.tag} ${k} (pointer)`).not.toBeNull();
      }
    }
  });

  it('keeps all five keys out of text fields and selects', () => {
    const fields = [
      target('input', { type: 'text' }),
      target('input'),
      target('input', { type: 'range' }),
      target('textarea'),
      target('select'),
      target('div', { editable: true }),
    ];
    for (const t of fields) {
      for (const k of [' ', 'ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown']) {
        expect(actionFor(key(k), t, true), `${t.tag}/${t.type} ${k}`).toBeNull();
        expect(actionFor(key(k), t, false)).toBeNull();
      }
    }
  });

  it('keeps the keys widgets use for themselves', () => {
    for (const role of ['slider', 'tab', 'menuitem', 'listbox', 'option', 'radio', 'combobox', 'textbox']) {
      expect(actionFor(key('ArrowDown'), target('div', { role }), true), role).toBeNull();
      expect(actionFor(key(' '), target('div', { role }), true), role).toBeNull();
    }
  });

  it('Space presses a button you tabbed to, but plays when you had clicked it', () => {
    for (const t of [target('button'), target('summary'), target('div', { role: 'button' }), target('span', { role: 'switch' })]) {
      expect(actionFor(key(' '), t, true), t.tag).toBeNull(); // keyboard: the button wins
      expect(actionFor(key(' '), t, false), t.tag).toEqual({ kind: 'toggle' }); // after a click: play/pause
      // arrows do nothing on a button, so they always control the player
      expect(actionFor(key('ArrowRight'), t, true), t.tag).toEqual({ kind: 'skip', seconds: SKIP_SECONDS });
    }
  });
});

describe('stepVolume', () => {
  it('moves in whole percents without drifting', () => {
    expect(stepVolume(0.85, VOLUME_STEP)).toBe(0.9);
    expect(stepVolume(0.7, VOLUME_STEP)).toBe(0.75);
    expect(stepVolume(0.3, -VOLUME_STEP)).toBe(0.25);
    let v = 0;
    for (let i = 0; i < 20; i++) v = stepVolume(v, VOLUME_STEP);
    expect(v).toBe(1);
    for (let i = 0; i < 20; i++) v = stepVolume(v, -VOLUME_STEP);
    expect(v).toBe(0);
  });

  it('stays between 0 and 1', () => {
    expect(stepVolume(1, VOLUME_STEP)).toBe(1);
    expect(stepVolume(0, -VOLUME_STEP)).toBe(0);
    expect(stepVolume(0.02, -VOLUME_STEP)).toBe(0);
    expect(stepVolume(0.98, VOLUME_STEP)).toBe(1);
  });

  it('snaps an in-between level (set with the slider) to the grid as it moves', () => {
    expect(stepVolume(0.374, VOLUME_STEP)).toBe(0.42);
  });
});

describe('skipTarget', () => {
  it('moves by the given seconds', () => {
    expect(skipTarget(30, 5, 600)).toBe(35);
    expect(skipTarget(30, -5, 600)).toBe(25);
  });
  it('never leaves the file', () => {
    expect(skipTarget(2, -5, 600)).toBe(0);
    expect(skipTarget(598, 5, 600)).toBe(600);
  });
  it('copes with an unknown or endless duration', () => {
    expect(skipTarget(10, 5, NaN)).toBe(15);
    expect(skipTarget(10, 5, Infinity)).toBe(15);
    expect(skipTarget(10, -50, NaN)).toBe(0);
    expect(skipTarget(10, 5, 0)).toBe(15);
  });
});

describe('targetFacts', () => {
  const make = (html: string): Element => {
    const host = document.createElement('div');
    host.innerHTML = html;
    document.body.append(host);
    return host.firstElementChild as Element;
  };

  it('reads tag, type, role and editability', () => {
    expect(targetFacts(make('<button>x</button>'))).toEqual({ tag: 'button', type: '', role: '', editable: false });
    expect(targetFacts(make('<input type="Text">'))).toMatchObject({ tag: 'input', type: 'text' });
    expect(targetFacts(make('<div role="SLIDER"></div>'))).toMatchObject({ tag: 'div', role: 'slider' });
    expect(targetFacts(make('<div contenteditable="true"></div>')).editable).toBe(true);
    expect(targetFacts(make('<div contenteditable></div>')).editable).toBe(true);
    expect(targetFacts(make('<div contenteditable="false"></div>')).editable).toBe(false);
  });

  it('treats a missing or non-element target as "nothing focused"', () => {
    expect(targetFacts(null).tag).toBe('');
    expect(targetFacts(document).tag).toBe('');
    expect(targetFacts(document.createTextNode('hi')).tag).toBe('');
  });
});

describe('keyboard or pointer?', () => {
  it('follows the last click or Tab press', () => {
    watchInputModality();
    watchInputModality(); // idempotent
    expect(keyboardDriven()).toBe(true);
    window.dispatchEvent(new Event('pointerdown'));
    expect(keyboardDriven()).toBe(false);
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }));
    expect(keyboardDriven()).toBe(false); // other keys do not switch it back
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }));
    expect(keyboardDriven()).toBe(true);
  });
});
