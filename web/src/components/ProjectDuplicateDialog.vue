<script setup lang="ts">
// Duplicate project: a copy with a new key and name, with everything in it
// (POST /projects/{pid}/duplicate, the same code as save + load).
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import { api, errorMessage } from '@/lib/api'
import { freeProjectKey, normalizeProjectKey, validProjectKey, type ProjectLoadResult } from '@/lib/projectArchive'
import type { Project } from '@/lib/types'
import { useI18n } from 'vue-i18n'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ project: Project | null; takenKeys: string[] }>()
const emit = defineEmits<{ duplicated: [project: Project, result: ProjectLoadResult] }>()
const { t } = useI18n()

const key = ref('')
const name = ref('')
const saving = ref(false)
const error = ref('')
watch(visible, (v) => {
  if (!v || !props.project) return
  key.value = freeProjectKey(props.project.key, props.takenKeys)
  name.value = t('projectArchive.copyName', { name: props.project.name })
  error.value = ''
})
const keyTaken = computed(() => props.takenKeys.some((k) => k.toUpperCase() === normalizeProjectKey(key.value)))
const keyProblem = computed(() => {
  if (!key.value) return ''
  if (!validProjectKey(key.value)) return t('projectArchive.keyInvalid')
  if (keyTaken.value) return t('projectArchive.keyTaken', { key: normalizeProjectKey(key.value) })
  return ''
})
const ready = computed(() => !!props.project && validProjectKey(key.value) && !keyTaken.value && !!name.value.trim() && !saving.value)

async function save() {
  if (!props.project || !ready.value) return
  saving.value = true
  error.value = ''
  try {
    const res = await api.post<ProjectLoadResult>(`/projects/${props.project.id}/duplicate`, { key: normalizeProjectKey(key.value), name: name.value.trim() })
    visible.value = false
    if (res.project) emit('duplicated', res.project, res)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" :header="t('projectArchive.duplicateTitle', { name: project?.name ?? '' })" modal class="project-duplicate-dialog" style="width: min(480px, 95vw)" :closable="!saving">
    <form id="project-duplicate-form" class="grid" style="gap: 12px" @submit.prevent="save">
      <p class="small muted" style="margin: 0">{{ t('projectArchive.duplicateText') }}</p>
      <div class="field">
        <label for="pd-key">{{ t('common.key') }}</label>
        <InputText
          id="pd-key"
          v-model="key"
          class="mono"
          style="text-transform: uppercase"
          autocomplete="off"
          spellcheck="false"
          required
          autofocus
          :invalid="!!keyProblem"
          :aria-describedby="keyProblem ? 'pd-key-hint pd-key-problem' : 'pd-key-hint'"
        />
        <small id="pd-key-hint" class="muted">{{ t('projectArchive.keyHint', { example: (normalizeProjectKey(key) || 'KEY') + '-REQ-1' }) }}</small>
        <small v-if="keyProblem" id="pd-key-problem" class="warn-text" aria-live="polite">{{ keyProblem }}</small>
      </div>
      <div class="field">
        <label for="pd-name">{{ t('common.name') }}</label>
        <InputText id="pd-name" v-model="name" required maxlength="200" />
      </div>
      <Message v-if="error" severity="error" :closable="false"><span role="alert">{{ error }}</span></Message>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" severity="secondary" text :disabled="saving" @click="visible = false" />
        <Button type="submit" :label="t('projectArchive.duplicate')" icon="pi pi-copy" :loading="saving" :disabled="!ready" />
      </div>
    </form>
  </Dialog>
</template>
