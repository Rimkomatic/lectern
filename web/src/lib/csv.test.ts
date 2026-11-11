import { describe, expect, it } from 'vitest';
import { detectDelimiter, parseCsv } from './csv';

describe('parseCsv', () => {
  it('parses simple rows', () => {
    expect(parseCsv('a,b,c\n1,2,3\n').rows).toEqual([
      ['a', 'b', 'c'],
      ['1', '2', '3'],
    ]);
  });
  it('handles quotes, escaped quotes and embedded newlines', () => {
    const { rows } = parseCsv('name,quote\n"Smith, J","He said ""hi"""\n"multi\nline",x\n');
    expect(rows).toEqual([
      ['name', 'quote'],
      ['Smith, J', 'He said "hi"'],
      ['multi\nline', 'x'],
    ]);
  });
  it('handles CRLF, BOM and a missing final newline', () => {
    expect(parseCsv('﻿a,b\r\n1,2').rows).toEqual([
      ['a', 'b'],
      ['1', '2'],
    ]);
  });
  it('keeps empty fields', () => {
    expect(parseCsv('a,,c\n,,\n').rows).toEqual([
      ['a', '', 'c'],
      ['', '', ''],
    ]);
  });
  it('detects tab and semicolon files', () => {
    expect(detectDelimiter('a\tb\tc\n')).toBe('\t');
    expect(detectDelimiter('a;b;c\n')).toBe(';');
    expect(detectDelimiter('single\n')).toBe(',');
    expect(parseCsv('a;b\n1;2\n').rows[1]).toEqual(['1', '2']);
  });
  it('stops at maxRows and says so', () => {
    const text = Array.from({ length: 10 }, (_, i) => `r${i},x`).join('\n') + '\n';
    const r = parseCsv(text, 3);
    expect(r.rows).toHaveLength(3);
    expect(r.truncated).toBe(true);
    expect(parseCsv('a,b\n', 3).truncated).toBe(false);
  });
  it('does not hang on an unterminated quote', () => {
    const r = parseCsv('a,"never closed\nb,c');
    expect(r.rows.length).toBeGreaterThan(0);
  });
});
