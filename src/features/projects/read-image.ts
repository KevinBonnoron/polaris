const MAX_DIMENSION = 256;
const OUTPUT_MIME = 'image/png';

export async function readImageAsDataUrl(file: File): Promise<string> {
  if (!file.type.startsWith('image/') && !file.name.toLowerCase().endsWith('.svg')) {
    throw new Error('Please choose an image file.');
  }
  if (file.type === 'image/svg+xml' || file.name.toLowerCase().endsWith('.svg')) {
    return await readSvgAsDataUrl(file);
  }
  return await rasterize(file);
}

async function readSvgAsDataUrl(file: File): Promise<string> {
  const text = await file.text();
  const encoded = btoa(unescape(encodeURIComponent(text)));
  return `data:image/svg+xml;base64,${encoded}`;
}

async function rasterize(file: File): Promise<string> {
  const bitmap = await createImageBitmap(file);
  const { width, height } = bitmap;
  const scale = Math.min(1, MAX_DIMENSION / Math.max(width, height));
  const targetW = Math.max(1, Math.round(width * scale));
  const targetH = Math.max(1, Math.round(height * scale));

  const canvas = document.createElement('canvas');
  canvas.width = targetW;
  canvas.height = targetH;
  const ctx = canvas.getContext('2d');
  if (!ctx) {
    bitmap.close?.();
    throw new Error('Canvas not supported.');
  }
  ctx.drawImage(bitmap, 0, 0, targetW, targetH);
  bitmap.close?.();
  return canvas.toDataURL(OUTPUT_MIME);
}
