// PrimeVue presets per theme, built from the registry with definePreset(Aura).
import Aura from '@primevue/themes/aura'
import { definePreset } from '@primevue/themes'
import { THEME_DEFS, type ConcreteThemeId, type ThemeDef } from './registry'

/** Preset key: light and dark share one preset (switched by .app-dark). */
export type PresetKey = 'base' | Exclude<ConcreteThemeId, 'light' | 'dark'>

export function presetKey(id: ConcreteThemeId): PresetKey {
  return id === 'light' || id === 'dark' ? 'base' : id
}

function basePreset() {
  const light = THEME_DEFS.light.colors!
  const dark = THEME_DEFS.dark.colors!
  return definePreset(Aura, {
    semantic: {
      primary: THEME_DEFS.light.primary,
      colorScheme: {
        // indigo.600 instead of Aura's indigo.500: white text and links reach 4.5:1
        light: { primary: { color: light.primary, contrastColor: light.onPrimary, hoverColor: light.primaryHover, activeColor: '{primary.800}' } },
        dark: { primary: { color: dark.primary, contrastColor: dark.onPrimary, hoverColor: dark.primaryHover, activeColor: '{primary.200}' } },
      },
    },
  })
}

/** Design tokens of a decorative theme (exported for tests). */
export function presetTokens(def: ThemeDef) {
  const c = def.colors!
  const dark = def.scheme === 'dark'
  const scheme = {
    surface: def.surface,
    primary: { color: c.primary, contrastColor: c.onPrimary, hoverColor: c.primaryHover, activeColor: dark ? '{primary.200}' : '{primary.800}' },
    highlight: dark
      ? { background: `color-mix(in srgb, ${c.primary}, transparent 82%)`, focusBackground: `color-mix(in srgb, ${c.primary}, transparent 74%)`, color: c.text, focusColor: c.text }
      : { background: '{primary.50}', focusBackground: '{primary.100}', color: '{primary.800}', focusColor: '{primary.900}' },
    text: { color: c.text, hoverColor: c.text, mutedColor: c.muted, hoverMutedColor: c.text },
    content: { background: c.card, borderColor: c.border },
    formField: dark
      ? { background: c.bg, color: c.text, placeholderColor: c.muted, floatLabelColor: c.muted, borderColor: '{surface.600}' }
      : { background: c.card, color: c.text, placeholderColor: c.muted, floatLabelColor: c.muted },
    overlay: {
      select: { background: c.card, borderColor: c.border, color: c.text },
      popover: { background: c.card, borderColor: c.border, color: c.text },
      modal: { background: c.card, borderColor: c.border, color: c.text },
    },
  }
  const tokens: Record<string, unknown> = {
    semantic: {
      primary: def.primary,
      colorScheme: { light: dark ? { surface: def.surface } : scheme, dark: dark ? scheme : { surface: def.surface } },
    },
  }
  if (def.radius) {
    const r = def.radius
    tokens.primitive = { borderRadius: { none: '0', xs: `${Math.round(r / 4)}px`, sm: `${Math.round(r / 2)}px`, md: `${Math.round((r * 2) / 3)}px`, lg: `${r}px`, xl: `${r + 4}px` } }
  }
  return tokens
}

const cache = new Map<PresetKey, unknown>()

/** The PrimeVue preset for a painted theme (built once, then cached). */
export function presetFor(id: ConcreteThemeId): unknown {
  const key = presetKey(id)
  let p = cache.get(key)
  if (!p) {
    p = key === 'base' ? basePreset() : definePreset(Aura, presetTokens(THEME_DEFS[key]))
    cache.set(key, p)
  }
  return p
}
