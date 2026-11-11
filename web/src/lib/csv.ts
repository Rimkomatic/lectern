/** Picks the delimiter that appears most in the first line. */
export function detectDelimiter(text: string): string {
  const firstLine = text.split(/\r?\n/, 1)[0] ?? '';
  let best = ',';
  let bestCount = 0;
  for (const d of [',', '\t', ';', '|']) {
    const count = firstLine.split(d).length - 1;
    if (count > bestCount) {
      best = d;
      bestCount = count;
    }
  }
  return best;
}

/**
 * Parses CSV/TSV text (RFC 4180 quoting: "" inside quotes, newlines inside
 * quotes). Stops after maxRows rows and reports whether it was cut short.
 */
export function parseCsv(
  text: string,
  maxRows = 1000,
  delimiter = detectDelimiter(text),
): { rows: string[][]; truncated: boolean } {
  if (text.charCodeAt(0) === 0xfeff) text = text.slice(1);
  const rows: string[][] = [];
  let row: string[] = [];
  let field = '';
  let inQuotes = false;
  let i = 0;

  const endRow = () => {
    row.push(field);
    field = '';
    rows.push(row);
    row = [];
  };

  while (i < text.length) {
    const c = text[i];
    if (inQuotes) {
      if (c === '"') {
        if (text[i + 1] === '"') {
          field += '"';
          i += 2;
          continue;
        }
        inQuotes = false;
      } else {
        field += c;
      }
      i++;
      continue;
    }
    if (c === '"' && field === '') {
      inQuotes = true;
    } else if (c === delimiter) {
      row.push(field);
      field = '';
    } else if (c === '\r' || c === '\n') {
      if (c === '\r' && text[i + 1] === '\n') i++;
      endRow();
      if (rows.length >= maxRows) {
        return { rows, truncated: i + 1 < text.length && text.slice(i + 1).trim() !== '' };
      }
    } else {
      field += c;
    }
    i++;
  }
  if (field !== '' || row.length > 0) endRow();
  return { rows, truncated: false };
}
