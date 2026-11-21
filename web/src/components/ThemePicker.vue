<script setup lang="ts">
// Colour theme picker: a Select listing every theme with a small swatch.
//
// Standalone (default, `save` true): shows the current preference and, on
// change, applies the theme at once and stores it via prefs.setTheme()
// (PATCH /auth/me). In a form, pass v-model and :save="false" to only emit.
import { computed } from 'vue'
import Select from 'primevue/select'
import { usePrefsStore } from '@/stores/prefs'
import { THEME_DEFS, THEME_IDS, THEME_LABELS, THEME_TEXT, type ThemeId } from '@/themes/registry'

type Value = ThemeId | ''

const props = withDefaults(
  defineProps<{
    /** selected value; omit to show the stored preference */
    modelValue?: Value | null
    /** apply and save the choice through the prefs store (default true) */
    save?: boolean
    /** offer "not set" ('') as first option */
    allowUnset?: boolean
    /** display name of a theme ('' = not set); wins over `t` */
    labelFor?: (theme: Value) => string
    /** translation function; keys: theme.<id>, theme.unset, theme.picker */
    t?: (key: string) => string
    /** accessible name of the control (default: t('theme.picker') or "Colour theme") */
    ariaLabel?: string
    inputId?: string
    /** show only the swatch in the closed control (for the top bar) */
    compact?: boolean
  }>(),
  { modelValue: undefined, save: true, allowUnset: false, labelFor: undefined, t: undefined, ariaLabel: undefined, inputId: undefined, compact: false },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: Value): void
  (e: 'change', value: Value): void
}>()

const prefs = usePrefsStore()

function translate(key: string, fallback: string): string {
  if (!props.t) return fallback
  const s = props.t(key)
  return s && s !== key ? s : fallback
}

function label(v: Value): string {
  if (props.labelFor) return props.labelFor(v)
  return v === '' ? translate('theme.unset', THEME_TEXT.unset) : translate(`theme.${v}`, THEME_LABELS[v])
}

const pickerLabel = computed(() => props.ariaLabel ?? translate('theme.picker', THEME_TEXT.picker))

const options = computed(() => {
  const ids: Value[] = props.allowUnset ? ['', ...THEME_IDS] : [...THEME_IDS]
  return ids.map((value) => ({ value, label: label(value) }))
})

const current = computed<Value>(() => {
  if (props.modelValue !== undefined && props.modelValue !== null) return props.modelValue
  if (props.modelValue === null) return ''
  return prefs.themePref || (props.allowUnset ? '' : prefs.theme)
})

function swatch(v: Value) {
  // "not set" previews what is painted now
  return THEME_DEFS[v === '' ? prefs.theme : v].swatch
}

function onChange(v: Value | null) {
  const value: Value = v ?? ''
  emit('update:modelValue', value)
  emit('change', value)
  if (props.save && value) void prefs.setTheme(value)
}
</script>

<template>
  <Select
    :model-value="current"
    :options="options"
    option-label="label"
    option-value="value"
    :input-id="inputId"
    :aria-label="pickerLabel"
    :class="['theme-picker', { 'theme-picker-compact': compact }]"
    scroll-height="min(75vh, 46rem)"
    overlay-class="theme-picker-overlay"
    @update:model-value="onChange"
  >
    <template #value="{ value }">
      <span class="theme-picker-item" :title="compact ? label(value ?? '') : undefined">
        <span class="theme-swatch" aria-hidden="true">
          <span :style="{ background: swatch(value ?? '').chrome }" />
          <span :style="{ background: swatch(value ?? '').bg }" />
          <span :style="{ background: swatch(value ?? '').card }" />
          <span :style="{ background: swatch(value ?? '').accent }" />
        </span>
        <span :class="compact ? 'theme-picker-sr' : 'theme-picker-name'">{{ label(value ?? '') }}</span>
      </span>
    </template>
    <template #option="{ option }">
      <span class="theme-picker-item">
        <span class="theme-swatch" aria-hidden="true">
          <span :style="{ background: swatch(option.value).chrome }" />
          <span :style="{ background: swatch(option.value).bg }" />
          <span :style="{ background: swatch(option.value).card }" />
          <span :style="{ background: swatch(option.value).accent }" />
        </span>
        <span class="theme-picker-name">{{ option.label }}</span>
      </span>
    </template>
  </Select>
</template>

<style>
/* The overlay is teleported to <body>, so these rules are not scoped. The
   list is tall enough for most themes at once, and its scrollbar stays
   visible: overlay scrollbars (macOS, some Linux desktops) hide until you
   scroll, so the themes below the fold looked missing. */
.theme-picker-overlay .p-select-list-container {
  overflow-y: scroll;
}
/* Chrome ignores ::-webkit-scrollbar once scrollbar-color/-width is set,
   so the standard properties are only for browsers without it (Firefox). */
@supports not selector(::-webkit-scrollbar) {
  .theme-picker-overlay .p-select-list-container {
    scrollbar-width: auto;
    scrollbar-color: var(--p-text-muted-color, #888) transparent;
  }
}
.theme-picker-overlay .p-select-list-container::-webkit-scrollbar {
  width: 10px;
}
.theme-picker-overlay .p-select-list-container::-webkit-scrollbar-thumb {
  background: var(--p-text-muted-color, #888);
  border-radius: 5px;
  border: 2px solid transparent;
  background-clip: padding-box;
}
.theme-picker-overlay .p-select-list-container::-webkit-scrollbar-track {
  background: transparent;
}
</style>

<style scoped>
.theme-picker-item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.theme-picker-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.theme-swatch {
  display: inline-grid;
  grid-template-columns: 8px 8px 8px 8px;
  height: 16px;
  border-radius: 4px;
  overflow: hidden;
  flex: none;
  box-shadow: 0 0 0 1px color-mix(in srgb, currentColor 35%, transparent);
}
.theme-swatch > span {
  display: block;
}
.theme-picker-sr {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}
</style>
