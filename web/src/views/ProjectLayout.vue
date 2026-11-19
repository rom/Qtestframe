<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import ProgressSpinner from 'primevue/progressspinner'
import Message from 'primevue/message'
import { useProjectStore } from '@/stores/project'
import { usePrefsStore } from '@/stores/prefs'
import { errorMessage } from '@/lib/api'
import { ref } from 'vue'

const props = defineProps<{ projectId: string }>()
const proj = useProjectStore()
const prefs = usePrefsStore()
const route = useRoute()
const error = ref('')

watch(
  () => props.projectId,
  async (id) => {
    error.value = ''
    try {
      await proj.open(id)
      prefs.rememberProject(id)
    } catch (e) {
      error.value = errorMessage(e)
    }
  },
  { immediate: true },
)

const tabs = [
  { to: 'dashboard', icon: 'pi pi-chart-bar', label: 'Dashboard' },
  { to: 'requirements', icon: 'pi pi-list-check', label: 'Requirements' },
  { to: 'tests', icon: 'pi pi-check-square', label: 'Tests' },
  { to: 'runs', icon: 'pi pi-play-circle', label: 'Operations' },
  { to: 'traceability', icon: 'pi pi-th-large', label: 'Traceability' },
  { to: 'baselines', icon: 'pi pi-flag', label: 'Baselines' },
  { to: 'settings', icon: 'pi pi-tags', label: 'Categories & settings' },
  { to: 'export', icon: 'pi pi-download', label: 'Export' },
]
const ready = computed(() => proj.project?.id === props.projectId)
const isRunsChild = computed(() => route.name === 'run' || route.name === 'execute')
</script>

<template>
  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <div v-else-if="!ready" class="empty"><ProgressSpinner style="width: 40px; height: 40px" /></div>
  <template v-else>
    <div class="row" style="margin-bottom: 4px">
      <RouterLink :to="{ name: 'projects' }" class="muted small">Projects</RouterLink>
      <i class="pi pi-angle-right muted small" />
      <span class="small" style="font-weight: 600">{{ proj.project?.name }}</span>
      <span class="key">{{ proj.project?.key }}</span>
    </div>
    <nav class="tabs-nav">
      <RouterLink
        v-for="t in tabs"
        :key="t.to"
        :to="{ name: t.to, params: { projectId } }"
        :class="{ 'router-link-active': t.to === 'runs' && isRunsChild }"
      >
        <i :class="t.icon" />{{ t.label }}
      </RouterLink>
    </nav>
    <RouterView :key="projectId" />
  </template>
</template>
