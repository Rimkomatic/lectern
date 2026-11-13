import DOMPurify from 'dompurify';
import { Marked } from 'marked';

// Course folders hold Markdown notes and HTML pages written by other people,
// so everything is cleaned before it is put on the page. The page this runs in
// has the signed-in session, so the rules are strict: no script of any kind, no
// styles (a page cannot restyle or cover the app), no forms, frames or embeds,
// no ids or classes (they could borrow the app's own), links only to other web
// sites and e-mail addresses, and no picture loaded from another site.
// Pictures stored in the course next to the page are the exception: the caller
// says where they are served from (see `resolveImage`).

export interface RenderOptions {
  /**
   * Turns a picture written as a relative path in the page ("images/a.png")
   * into a URL that serves it. Without it such pictures are left out.
   */
  resolveImage?: (ref: string) => string;
}

export interface Rendered {
  html: string;
  /** Pictures hosted on other web sites. They are not loaded. */
  blockedImages: number;
}

const FORBID_TAGS = [
  'style', 'script', 'noscript', 'template', 'link', 'meta', 'base', 'title',
  'form', 'input', 'button', 'select', 'option', 'optgroup', 'textarea', 'label', 'fieldset', 'legend', 'datalist', 'output',
  'iframe', 'frame', 'frameset', 'object', 'embed', 'applet', 'dialog', 'canvas',
  'audio', 'video', 'source', 'track', 'picture', 'map', 'area',
  'marquee', 'blink', // animated text from old pages
];

const FORBID_ATTR = [
  'style', 'class', 'id', 'name', 'srcset', 'sizes',
  // presentation that fights the app's light and dark themes
  'color', 'bgcolor', 'background', 'face', 'size', 'text', 'link', 'vlink', 'alink',
  'autofocus', 'tabindex', 'accesskey', 'contenteditable', 'draggable', 'spellcheck',
  'ping', 'action', 'formaction', 'form', 'poster', 'is', 'slot', 'part',
  'itemprop', 'itemscope', 'itemtype', 'itemid', 'itemref',
];

// Links go to other web sites or mail addresses. A relative link would land on a
// made-up address of this app, and a "#…" link would change the app's own page.
// DOMPurify tests every attribute value against this unless the attribute is listed
// as "not a URL", so the harmless ones that real pages use (table spans, language,
// list numbering…) are listed below or they would all be dropped.
const ALLOWED_URI_REGEXP = /^(?:https?:|mailto:)/i;
const NOT_URLS = [
  'lang', 'dir', 'translate', 'hreflang',
  'align', 'valign', 'border', 'cellpadding', 'cellspacing',
  'colspan', 'rowspan', 'span', 'scope', 'headers',
  'start', 'type', 'reversed', 'datetime', 'open',
];

const RASTER_DATA = /^data:image\/(?:png|jpe?g|gif|webp|avif|bmp)[;,]/i;
const OTHER_SITE = /^(?:https?:)?\/\//i;
const HAS_SCHEME = /^(?:[a-z][a-z0-9+.-]*:|\/\/)/i;

// State for the hooks of one call. Cleaning is synchronous, so one at a time.
let current: { resolveImage?: (ref: string) => string; blocked: number } = { blocked: 0 };
const imageSources = new WeakMap<Element, string>();
let hooked = false;

function hook(): void {
  if (hooked) return;
  hooked = true;
  // DOMPurify drops a relative src before anything else sees it: note it here.
  DOMPurify.addHook('uponSanitizeAttribute', (node, data) => {
    if (data.attrName === 'src' && node.nodeName === 'IMG') imageSources.set(node, data.attrValue);
  });
  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    if (node.nodeName === 'A') {
      // Links open in a new tab and never get the opener.
      if (node.getAttribute('href')) {
        node.setAttribute('target', '_blank');
        node.setAttribute('rel', 'noopener noreferrer');
      }
      return;
    }
    if (node.nodeName !== 'IMG') {
      node.removeAttribute('width'); // a fixed width from an old page would overflow a phone
      node.removeAttribute('height');
      return;
    }
    const raw = imageSources.get(node);
    imageSources.delete(node);
    node.removeAttribute('src');
    if (raw) {
      if (RASTER_DATA.test(raw)) {
        node.setAttribute('src', raw);
      } else if (OTHER_SITE.test(raw)) {
        current.blocked++; // the app's security policy would refuse it anyway
      } else if (!HAS_SCHEME.test(raw) && current.resolveImage) {
        node.setAttribute('src', current.resolveImage(raw));
      }
    }
    node.setAttribute('loading', 'lazy');
    node.setAttribute('decoding', 'async');
    node.setAttribute('referrerpolicy', 'no-referrer');
  });
}

function clean(html: string, opts: RenderOptions): Rendered {
  hook();
  current = { resolveImage: opts.resolveImage, blocked: 0 };
  try {
    const out = DOMPurify.sanitize(html, {
      USE_PROFILES: { html: true }, // no SVG, no MathML
      FORBID_TAGS,
      FORBID_ATTR,
      ALLOWED_URI_REGEXP,
      ADD_URI_SAFE_ATTR: NOT_URLS,
      ALLOW_DATA_ATTR: false,
    });
    return { html: out, blockedImages: current.blocked };
  } finally {
    current = { blocked: 0 };
  }
}

// Task lists ("- [x] done") become ☑ and ☐: the checkbox element itself is not allowed.
const markdown = new Marked({
  gfm: true,
  breaks: false,
  renderer: {
    checkbox({ checked }) {
      return checked ? '☑ ' : '☐ ';
    },
  },
});

/** Renders Markdown to HTML that is safe to put on the page. */
export function markdownToHtml(source: string, opts: RenderOptions = {}): Rendered {
  return clean(markdown.parse(source, { async: false }), opts);
}

/** Cleans a page of HTML (a whole document or a fragment) so it is safe to put on the page. */
export function cleanHtml(source: string, opts: RenderOptions = {}): Rendered {
  return clean(source, opts);
}

/** Markdown to safe HTML (the string only). */
export function renderMarkdown(source: string, opts: RenderOptions = {}): string {
  return markdownToHtml(source, opts).html;
}
