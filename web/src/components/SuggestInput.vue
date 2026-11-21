<script setup lang="ts">
// A text field (or textarea with `multiline`) that suggests the values
// saved in it before: the user's own history of the field and, for a few
// short fields, the values used in the project (lib/suggest.ts; keys and
// behaviour in composables/suggest.ts). All other attributes and listeners
// go to the inner PrimeVue field, as with InputText/Textarea.
import { ref } from 'vue'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import SuggestList from './SuggestList.vue'
import { useSuggest } from '@/composables/suggest'
import type { SuggestField } from '@/lib/suggest'

defineOptions({ inheritAttrs: false })
const model = defineModel<string>({ default: '' })
// without `field` (or with autocomplete off) it is a plain field
const props = defineProps<{ field?: SuggestField; projectId?: string; multiline?: boolean }>()

const input = ref<{ $el: HTMLInputElement | HTMLTextAreaElement } | null>(null)
const sg = useSuggest({
  field: () => props.field,
  projectId: () => props.projectId,
  model,
  el: () => input.value?.$el ?? null,
  multiline: () => !!props.multiline,
})
const onKeydown = (e: KeyboardEvent) => void sg.onKeydown(e)
defineExpose({ focus: () => input.value?.$el?.focus() })
</script>

<template>
  <div class="suggest-input">
    <!-- the own keydown listener first: a key the list uses (Enter on a suggestion) stops the caller's -->
    <Textarea
      v-if="multiline"
      ref="input"
      v-model="model"
      @keydown="onKeydown"
      v-bind="{ ...$attrs, ...sg.attrs.value }"
      @input="sg.onInput"
      @blur="sg.onBlur"
      @focus="sg.onFocus"
    />
    <InputText v-else ref="input" v-model="model" @keydown="onKeydown" v-bind="{ ...$attrs, ...sg.attrs.value }" @input="sg.onInput" @blur="sg.onBlur" @focus="sg.onFocus" />
    <SuggestList :state="sg" />
  </div>
</template>

<style>
.suggest-input {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.suggest-input > .p-inputtext,
.suggest-input > .p-textarea {
  width: 100%;
}
/* inside an IconField (the search box): the icon's padding, as for a bare input */
.p-iconfield .suggest-input:not(:first-child) > .p-inputtext {
  padding-inline-start: calc((var(--p-form-field-padding-x) * 2) + var(--p-icon-size));
}
.p-iconfield .suggest-input:last-child > .p-inputtext:not(:last-child) {
  padding-inline-end: var(--p-form-field-padding-x);
}
</style>
