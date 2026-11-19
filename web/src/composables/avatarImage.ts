// Canvas work for avatar / icon uploads: load, crop, scale to 256×256, encode
// as webp (or png) under 1 MB. The math is in lib/image.ts.
import { AVATAR_SIZE, MAX_IMAGE_BYTES, cropRect, nextQuality, outputSize, outputTypes, type Crop } from '@/lib/image'

/**
 * Load a picked file as an image element. A data: URL is used because the
 * server's CSP allows `img-src 'self' data:` (no blob:).
 */
export function loadImage(file: Blob): Promise<{ img: HTMLImageElement; release: () => void }> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(new Error('image'))
    reader.onload = () => {
      const img = new Image()
      img.onload = () => resolve({ img, release: () => undefined })
      img.onerror = () => reject(new Error('image'))
      img.src = String(reader.result)
    }
    reader.readAsDataURL(file)
  })
}

/** Draw the crop of `img` into `canvas` (preview and output). */
export function drawCrop(canvas: HTMLCanvasElement, img: HTMLImageElement, crop: Crop, size = AVATAR_SIZE): void {
  const r = cropRect(img.naturalWidth, img.naturalHeight, crop)
  const out = outputSize(r, size)
  canvas.width = out
  canvas.height = out
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'
  ctx.clearRect(0, 0, out, out)
  ctx.drawImage(img, r.sx, r.sy, r.sw, r.sh, 0, 0, out, out)
}

function toBlob(canvas: HTMLCanvasElement, type: string, quality?: number): Promise<Blob | null> {
  return new Promise((resolve) => canvas.toBlob((b) => resolve(b), type, quality))
}

let webp: boolean | null = null
function supportsWebp(): boolean {
  if (webp !== null) return webp
  try {
    const c = document.createElement('canvas')
    c.width = c.height = 1
    webp = c.toDataURL('image/webp').startsWith('data:image/webp')
  } catch {
    webp = false
  }
  return webp
}

/** Encode the cropped image; resolves null when it cannot get under the limit. */
export async function encodeAvatar(img: HTMLImageElement, crop: Crop, maxBytes = MAX_IMAGE_BYTES): Promise<Blob | null> {
  const canvas = document.createElement('canvas')
  drawCrop(canvas, img, crop)
  for (const type of outputTypes(supportsWebp())) {
    let q: number | null = 0.92
    while (q !== null) {
      const blob = await toBlob(canvas, type, q)
      if (blob && blob.type === type && blob.size <= maxBytes) return blob
      if (!blob || blob.type !== type || type === 'image/png') break
      q = nextQuality(q)
    }
  }
  return null
}
