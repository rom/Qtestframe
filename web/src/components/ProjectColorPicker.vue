<script setup lang="ts">
// Choose a project colour: the theme colour, one of the palette colours, or
// any colour (native colour input). A radio group for keyboard users.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { isHexColor, PROJECT_COLORS, readableOn } from '@/lib/projectColors'
import { uid } from '@/lib/a11y'

const model = defineModel<string>({ default: '' })
defineProps<{ disabled?: boolean }>()
const { t } = useI18n()
const name = uid('pcolor')
const custom = computed(() => isHexColor(model.value) && !(PROJECT_COLORS as readonly string[]).includes(model.value.toLowerCase()))
</script>

<template>
  <div class="color-picker" role="radiogroup" :aria-label="t('projectColor.label')">
    <label class="swatch theme" :class="{ on: !model }" :title="t('projectColor.theme')">
      <input type="radio" :name="name" value="" :checked="!model" :disabled="disabled" class="sr-only" @change="model = ''" />
      <i class="pi pi-palette" aria-hidden="true" />
      <span class="sr-only">{{ t('projectColor.theme') }}</span>
    </label>
    <label v-for="c in PROJECT_COLORS" :key="c" class="swatch" :class="{ on: model.toLowerCase() === c }" :style="{ background: c, color: readableOn(c) }" :title="c">
      <input type="radio" :name="name" :value="c" :checked="model.toLowerCase() === c" :disabled="disabled" class="sr-only" @change="model = c" />
      <i v-if="model.toLowerCase() === c" class="pi pi-check" aria-hidden="true" />
      <span class="sr-only">{{ t('projectColor.swatch', { color: c }) }}</span>
    </label>
    <label class="swatch custom" :class="{ on: custom }" :style="custom ? { background: model, color: readableOn(model) } : {}" :title="t('projectColor.custom')">
      <input type="color" :value="isHexColor(model) ? model : '#4f46e5'" :disabled="disabled" :aria-label="t('projectColor.custom')" @input="model = ($event.target as HTMLInputElement).value" />
      <i class="pi pi-plus" aria-hidden="true" />
    </label>
  </div>
</template>

<style scoped>
.color-picker {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.swatch {
  position: relative;
  display: inline-grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  border: 1px solid var(--qtf-border);
  cursor: pointer;
}
.swatch.theme {
  background: color-mix(in srgb, var(--p-primary-color) 15%, transparent);
  color: var(--p-primary-color);
}
.swatch.on {
  outline: 3px solid var(--p-primary-color);
  outline-offset: 2px;
}
.swatch:focus-within {
  outline: 3px solid var(--qtf-text);
  outline-offset: 2px;
}
.swatch.custom input {
  position: absolute;
  inset: 0;
  opacity: 0;
  width: 100%;
  height: 100%;
  cursor: pointer;
}
.swatch .pi {
  font-size: 13px;
}
</style>
