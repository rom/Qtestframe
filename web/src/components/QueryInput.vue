<script setup lang="ts">
import { computed, ref } from 'vue'
import InputText from 'primevue/inputtext'
import Button from 'primevue/button'
import Popover from 'primevue/popover'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import QueryHelp from './QueryHelp.vue'
import { ApiError } from '@/lib/api'
import { positionToIndex } from '@/lib/highlight'
import type { QUERY_FIELDS } from '@/lib/query'

const model = defineModel<string>({ default: '' })
const props = defineProps<{ scope: keyof typeof QUERY_FIELDS; error?: unknown; placeholder?: string; composed?: string }>()
const emit = defineEmits<{ submit: [] }>()
const help = ref<InstanceType<typeof Popover>>()

/** Show `invalid_query` errors with a caret under the reported position. */
const queryError = computed(() => {
  const e = props.error
  if (!(e instanceof ApiError) || e.code !== 'invalid_query') return null
  const expr = props.composed ?? model.value
  if (e.position === undefined || e.position === null) return { message: e.message, expr: '', caret: '' }
  const idx = positionToIndex(expr, e.position)
  return { message: e.message, expr, caret: ' '.repeat(idx) + '^' }
})
</script>

<template>
  <div class="grow" style="min-width: 260px">
    <div class="row nowrap">
      <IconField class="grow">
        <InputIcon class="pi pi-filter" />
        <InputText
          v-model="model"
          class="w-full mono"
          :invalid="!!queryError"
          :placeholder="placeholder ?? 'Filter, e.g. status:approved obligation:must -tag:legacy'"
          aria-label="Filter expression"
          @keydown.enter.prevent="emit('submit')"
        />
      </IconField>
      <Button icon="pi pi-question-circle" text rounded severity="secondary" aria-label="Query help" v-tooltip.bottom="'Query syntax'" @click="help?.toggle($event)" />
      <Button v-if="model" icon="pi pi-times" text rounded severity="secondary" aria-label="Clear" @click="model = ''; emit('submit')" />
    </div>
    <div v-if="queryError" class="query-error">
      <div>{{ queryError.message }}</div>
      <div v-if="queryError.expr">{{ queryError.expr }}</div>
      <div v-if="queryError.caret">{{ queryError.caret }}</div>
    </div>
    <Popover ref="help"><QueryHelp :scope="scope" /></Popover>
  </div>
</template>
