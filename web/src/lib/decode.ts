// Reads the bytes of a text file the way the person who made it meant them.
// Most files are UTF-8 (with or without a byte-order mark), but course folders
// also hold old HTML pages and notes in Windows-1252, Latin-1, UTF-16 or an
// East-Asian encoding. Reading those as UTF-8 fills the page with "�".

/** UTF-16 text without a byte-order mark: a zero byte beside every ASCII letter, always on the same side. */
function sniffUtf16(b: Uint8Array): 'utf-16le' | 'utf-16be' | null {
  const n = Math.min(b.length, 512) & ~1;
  const pairs = n / 2;
  if (pairs < 4) return null;
  let even = 0;
  let odd = 0;
  for (let i = 0; i < n; i++) {
    if (b[i] === 0) {
      if (i % 2 === 0) even++;
      else odd++;
    }
  }
  if (odd * 4 >= pairs && odd >= 4 * even) return 'utf-16le';
  if (even * 4 >= pairs && even >= 4 * odd) return 'utf-16be';
  return null;
}

/** The charset a page declares: <meta charset=…>, <meta http-equiv … content="…; charset=…"> or an XML declaration. */
export function declaredCharset(b: Uint8Array): string | null {
  const head = new TextDecoder('latin1').decode(b.subarray(0, 2048));
  const m =
    /<meta[^>]+charset\s*=\s*["']?\s*([a-z0-9_.:-]+)/i.exec(head) ??
    /^\s*<\?xml[^>]+encoding\s*=\s*["']\s*([a-z0-9_.:-]+)/i.exec(head);
  return m ? m[1].toLowerCase() : null;
}

function decodeWith(label: string, b: Uint8Array): string | null {
  try {
    return new TextDecoder(label).decode(b);
  } catch {
    return null; // a label this browser does not know
  }
}

export function decodeBytes(input: ArrayBuffer | Uint8Array): string {
  const b = input instanceof Uint8Array ? input : new Uint8Array(input);
  if (b.length >= 2 && b[0] === 0xff && b[1] === 0xfe) return new TextDecoder('utf-16le').decode(b.subarray(2));
  if (b.length >= 2 && b[0] === 0xfe && b[1] === 0xff) return new TextDecoder('utf-16be').decode(b.subarray(2));
  const wide = sniffUtf16(b);
  if (wide) return new TextDecoder(wide).decode(b);

  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(b); // also drops a UTF-8 byte-order mark
  } catch {
    // not UTF-8
  }
  const label = declaredCharset(b);
  if (label && !/^utf-?8$/.test(label)) {
    const text = decodeWith(label, b);
    if (text !== null) return text;
  }
  // Without a word from the file, the common case is Windows-1252, which is also
  // what browsers use for an undeclared page. (A page that claims UTF-8 but is
  // not valid UTF-8 ends up here too.)
  return new TextDecoder('windows-1252').decode(b);
}
