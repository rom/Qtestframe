<script setup lang="ts">
// Drop zone with an equivalent "Choose files" button; checks the size limit
// and hands accepted files to the parent.
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import { useI18n } from 'vue-i18n'
import { DEFAULT_MAX_UPLOAD_MB, MB, uploadProblem } from '@/lib/files'
import { uid } from '@/lib/a11y'

const props = withDefaults(defineProps<{ maxMb?: number; multiple?: boolean; disabled?: boolean; accept?: string; label?: string }>(), {
  maxMb: DEFAULT_MAX_UPLOAD_MB,
  multiple: true,
  disabled: false,
  accept: '',
  label: '',
})
const emit = defineEmits<{ files: [files: File[]]; rejected: [names: string[]] }>()
const { t } = useI18n()
const input = ref<HTMLInputElement>()
const over = ref(false)
const hintId = uid('drop-hint')
const message = ref('')
const limitText = computed(() => (props.maxMb >= 1024 ? '' : t('files.limit', { size: `${props.maxMb} MB` })))

function take(list: FileList | File[] | null | undefined) {
  if (!list || props.disabled) return
  const files = Array.from(list).slice(0, props.multiple ? undefined : 1)
  const ok: File[] = []
  const bad: string[] = []
  for (const f of files) {
    const p = uploadProblem(f, props.maxMb * MB)
    if (p) bad.push(p === 'too_large' ? t('files.tooLarge', { name: f.name, size: `${props.maxMb} MB` }) : t('files.emptyFile', { name: f.name }))
    else ok.push(f)
  }
  message.value = bad.join(' ')
  if (bad.length) emit('rejected', bad)
  if (ok.length) emit('files', ok)
}
function onDrop(e: DragEvent) {
  over.value = false
  take(e.dataTransfer?.files)
}
function onChange(e: Event) {
  const el = e.target as HTMLInputElement
  take(el.files)
  el.value = ''
}
defineExpose({ open: () => input.value?.click() })
</script>

<template>
  <div
    class="dropzone"
    :class="{ over, disabled }"
    @dragenter.prevent="over = !disabled"
    @dragover.prevent="over = !disabled"
    @dragleave.prevent="over = false"
    @drop.prevent="onDrop"
  >
    <i class="pi pi-cloud-upload" aria-hidden="true" />
    <div class="small">
      <span>{{ t('files.dropHere') }}</span>
      {{ ' ' }}<span :id="hintId" class="muted">{{ limitText }}</span>
    </div>
    <Button :label="label || t('files.choose')" icon="pi pi-paperclip" size="small" severity="secondary" outlined :disabled="disabled" :aria-describedby="hintId" @click="input?.click()" />
    <input ref="input" type="file" style="display: none" :multiple="multiple" :accept="accept || undefined" @change="onChange" />
    <div v-if="message" class="small error-text" role="alert" style="flex-basis: 100%">{{ message }}</div>
  </div>
</template>

<style scoped>
.dropzone {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  padding: 12px 14px;
  border: 2px dashed var(--qtf-border);
  border-radius: var(--qtf-radius);
  background: var(--qtf-card);
}
.dropzone > .pi {
  font-size: 22px;
  color: var(--p-primary-color);
}
.dropzone > .small {
  flex: 1 1 180px;
  min-width: 0;
}
.dropzone.over {
  border-color: var(--p-primary-color);
  background: color-mix(in srgb, var(--p-primary-color) 8%, var(--qtf-card));
}
.dropzone.disabled {
  opacity: 0.7;
}
</style>
