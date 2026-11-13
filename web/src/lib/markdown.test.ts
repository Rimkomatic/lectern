import { describe, expect, it } from 'vitest';
import { cleanHtml, markdownToHtml, renderMarkdown } from './markdown';

describe('renderMarkdown', () => {
  it('renders basic markdown', () => {
    const html = renderMarkdown('# Title\n\nSome **bold** text and a [link](https://example.com).');
    expect(html).toContain('<h1');
    expect(html).toContain('<strong>bold</strong>');
    expect(html).toContain('href="https://example.com"');
  });
  it('opens links safely in a new tab', () => {
    const html = renderMarkdown('[x](https://example.com)');
    expect(html).toContain('target="_blank"');
    expect(html).toContain('rel="noopener noreferrer"');
  });
  it('removes scripts, handlers and javascript: links', () => {
    const html = renderMarkdown(
      '<script>alert(1)</script>\n\n<img src=x onerror="alert(2)">\n\n[click](javascript:alert(3))\n\n<iframe src="https://evil.example"></iframe>',
    );
    expect(html).not.toMatch(/<script/i);
    expect(html).not.toMatch(/onerror/i);
    expect(html).not.toMatch(/javascript:/i);
    expect(html).not.toMatch(/<iframe/i);
  });
  it('renders tables (GFM)', () => {
    expect(renderMarkdown('| a | b |\n|---|---|\n| 1 | 2 |')).toContain('<table>');
  });
});

// ---- HTML pages, and the stricter rules both formats share ----


const resolve = (ref: string) => `/api/v1/items/7/asset?path=${encodeURIComponent(ref)}`;

/** Parses the cleaned HTML back, to look at what is left. */
function dom(html: string): HTMLElement {
  const d = document.createElement('div');
  d.innerHTML = html;
  return d;
}

describe('cleanHtml: pages', () => {
  it('keeps the structure of a page and drops its wrapper', () => {
    const { html } = cleanHtml(
      '<!DOCTYPE html><html><head><title>T</title><meta charset="utf-8"><style>p{color:red}</style></head>' +
        '<body><h1>Heading</h1><p>Some <b>bold</b>, <i>italic</i> and <a href="https://example.com/x">a link</a>.</p>' +
        '<ul><li>one</li><li>two</li></ul><table><tr><td>cell</td></tr></table><pre>code  here</pre></body></html>',
    );
    const d = dom(html);
    expect(d.querySelector('h1')?.textContent).toBe('Heading');
    expect(d.querySelector('b')?.textContent).toBe('bold');
    expect(d.querySelectorAll('li')).toHaveLength(2);
    expect(d.querySelector('td')?.textContent).toBe('cell');
    expect(d.querySelector('pre')?.textContent).toBe('code  here');
    expect(html).not.toMatch(/<style|<title|<meta|<html|<head|color:red/i);
  });

  it('handles a fragment, as saved from the middle of a web page', () => {
    const { html } = cleanHtml('<p>one</p>\n<p>two <br>three</p>');
    expect(dom(html).querySelectorAll('p')).toHaveLength(2);
  });

  it('never runs script: tags, handlers, javascript: and data: links, odd encodings', () => {
    const evil = [
      '<script>window.__x=1</script>',
      '<img src=x onerror="window.__x=2">',
      '<a href="javascript:window.__x=3">js</a>',
      '<a href="  JaVaScRiPt:window.__x=4">js</a>',
      '<a href="data:text/html,<script>window.__x=5</script>">data</a>',
      '<a href="vbscript:x">vb</a>',
      '<body onload="window.__x=6">',
      '<div onclick="window.__x=7">click</div>',
      '<svg onload="window.__x=8"><script>window.__x=9</script></svg>',
      '<math><mtext><table><mglyph><style><img src=x onerror="window.__x=10">',
      '<svg><p><style><img src=x onerror="window.__x=11">',
      '<noscript><p title="</noscript><img src=x onerror=window.__x=12>">',
      '<iframe src="https://example.com"></iframe><iframe srcdoc="<script>1</script>"></iframe>',
      '<object data="x"></object><embed src="x"><applet code="x"></applet>',
      '<meta http-equiv="refresh" content="0;url=https://evil.example">',
      '<base href="https://evil.example/">',
      '<link rel="stylesheet" href="https://evil.example/x.css">',
      '<form action="https://evil.example"><input name=pw type=password><button>Log in</button></form>',
      '<a href="https://example.com" onmouseover="window.__x=13">hover</a>',
    ].join('\n');
    const { html } = cleanHtml(evil);
    expect(html).not.toMatch(/<script|<iframe|<object|<embed|<applet|<meta|<base|<link|<form|<input|<button|<svg|<math|<style|<noscript/i);
    expect(html).not.toMatch(/\bon[a-z]+\s*=|javascript:|vbscript:|data:text|srcdoc|window\.__x=\d/i);
    // and what is left really is inert when parsed
    const d = dom(html);
    expect(d.querySelectorAll('[onerror],[onload],[onclick],[onmouseover]')).toHaveLength(0);
    expect((window as unknown as { __x?: number }).__x).toBeUndefined();
  });

  it('drops styles, classes, ids and presentation that could restyle or spoof the app', () => {
    const { html } = cleanHtml(
      '<p style="position:fixed;inset:0;background:#fff" class="btn btn-primary drawer" id="course-outline" name="x">hi</p>' +
        '<font color="#000000" face="Arial" size="5">black</font>' +
        '<table width="900" bgcolor="#ffffff" background="x.png"><tr><td width="500" height="9">c</td></tr></table>' +
        '<div aria-label="ok" title="kept" lang="de" dir="rtl">x</div>',
    );
    expect(html).not.toMatch(/style=|class=|id=|name=|color=|bgcolor=|face=|size=|background=|width=|height=/i);
    const d = dom(html);
    expect(d.querySelector('div')?.getAttribute('title')).toBe('kept');
    expect(d.querySelector('div')?.getAttribute('lang')).toBe('de');
  });

  it('keeps what a table or list needs to read correctly', () => {
    const { html } = cleanHtml(
      '<table border="1" cellpadding="4"><tr><th scope="col" colspan="2" align="center">h</th></tr>' +
        '<tr><td rowspan="2" valign="top">a</td><td>b</td></tr></table>' +
        '<ol type="a" start="3" reversed><li>x</li></ol><time datetime="2026-11-12">today</time><p lang="fr" dir="ltr">bonjour</p>',
    );
    const d = dom(html);
    expect(d.querySelector('th')?.getAttribute('colspan')).toBe('2');
    expect(d.querySelector('td')?.getAttribute('rowspan')).toBe('2');
    expect(d.querySelector('th')?.getAttribute('align')).toBe('center');
    expect(d.querySelector('table')?.getAttribute('border')).toBe('1');
    expect(d.querySelector('ol')?.getAttribute('start')).toBe('3');
    expect(d.querySelector('ol')?.getAttribute('type')).toBe('a');
    expect(d.querySelector('time')?.getAttribute('datetime')).toBe('2026-11-12');
    expect(d.querySelector('p')?.getAttribute('lang')).toBe('fr');
  });

  it('keeps links to web sites and mail, opened safely; drops the rest', () => {
    const { html } = cleanHtml(
      '<a href="https://example.com/a?b=1#c">web</a> <a href="http://example.com">plain</a> <a href="mailto:me@example.com">mail</a> ' +
        '<a href="lesson2.html">relative</a> <a href="/other">root</a> <a href="#section">fragment</a> <a href="//evil.example/x">proto</a> ' +
        '<a href="tel:123">tel</a> <a href="ftp://example.com/f">ftp</a> <a name="anchor">anchor</a>',
    );
    const hrefs = [...dom(html).querySelectorAll('a')].map((a) => a.getAttribute('href'));
    expect(hrefs.filter(Boolean)).toEqual(['https://example.com/a?b=1#c', 'http://example.com', 'mailto:me@example.com']);
    for (const a of dom(html).querySelectorAll('a[href]')) {
      expect(a.getAttribute('target')).toBe('_blank');
      expect(a.getAttribute('rel')).toBe('noopener noreferrer');
    }
    // the words of a link that was dropped are still there
    expect(dom(html).textContent).toContain('relative');
    expect(dom(html).textContent).toContain('fragment');
  });
});

describe('pictures', () => {
  it('serves relative pictures through the resolver, decoded as the page meant them', () => {
    const { html, blockedImages } = cleanHtml('<img src="images/My%20Pic.png" alt="diagram"> <img src="../up.jpg" alt=""> <img src="/root.gif">', {
      resolveImage: resolve,
    });
    const srcs = [...dom(html).querySelectorAll('img')].map((i) => i.getAttribute('src'));
    expect(srcs).toEqual([resolve('images/My%20Pic.png'), resolve('../up.jpg'), resolve('/root.gif')]);
    expect(blockedImages).toBe(0);
    for (const img of dom(html).querySelectorAll('img')) {
      expect(img.getAttribute('loading')).toBe('lazy');
      expect(img.getAttribute('referrerpolicy')).toBe('no-referrer');
    }
  });

  it('leaves relative pictures out when nothing can serve them', () => {
    const { html } = cleanHtml('<img src="images/a.png" alt="alt text">');
    expect(dom(html).querySelector('img')?.hasAttribute('src')).toBe(false);
    expect(dom(html).querySelector('img')?.getAttribute('alt')).toBe('alt text');
  });

  it('does not load pictures from other sites, and counts them', () => {
    const { html, blockedImages } = cleanHtml(
      '<img src="https://cdn.example.com/a.png" alt="a"><img src="http://example.com/b.png"><img src="//cdn.example.com/c.png"><img src="images/own.png">',
      { resolveImage: resolve },
    );
    const srcs = [...dom(html).querySelectorAll('img')].map((i) => i.getAttribute('src'));
    expect(srcs).toEqual([null, null, null, resolve('images/own.png')]);
    expect(blockedImages).toBe(3);
    expect(html).not.toContain('cdn.example.com');
  });

  it('keeps small embedded pictures and drops embedded anything else', () => {
    const { html } = cleanHtml(
      '<img src="data:image/png;base64,iVBORw0KGgo=">' +
        '<img src="data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+">' +
        '<img src="data:text/html,<script>1</script>">' +
        '<img src="javascript:1"><img src="file:///etc/passwd"><img src="blob:https://x/y">',
      { resolveImage: resolve },
    );
    const srcs = [...dom(html).querySelectorAll('img')].map((i) => i.getAttribute('src'));
    expect(srcs).toEqual(['data:image/png;base64,iVBORw0KGgo=', null, null, null, null, null]);
  });

  it('applies to Markdown pictures as well', () => {
    const r = markdownToHtml('![plan](diagrams/plan.png) ![out](https://example.com/x.png)', { resolveImage: resolve });
    const srcs = [...dom(r.html).querySelectorAll('img')].map((i) => i.getAttribute('src'));
    expect(srcs).toEqual([resolve('diagrams/plan.png'), null]);
    expect(r.blockedImages).toBe(1);
  });

  it('does not leak one call into the next', () => {
    cleanHtml('<img src="https://example.com/x.png">');
    expect(cleanHtml('<p>no pictures</p>').blockedImages).toBe(0);
    const a = cleanHtml('<img src="a.png">', { resolveImage: resolve });
    const b = cleanHtml('<img src="a.png">');
    expect(dom(a.html).querySelector('img')?.hasAttribute('src')).toBe(true);
    expect(dom(b.html).querySelector('img')?.hasAttribute('src')).toBe(false);
  });
});

describe('Markdown with the stricter rules', () => {
  it('turns task lists into ticks, since checkboxes are not allowed', () => {
    const { html } = markdownToHtml('- [x] done\n- [ ] to do');
    expect(html).not.toMatch(/<input/i);
    expect(dom(html).textContent).toContain('☑ done');
    expect(dom(html).textContent).toContain('☐ to do');
  });
  it('drops the style and class of embedded HTML', () => {
    const { html } = markdownToHtml('<div style="position:fixed" class="drawer">x</div>\n\n# T');
    expect(html).not.toMatch(/style=|class=/i);
  });
  it('keeps code, quotes and tables', () => {
    const { html } = markdownToHtml('> quote\n\n```go\nfmt.Println("hi")\n```\n\n| a | b |\n|---|---|\n| 1 | 2 |');
    const d = dom(html);
    expect(d.querySelector('blockquote')).not.toBeNull();
    expect(d.querySelector('pre code')?.textContent).toContain('fmt.Println("hi")');
    expect(d.querySelector('table')).not.toBeNull();
  });
});
