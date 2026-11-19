<script setup lang="ts">
import { ref } from 'vue'
import InputText from 'primevue/inputtext'
import { parseTags } from '@/lib/query'
import { useI18n } from 'vue-i18n'

const model = defineModel<string[]>({ default: () => [] })
defineProps<{ disabled?: boolean; placeholder?: string; icon?: string; ariaLabel?: string }>()
const text = ref('')
const { t } = useI18n()

function commit() {
  const add = parseTags(text.value).filter((t) => !model.value.includes(t))
  if (add.length) model.value = [...model.value, ...add]
  text.value = ''
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Enter' || e.key === ',') {
    e.preventDefault()
    commit()
  } else if (e.key === 'Backspace' && !text.value && model.value.length) {
    model.value = model.value.slice(0, -1)
  }
}
function remove(t: string) {
  model.value = model.value.filter((x) => x !== t)
}
</script>

<template>
  <div class="row tags-input" style="gap: 6px; border: 1px solid var(--p-inputtext-border-color, var(--qtf-border)); border-radius: 6px; padding: 4px 6px; min-height: 38px">
    <span v-for="tag in model" :key="tag" class="tag-chip">
      <i :class="icon ?? 'pi pi-tag'" class="small muted" aria-hidden="true" />{{ tag }}
      <button v-if="!disabled" type="button" class="tag-remove" :aria-label="t('tagsInput.remove', { tag })" @click="remove(tag)"><i class="pi pi-times small" aria-hidden="true" /></button>
    </span>
    <InputText
      v-if="!disabled"
      v-model="text"
      :placeholder="model.length ? '' : placeholder ?? t('tagsInput.placeholder')"
      class="grow"
      :aria-label="ariaLabel"
      style="border: none; box-shadow: none; padding: 4px; min-width: 0; flex-basis: 120px; background: transparent"
      @keydown="onKey"
      @blur="commit"
    />
  </div>
</template>
