// Preset avatars and project icons shipped with the client ("preset:<name>",
// name = file base name). The images are simple original SVGs.
import { presetMap } from '@/lib/avatars'

export const AVATAR_PRESETS: Record<string, string> = presetMap(
  import.meta.glob('./avatars/*.svg', { eager: true, query: '?url', import: 'default' }) as Record<string, string>,
)

export const ICON_PRESETS: Record<string, string> = presetMap(
  import.meta.glob('./project-icons/*.svg', { eager: true, query: '?url', import: 'default' }) as Record<string, string>,
)
