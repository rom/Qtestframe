<script setup lang="ts">
// "Started <date> by <user>": when a project was started (created, or
// loaded or duplicated from a project archive) and by whom. The creator is
// left out when unknown (seeded projects, the command line, data older
// than the field) or when the account no longer exists.
import { computed } from 'vue'
import { formatDate, formatDateTime } from '@/lib/format'
import { projectCreator } from '@/lib/projectCreator'
import type { Project } from '@/lib/types'

const props = defineProps<{ project: Pick<Project, 'created_at' | 'created_by' | 'created_by_name' | 'created_by_email'>; icon?: boolean }>()

const who = computed(() => projectCreator(props.project))
const date = computed(() => formatDate(props.project.created_at))
</script>

<template>
  <span class="project-started">
    <i v-if="icon" class="pi pi-calendar" aria-hidden="true" />
    <i18n-t :keypath="who ? 'projects.startedOnBy' : 'projects.startedOn'" tag="span">
      <template #date><time :datetime="project.created_at" :title="formatDateTime(project.created_at)">{{ date }}</time></template>
      <template v-if="who" #user><span class="project-started-user" :title="who.email && who.email !== who.name ? who.email : undefined">{{ who.name }}</span></template>
    </i18n-t>
  </span>
</template>
