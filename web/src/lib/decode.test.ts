import { describe, expect, it } from 'vitest';
import { declaredCharset, decodeBytes } from './decode';

const bytes = (...n: number[]) => new Uint8Array(n);
const ascii = (s: string) => Uint8Array.from(s, (c) => c.charCodeAt(0));

function utf16(s: string, bigEndian: boolean, bom: boolean): Uint8Array {
  const out: number[] = bom ? (bigEndian ? [0xfe, 0xff] : [0xff, 0xfe]) : [];
  for (let i = 0; i < s.length; i++) {
    const u = s.charCodeAt(i);
    out.push(...(bigEndian ? [u >> 8, u & 255] : [u & 255, u >> 8]));
  }
  return Uint8Array.from(out);
}

describe('decodeBytes', () => {
  it('reads UTF-8, with or without a byte-order mark', () => {
    const enc = new TextEncoder();
    expect(decodeBytes(enc.encode('Café – 你好 😀'))).toBe('Café – 你好 😀');
    expect(decodeBytes(Uint8Array.from([0xef, 0xbb, 0xbf, ...enc.encode('hi')]))).toBe('hi');
    expect(decodeBytes(new Uint8Array(0))).toBe('');
  });

  it('reads UTF-16 with a mark, either way round', () => {
    expect(decodeBytes(utf16('Café 你好', false, true))).toBe('Café 你好');
    expect(decodeBytes(utf16('Café 你好', true, true))).toBe('Café 你好');
  });

  it('reads UTF-16 without a mark when the text is mostly ASCII', () => {
    const page = '<p>Hello, this is a page. 一二三</p>';
    expect(decodeBytes(utf16(page, false, false))).toBe(page);
    expect(decodeBytes(utf16(page, true, false))).toBe(page);
  });

  it('falls back to Windows-1252 for old files', () => {
    // "Café – “quoted”" as Windows-1252
    expect(decodeBytes(bytes(0x43, 0x61, 0x66, 0xe9, 0x20, 0x96, 0x20, 0x93, 0x71, 0x94))).toBe('Café – “q”');
  });

  it('honours a declared charset', () => {
    // 0x82 0xa0 is あ in Shift_JIS, and not valid UTF-8
    const page = Uint8Array.from([...ascii('<meta charset="Shift_JIS"><p>'), 0x82, 0xa0]);
    expect(decodeBytes(page)).toBe('<meta charset="Shift_JIS"><p>あ');
    const latin = Uint8Array.from([...ascii('<meta http-equiv="Content-Type" content="text/html; charset=iso-8859-2">'), 0xb1]);
    expect(decodeBytes(latin).endsWith('ą')).toBe(true);
  });

  it('does not trust a UTF-8 claim the bytes contradict, or an unknown label', () => {
    const lie = Uint8Array.from([...ascii('<meta charset="utf-8">caf'), 0xe9]);
    expect(decodeBytes(lie)).toBe('<meta charset="utf-8">café');
    const odd = Uint8Array.from([...ascii('<meta charset="no-such-charset">caf'), 0xe9]);
    expect(decodeBytes(odd)).toBe('<meta charset="no-such-charset">café');
  });

  it('accepts an ArrayBuffer', () => {
    expect(decodeBytes(new TextEncoder().encode('ok').buffer)).toBe('ok');
  });
});

describe('declaredCharset', () => {
  it('finds the declaration in its usual forms', () => {
    expect(declaredCharset(ascii('<meta charset=UTF-8>'))).toBe('utf-8');
    expect(declaredCharset(ascii("<META HTTP-EQUIV='Content-Type' CONTENT='text/html; charset=Windows-1251'>"))).toBe('windows-1251');
    expect(declaredCharset(ascii('<?xml version="1.0" encoding="ISO-8859-1"?><p>'))).toBe('iso-8859-1');
  });
  it('finds none in plain text', () => {
    expect(declaredCharset(ascii('just words, charset is not here'))).toBeNull();
  });
});
