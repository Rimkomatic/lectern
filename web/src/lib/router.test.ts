import { describe, expect, it } from 'vitest';
import { links, parseHash } from './router.svelte';

describe('parseHash', () => {
  it('routes the basics', () => {
    expect(parseHash('')).toEqual({ name: 'home' });
    expect(parseHash('#/')).toEqual({ name: 'home' });
    expect(parseHash('#/settings')).toEqual({ name: 'settings' });
    expect(parseHash('#/admin')).toEqual({ name: 'admin' });
    expect(parseHash('#/course/12')).toEqual({ name: 'course', courseId: 12 });
  });
  it('reads item routes and resume parameters', () => {
    expect(parseHash('#/course/3/item/45')).toEqual({ name: 'item', courseId: 3, itemId: 45, t: undefined, p: undefined });
    expect(parseHash('#/course/3/item/45?t=91.5&p=7')).toEqual({ name: 'item', courseId: 3, itemId: 45, t: 91.5, p: 7 });
    expect(parseHash('#/course/3/item/45?t=-4&p=abc')).toMatchObject({ t: undefined, p: undefined });
  });
  it('rejects malformed routes', () => {
    for (const h of ['#/course', '#/course/abc', '#/course/0', '#/course/1/item', '#/course/1/item/x', '#/course/1/foo/2', '#/nope', '#/settings/x', '#/course/1/item/2/extra']) {
      expect(parseHash(h)).toEqual({ name: 'notfound' });
    }
  });
});

describe('links', () => {
  it('builds hrefs that parse back', () => {
    expect(links.item(3, 45)).toBe('#/course/3/item/45');
    expect(links.item(3, 45, { t: 91.7 })).toBe('#/course/3/item/45?t=91');
    expect(links.item(3, 45, { p: 1 })).toBe('#/course/3/item/45'); // page 1 is the default
    expect(parseHash(links.item(3, 45, { t: 10, p: 4 }))).toMatchObject({ name: 'item', courseId: 3, itemId: 45, t: 10, p: 4 });
    expect(parseHash(links.course(9))).toEqual({ name: 'course', courseId: 9 });
  });
  it('opens a chapter at its start, even when that is the very beginning', () => {
    expect(links.chapter(3, 45, 754.9)).toBe('#/course/3/item/45?t=754');
    expect(links.chapter(3, 45, 0)).toBe('#/course/3/item/45?t=0'); // item() would drop it, and the saved spot would win
    expect(links.chapter(3, 45, -5)).toBe('#/course/3/item/45?t=0');
    expect(parseHash(links.chapter(3, 45, 0))).toMatchObject({ name: 'item', courseId: 3, itemId: 45, t: 0 });
  });
});
