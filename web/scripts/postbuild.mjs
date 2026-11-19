// Vite empties the output directory; the Go embed needs the placeholder kept.
import { writeFileSync, mkdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const dist = resolve(dirname(fileURLToPath(import.meta.url)), '../../internal/webui/dist')
mkdirSync(dist, { recursive: true })
writeFileSync(resolve(dist, '.gitkeep'), '')
