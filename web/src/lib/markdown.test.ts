import { describe, expect, it } from 'vitest';
import { renderMarkdown } from './markdown';

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
