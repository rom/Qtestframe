<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import ProgressSpinner from 'primevue/progressspinner'
import Message from 'primevue/message'
import ProjectIcon from '@/components/ProjectIcon.vue'
import { useProjectStore } from '@/stores/project'
import { usePrefsStore } from '@/stores/prefs'
import { errorMessage } from '@/lib/api'
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string }>()
const proj = useProjectStore()
const prefs = usePrefsStore()
const route = useRoute()
const error = ref('')
const { t } = useI18n()

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

const tabs = computed(() => [
  { to: 'dashboard', icon: 'pi pi-chart-bar', label: t('nav.dashboard') },
  { to: 'requirements', icon: 'pi pi-list-check', label: t('nav.requirements') },
  { to: 'tests', icon: 'pi pi-check-square', label: t('nav.tests') },
  { to: 'runs', icon: 'pi pi-play-circle', label: t('nav.operations') },
  { to: 'traceability', icon: 'pi pi-th-large', label: t('nav.traceability') },
  { to: 'graph', icon: 'pi pi-sitemap', label: t('nav.graph') },
  { to: 'flow', icon: 'pi pi-share-alt', label: t('nav.flow') },
  { to: 'tags', icon: 'pi pi-hashtag', label: t('nav.tags') },
  { to: 'baselines', icon: 'pi pi-flag', label: t('nav.baselines') },
  { to: 'documents', icon: 'pi pi-folder', label: t('nav.documents') },
  { to: 'reports', icon: 'pi pi-file-pdf', label: t('nav.reports') },
  { to: 'settings', icon: 'pi pi-tags', label: t('nav.settings') },
  { to: 'export', icon: 'pi pi-download', label: t('nav.export') },
])
const ready = computed(() => proj.project?.id === props.projectId)
const isRunsChild = computed(() => route.name === 'run' || route.name === 'execute')
</script>

<template>
  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <div v-else-if="!ready" class="empty" aria-busy="true"><ProgressSpinner style="width: 40px; height: 40px" :aria-label="t('common.loading')" /></div>
  <template v-else>
    <nav class="row breadcrumb" style="margin-bottom: 4px" :aria-label="t('nav.breadcrumb')">
      <RouterLink :to="{ name: 'projects' }" class="muted small">{{ t('nav.breadcrumbProjects') }}</RouterLink>
      <i class="pi pi-angle-right muted small" aria-hidden="true" />
      <ProjectIcon :icon="proj.project?.icon" :project-key="proj.project?.key" :color="proj.project?.color" :size="20" />
      <span class="small" style="font-weight: 600" aria-current="page">{{ proj.project?.name }}</span>
      <span class="key">{{ proj.project?.key }}</span>
    </nav>
    <nav class="tabs-nav" :aria-label="t('nav.projectSections')">
      <RouterLink
        v-for="tab in tabs"
        :key="tab.to"
        :to="{ name: tab.to, params: { projectId } }"
        :class="{ 'router-link-active': tab.to === 'runs' && isRunsChild }"
        :aria-current="tab.to === 'runs' && isRunsChild ? 'page' : undefined"
      >
        <i :class="tab.icon" aria-hidden="true" />{{ tab.label }}
      </RouterLink>
    </nav>
    <RouterView :key="projectId" />
  </template>
</template>
