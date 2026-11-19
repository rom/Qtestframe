<script setup lang="ts">
import { ref } from 'vue'
import InputText from 'primevue/inputtext'
import { parseTags } from '@/lib/query'

const model = defineModel<string[]>({ default: () => [] })
defineProps<{ disabled?: boolean; placeholder?: string }>()
const text = ref('')

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
  <div class="row" style="gap: 6px; border: 1px solid var(--p-inputtext-border-color, var(--qtf-border)); border-radius: 6px; padding: 4px 6px; min-height: 38px">
    <span v-for="t in model" :key="t" class="tag-chip">
      <i class="pi pi-tag small muted" />{{ t }}
      <i v-if="!disabled" class="pi pi-times small clickable" role="button" :aria-label="'Remove ' + t" @click="remove(t)" />
    </span>
    <InputText
      v-if="!disabled"
      v-model="text"
      :placeholder="model.length ? '' : placeholder ?? 'Add tag and press Enter'"
      class="grow"
      style="border: none; box-shadow: none; padding: 4px; min-width: 120px; background: transparent"
      @keydown="onKey"
      @blur="commit"
    />
  </div>
</template>
