// What a text lesson is: Markdown, a page of HTML, or plain text. HTML shows up
// as .html files and also as .txt files that were saved from a web page.

export type TextFormat = 'markdown' | 'html' | 'plain';

// Tags that mean "this is a web page" (not <T> in code or <name> in a sentence).
const TAG =
  /<\/?(?:html|body|head|title|meta|p|div|span|br|hr|h[1-6]|ul|ol|li|dl|dt|dd|a|b|i|u|em|strong|small|sup|sub|table|thead|tbody|tfoot|tr|td|th|img|pre|code|blockquote|section|article|header|footer|figure|figcaption|center|font)(?=[\s/>])[^<>]*>/gi;

/** Does this text read as HTML? Strict on purpose: a tutorial that mentions <p> and <br> in a sentence is not a page. */
export function looksLikeHtml(text: string): boolean {
  const head = text.slice(0, 20000).replace(/^﻿/, '');
  if (/^\s*(?:<!doctype\s+html|<html[\s>])/i.test(head)) return true;
  const startsWithTag = /^\s*</.test(head);
  let n = 0;
  for (const _ of head.matchAll(TAG)) {
    n++;
    if (startsWithTag && n >= 2) return true;
    if (n >= 8) return true;
  }
  return false;
}

/** How to show a text-like lesson. (CSV has its own table and never gets here.) */
export function formatOf(item: { kind: string; ext: string }, text: string): TextFormat {
  if (item.kind === 'markdown') return 'markdown';
  const ext = item.ext.toLowerCase();
  if (ext === 'html' || ext === 'htm') return 'html';
  // Only a .txt (or a file with no extension) is judged by what is in it: a .js
  // or .json file that contains HTML strings stays source code.
  if (ext === 'txt' || ext === '') return looksLikeHtml(text) ? 'html' : 'plain';
  return 'plain';
}

/**
 * A picture reference as a page writes it ("images/My%20Pic.png?raw=1") to the
 * file name it means ("images/My Pic.png"): without the query and fragment, and
 * with the percent-escapes undone.
 */
export function assetRef(ref: string): string {
  const cut = ref.search(/[?#]/);
  const bare = cut >= 0 ? ref.slice(0, cut) : ref;
  try {
    return decodeURIComponent(bare);
  } catch {
    return bare; // a stray "%" in the name
  }
}

/** Cleaned HTML with no words and no pictures left: the page was only scripts, styles or forms. */
export function isBlankHtml(html: string): boolean {
  return !/<img\b/i.test(html) && html.replace(/<[^>]*>/g, '').trim() === '';
}
