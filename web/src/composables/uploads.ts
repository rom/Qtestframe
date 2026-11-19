// Upload queue with progress for attachment and archive uploads.
import { ref } from 'vue'
import { upload, type UploadOptions } from '@/lib/upload'
import { errorMessage } from '@/lib/api'

export interface QueuedUpload {
  id: number
  name: string
  size: number
  percent: number
  error: string
  done: boolean
}

let seq = 0

export function useUploads() {
  const queue = ref<QueuedUpload[]>([])
  /** last finished event, for a polite live region */
  const announcement = ref('')

  /** Upload files one after another; resolves the server responses of the successful ones. */
  async function run<T>(files: File[], path: string, opts: Omit<UploadOptions, 'onProgress'> = {}, labels: { done: (name: string) => string; failed: (name: string, error: string) => string }): Promise<T[]> {
    const out: T[] = []
    const items = files.map((f) => ({ id: ++seq, name: f.name, size: f.size, percent: 0, error: '', done: false }))
    queue.value = [...queue.value.filter((q) => !q.done || q.error), ...items]
    for (let i = 0; i < files.length; i++) {
      const item = queue.value.find((q) => q.id === items[i].id)!
      try {
        out.push(await upload<T>(path, files[i], { ...opts, onProgress: (p) => (item.percent = p) }))
        item.done = true
        item.percent = 100
        announcement.value = labels.done(item.name)
      } catch (e) {
        item.done = true
        item.error = errorMessage(e)
        announcement.value = labels.failed(item.name, item.error)
      }
    }
    // keep failed rows visible, drop finished ones after a moment
    setTimeout(() => (queue.value = queue.value.filter((q) => !q.done || q.error)), 2500)
    return out
  }
  function dismiss(id: number) {
    queue.value = queue.value.filter((q) => q.id !== id)
  }
  return { queue, announcement, run, dismiss }
}
