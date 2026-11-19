// PrimeVue presets per theme, built from the registry with definePreset(Aura).
import Aura from '@primevue/themes/aura'
import { definePreset } from '@primevue/themes'
import { THEME_DEFS, type ConcreteThemeId, type ThemeDef } from './registry'

/** Preset key: light and dark share one preset (switched by .app-dark). */
export type PresetKey = 'base' | Exclude<ConcreteThemeId, 'light' | 'dark'>

export function presetKey(id: ConcreteThemeId): PresetKey {
  return id === 'light' || id === 'dark' ? 'base' : id
}

/**
 * Component tokens shared by every preset: Aura's severity buttons (green,
 * orange, red at 500) and secondary text buttons do not reach 4.5:1, so
 * light schemes use darker and dark schemes lighter shades, and secondary
 * text follows the theme's muted text colour (AA on cards and the page).
 */
export const A11Y_COMPONENTS = {
  button: {
    colorScheme: {
      light: {
        root: {
          success: { background: '{green.700}', hoverBackground: '{green.800}', activeBackground: '{green.900}', borderColor: '{green.700}', hoverBorderColor: '{green.800}', activeBorderColor: '{green.900}' },
          warn: { background: '{orange.700}', hoverBackground: '{orange.800}', activeBackground: '{orange.900}', borderColor: '{orange.700}', hoverBorderColor: '{orange.800}', activeBorderColor: '{orange.900}' },
          danger: { background: '{red.600}', hoverBackground: '{red.700}', activeBackground: '{red.800}', borderColor: '{red.600}', hoverBorderColor: '{red.700}', activeBorderColor: '{red.800}' },
          info: { background: '{sky.700}', hoverBackground: '{sky.800}', activeBackground: '{sky.900}', borderColor: '{sky.700}', hoverBorderColor: '{sky.800}', activeBorderColor: '{sky.900}' },
          help: { background: '{purple.600}', hoverBackground: '{purple.700}', activeBackground: '{purple.800}', borderColor: '{purple.600}', hoverBorderColor: '{purple.700}', activeBorderColor: '{purple.800}' },
        },
        outlined: {
          secondary: { color: '{text.muted.color}', borderColor: '{surface.300}' },
          success: { color: '{green.700}', borderColor: '{green.300}' },
          warn: { color: '{orange.700}', borderColor: '{orange.300}' },
          danger: { color: '{red.700}', borderColor: '{red.300}' },
          info: { color: '{sky.700}', borderColor: '{sky.300}' },
          help: { color: '{purple.700}', borderColor: '{purple.300}' },
        },
        text: {
          secondary: { color: '{text.muted.color}' },
          success: { color: '{green.700}' },
          warn: { color: '{orange.700}' },
          danger: { color: '{red.700}' },
          info: { color: '{sky.700}' },
          help: { color: '{purple.700}' },
        },
      },
      dark: {
        outlined: {
          secondary: { color: '{text.muted.color}', borderColor: '{surface.600}' },
          success: { color: '{green.300}' },
          warn: { color: '{orange.300}' },
          danger: { color: '{red.300}' },
          info: { color: '{sky.300}' },
          help: { color: '{purple.300}' },
        },
        text: {
          secondary: { color: '{text.muted.color}' },
          success: { color: '{green.300}' },
          warn: { color: '{orange.300}' },
          danger: { color: '{red.300}' },
          info: { color: '{sky.300}' },
          help: { color: '{purple.300}' },
        },
      },
    },
  },
  togglebutton: {
    colorScheme: {
      light: { root: { color: '{text.muted.color}' }, icon: { color: '{text.muted.color}' } },
      dark: { root: { color: '{text.muted.color}' }, icon: { color: '{text.muted.color}' } },
    },
  },
}

function basePreset() {
  const light = THEME_DEFS.light.colors!
  const dark = THEME_DEFS.dark.colors!
  return definePreset(Aura, {
    components: A11Y_COMPONENTS,
    semantic: {
      primary: THEME_DEFS.light.primary,
      colorScheme: {
        // indigo.600 instead of Aura's indigo.500: white text and links reach 4.5:1
        light: {
          primary: { color: light.primary, contrastColor: light.onPrimary, hoverColor: light.primaryHover, activeColor: '{primary.800}' },
          // muted text as in styles.css (Aura's slate.500 is 4.4:1 on the page background)
          text: { mutedColor: light.muted },
        },
        dark: {
          primary: { color: dark.primary, contrastColor: dark.onPrimary, hoverColor: dark.primaryHover, activeColor: '{primary.200}' },
          text: { mutedColor: dark.muted },
        },
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
    components: A11Y_COMPONENTS,
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
