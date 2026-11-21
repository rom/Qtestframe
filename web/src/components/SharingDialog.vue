<script setup lang="ts">
// The Share dialog of a project (project header and projects list).
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import SharingPanel from '@/components/SharingPanel.vue'
import type { Project, ProjectSharing } from '@/lib/types'
import { useI18n } from 'vue-i18n'

defineProps<{ project: Project | null }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ changed: [sharing: ProjectSharing] }>()
const { t } = useI18n()
</script>

<template>
  <Dialog v-model:visible="visible" :header="project ? t('sharing.dialogTitle', { name: project.name }) : t('sharing.title')" modal class="sharing-dialog" style="width: min(760px, 96vw)">
    <SharingPanel v-if="project && visible" :project="project" @changed="(s) => emit('changed', s)" />
    <template #footer>
      <Button :label="t('common.close')" severity="secondary" text @click="visible = false" />
    </template>
  </Dialog>
</template>
