import { describe, expect, it } from 'vitest';
import { assetRef, formatOf, isBlankHtml, looksLikeHtml } from './textformat';

describe('looksLikeHtml', () => {
  it('recognises documents and fragments', () => {
    for (const s of [
      '<!DOCTYPE html>\n<html><body>x</body></html>',
      '  <html lang="en">',
      '﻿<!doctype HTML>',
      '<p>First paragraph.</p>\n<p>Second one.</p>',
      '<h2>Week 1</h2>\n<ul><li>Read</li><li>Watch</li></ul>',
      '<div class="lecture"><p>Hi <b>there</b></p></div>',
      '<br>\nA line, then <br/> another',
    ]) {
      expect(looksLikeHtml(s), s).toBe(true);
    }
  });
  it('recognises a page that starts with a line of text, once there are plenty of tags', () => {
    const s = 'Lecture 3 notes\n\n' + '<p>one</p>\n<p>two</p>\n<p>three</p>\n<p>four</p>\n'.repeat(1);
    expect(looksLikeHtml(s)).toBe(true);
  });
  it('leaves ordinary text, code and prose about tags alone', () => {
    for (const s of [
      '',
      'Just a plain note.\nLine two.',
      'if a < b && c > d { return }',
      'List<string> names = new List<string>();',
      'Use <p> for paragraphs and <br> for line breaks.',
      'Press <Enter> then <Tab>.',
      '# Heading\n\n* item\n* item',
      '<name>: the thing you type',
    ]) {
      expect(looksLikeHtml(s), s).toBe(false);
    }
  });
});

describe('formatOf', () => {
  const html = '<p>a</p><p>b</p>';
  it('Markdown is Markdown', () => {
    expect(formatOf({ kind: 'markdown', ext: 'md' }, 'x')).toBe('markdown');
    expect(formatOf({ kind: 'markdown', ext: 'md' }, html)).toBe('markdown');
  });
  it('.html and .htm are pages whatever is in them', () => {
    expect(formatOf({ kind: 'text', ext: 'html' }, 'plain words')).toBe('html');
    expect(formatOf({ kind: 'text', ext: 'HTM' }, '')).toBe('html');
  });
  it('a .txt is judged by its content', () => {
    expect(formatOf({ kind: 'text', ext: 'txt' }, html)).toBe('html');
    expect(formatOf({ kind: 'text', ext: 'txt' }, 'plain words')).toBe('plain');
    expect(formatOf({ kind: 'text', ext: '' }, html)).toBe('html');
  });
  it('source code stays source code', () => {
    for (const ext of ['js', 'ts', 'json', 'xml', 'py', 'go', 'log', 'yaml', 'srt', 'vtt']) {
      expect(formatOf({ kind: 'text', ext }, html), ext).toBe('plain');
    }
  });
});

describe('assetRef', () => {
  it('undoes percent-escapes, as the page meant the file name', () => {
    expect(assetRef('images/My%20Pic.png')).toBe('images/My Pic.png');
    expect(assetRef('caf%C3%A9.jpg')).toBe('café.jpg');
  });
  it('drops the query and the fragment before decoding', () => {
    expect(assetRef('a.png?raw=1')).toBe('a.png');
    expect(assetRef('a.png#top')).toBe('a.png');
    expect(assetRef('a%23b.png')).toBe('a#b.png'); // an escaped "#" is part of the name
  });
  it('keeps a name with a stray percent sign', () => {
    expect(assetRef('100%.png')).toBe('100%.png');
  });
  it('leaves paths alone', () => {
    expect(assetRef('../up/pic.gif')).toBe('../up/pic.gif');
    expect(assetRef('/shared/pic.gif')).toBe('/shared/pic.gif');
  });
});

describe('isBlankHtml', () => {
  it('is true when only empty tags remain', () => {
    expect(isBlankHtml('')).toBe(true);
    expect(isBlankHtml('<div> <p></p>\n</div>')).toBe(true);
  });
  it('is false for words or a picture', () => {
    expect(isBlankHtml('<p>hi</p>')).toBe(false);
    expect(isBlankHtml('<div><img alt="" src="/x.png"></div>')).toBe(false);
  });
});
