// Course covers are shrunk in the browser before they are uploaded: a 12 MB
// photo becomes a few hundred kilobytes, so the server's 5 MB limit is rarely
// met and the gallery stays quick to load.

export const COVER_TYPES = ['image/jpeg', 'image/png', 'image/webp', 'image/avif'] as const;
export const COVER_ACCEPT = COVER_TYPES.join(',');

/** What the server takes (maxCoverBytes in internal/api/covers.go). */
export const MAX_COVER_BYTES = 5 * 1024 * 1024;
/** Covers show at about 400 px wide, 800 on sharp screens; this leaves headroom. */
export const COVER_MAX_W = 1600;
export const COVER_MAX_H = 1000;
/** A picture no bigger than this, in bytes and pixels, is sent as it is. */
export const KEEP_BYTES = 1.5 * 1024 * 1024;

/** A problem with the chosen picture, worded for the person who chose it. */
export class CoverError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'CoverError';
  }
}

const BY_EXTENSION: Record<string, (typeof COVER_TYPES)[number]> = {
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  png: 'image/png',
  webp: 'image/webp',
  avif: 'image/avif',
};

/** The picture's type; some systems report none for .avif, so fall back to the extension. */
export function pictureType(name: string, type: string): string {
  if (type) return type;
  return BY_EXTENSION[name.split('.').pop()?.toLowerCase() ?? ''] ?? '';
}

/** The size to draw a w×h picture at so it fits the box, never enlarging it. */
export function fitWithin(w: number, h: number, maxW: number, maxH: number): { width: number; height: number } {
  const scale = Math.min(1, maxW / w, maxH / h);
  return { width: Math.max(1, Math.round(w * scale)), height: Math.max(1, Math.round(h * scale)) };
}

export function needsShrink(bytes: number, w: number, h: number): boolean {
  return bytes > KEEP_BYTES || w > COVER_MAX_W || h > COVER_MAX_H;
}

/**
 * Gets a chosen file ready to upload: small pictures go as they are, big ones
 * are scaled down and saved as JPEG (on white, since JPEG has no transparency).
 */
export async function prepareCover(file: File): Promise<Blob> {
  const type = pictureType(file.name, file.type);
  if (!(COVER_TYPES as readonly string[]).includes(type)) {
    throw new CoverError('Choose a JPEG, PNG, WebP or AVIF picture.');
  }
  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file);
  } catch {
    throw new CoverError("This browser can't read that picture. Try a JPEG or PNG.");
  }
  try {
    if (!needsShrink(file.size, bitmap.width, bitmap.height)) {
      return file.type ? file : file.slice(0, file.size, type);
    }
    const { width, height } = fitWithin(bitmap.width, bitmap.height, COVER_MAX_W, COVER_MAX_H);
    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new CoverError("This browser can't resize pictures.");
    ctx.fillStyle = '#fff';
    ctx.fillRect(0, 0, width, height);
    ctx.imageSmoothingQuality = 'high';
    ctx.drawImage(bitmap, 0, 0, width, height);
    const out = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/jpeg', 0.86));
    if (!out) throw new CoverError("That picture couldn't be prepared.");
    if (out.size > MAX_COVER_BYTES) throw new CoverError('That picture is too large.');
    return out;
  } finally {
    bitmap.close();
  }
}
