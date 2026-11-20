// The files behind "add to home screen": the manifest, its icons and the tags in
// index.html that point at them. Nothing here runs in the app; the test stops a
// renamed icon or a broken manifest from going unnoticed. It lives next to the
// other Node scripts (and is plain JavaScript) because it reads files from disk.
import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

const web = path.resolve(import.meta.dirname, '..'); // the web/ folder
const pub = (...p) => path.join(web, 'public', ...p);
const fromUrl = (src) => pub(src.replace(/^\//, ''));

/** Width and height from a PNG's header. */
function pngSize(file) {
  const b = readFileSync(file);
  expect(b.subarray(0, 8).toString('hex'), `${file} is not a PNG`).toBe('89504e470d0a1a0a');
  return { w: b.readUInt32BE(16), h: b.readUInt32BE(20) };
}

const manifest = JSON.parse(readFileSync(pub('manifest.webmanifest'), 'utf8'));
const indexHtml = readFileSync(path.join(web, 'index.html'), 'utf8');

describe('manifest', () => {
  it('says what the app is and how it opens', () => {
    expect(manifest.name).toBe('Lectern');
    expect(manifest.short_name.length).toBeLessThanOrEqual(12);
    expect(manifest.start_url).toBe('/');
    expect(manifest.scope).toBe('/');
    expect(manifest.display).toBe('standalone');
  });

  it('has icons that exist and are the size they claim', () => {
    expect(manifest.icons.length).toBeGreaterThanOrEqual(3);
    for (const icon of manifest.icons) {
      const file = fromUrl(icon.src);
      expect(existsSync(file), `${icon.src} is missing`).toBe(true);
      expect(icon.type).toBe('image/png');
      const [w, h] = icon.sizes.split('x').map(Number);
      expect(pngSize(file)).toEqual({ w, h });
    }
  });

  it('offers the sizes phones ask for, and a maskable one', () => {
    const any = manifest.icons.filter((i) => (i.purpose ?? 'any') === 'any').map((i) => i.sizes);
    expect(any).toContain('192x192');
    expect(any).toContain('512x512');
    expect(manifest.icons.some((i) => i.purpose === 'maskable' && i.sizes === '512x512')).toBe(true);
  });
});

describe('index.html', () => {
  it('links the manifest and the iOS icon', () => {
    expect(indexHtml).toContain('<link rel="manifest" href="/manifest.webmanifest" />');
    const m = indexHtml.match(/<link rel="apple-touch-icon" href="([^"]+)"/);
    expect(m, 'no apple-touch-icon link').not.toBeNull();
    const file = fromUrl(m[1]);
    expect(existsSync(file)).toBe(true);
    expect(pngSize(file)).toEqual({ w: 180, h: 180 });
  });

  it('keeps search engines out', () => {
    expect(indexHtml).toContain('<meta name="robots" content="noindex, nofollow" />');
    expect(readFileSync(pub('robots.txt'), 'utf8')).toMatch(/User-agent: \*\s+Disallow: \/\s*$/m);
  });
});
