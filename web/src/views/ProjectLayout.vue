<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from 'vue'
import { useRoute } from 'vue-router'
import ProgressSpinner from 'primevue/progressspinner'
import Message from 'primevue/message'
import ProjectIcon from '@/components/ProjectIcon.vue'
import Button from 'primevue/button'
import SharingDialog from '@/components/SharingDialog.vue'
import ImpactGateHost from '@/components/ImpactGateHost.vue'
import { canShare, isRestricted } from '@/lib/sharing'
import { useAuthStore } from '@/stores/auth'
import { useProjectStore } from '@/stores/project'
import { isSoftware } from '@/lib/projectTypes'
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
const auth = useAuthStore()
// permission checks inside a project use its effective permissions (auth.can)
proj.scoped = true
onBeforeUnmount(() => (proj.scoped = false))
const shareOpen = ref(false)
async function onShared() {
  try {
    await proj.refresh()
  } catch {
    // no access any more (left a restricted project)
    error.value = t('sharing.noLongerVisible')
  }
}

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
  { to: 'exploratory', icon: 'pi pi-compass', label: t('nav.exploratory') },
  { to: 'cycles', icon: 'pi pi-sync', label: t('nav.cycles') },
  { to: 'releases', icon: 'pi pi-box', label: t('nav.releases') },
  { to: 'defects', icon: 'pi pi-exclamation-circle', label: t('nav.defects') },
  { to: 'risks', icon: 'pi pi-shield', label: t('nav.risks') },
  { to: 'waivers', icon: 'pi pi-verified', label: t('nav.waivers') },
  { to: 'discussions', icon: 'pi pi-comments', label: t('nav.discussions') },
  { to: 'traceability', icon: 'pi pi-th-large', label: t('nav.traceability') },
  { to: 'verification', icon: 'pi pi-verified', label: t('nav.verification') },
  { to: 'variants', icon: 'pi pi-clone', label: t('variants.nav') },
  { to: 'code', icon: 'pi pi-code', label: t('code.nav') },
  { to: 'trackerSync', icon: 'pi pi-arrow-right-arrow-left', label: t('nav.trackerSync') },
  { to: 'analytics', icon: 'pi pi-chart-bar', label: t('nav.analytics') },
  { to: 'duplicates', icon: 'pi pi-copy', label: t('nav.duplicates') },
  { to: 'soa', icon: 'pi pi-table', label: t('nav.soa') },
  { to: 'graph', icon: 'pi pi-sitemap', label: t('nav.graph') },
  { to: 'impact', icon: 'pi pi-bullseye', label: t('nav.impact') },
  { to: 'flow', icon: 'pi pi-share-alt', label: t('nav.flow') },
  ...(isSoftware(proj.project) ? [{ to: 'prompts', icon: 'pi pi-sparkles', label: t('nav.prompts') }] : []),
  { to: 'catalog', icon: 'pi pi-book', label: t('nav.catalog') },
  { to: 'tags', icon: 'pi pi-hashtag', label: t('nav.tags') },
  { to: 'baselines', icon: 'pi pi-flag', label: t('nav.baselines') },
  { to: 'documents', icon: 'pi pi-folder', label: t('nav.documents') },
  { to: 'reports', icon: 'pi pi-file-pdf', label: t('nav.reports') },
  { to: 'settings', icon: 'pi pi-tags', label: t('nav.settings') },
  { to: 'export', icon: 'pi pi-download', label: t('nav.export') },
])
const ready = computed(() => proj.project?.id === props.projectId)
const isRunsChild = computed(() => route.name === 'run' || route.name === 'execute')
const isExploratoryChild = computed(() => route.name === 'exploratorySession')
const childActive = (to: string) => (to === 'runs' && isRunsChild.value) || (to === 'exploratory' && isExploratoryChild.value) || (to === 'cycles' && route.name === 'cycle')
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
      <i v-if="isRestricted(proj.project)" class="pi pi-lock muted small" v-tooltip="t('sharing.restrictedTip')" tabindex="0" role="img" :aria-label="t('sharing.restrictedTip')" />
      <span class="grow" />
      <Button v-if="canShare(auth.me, proj.project)" class="project-share" :label="t('sharing.share')" icon="pi pi-share-alt" size="small" severity="secondary" outlined @click="shareOpen = true" />
    </nav>
    <SharingDialog v-model:visible="shareOpen" :project="proj.project" @changed="onShared" />
    <nav class="tabs-nav" :aria-label="t('nav.projectSections')">
      <RouterLink
        v-for="tab in tabs"
        :key="tab.to"
        :to="{ name: tab.to, params: { projectId } }"
        :class="{ 'router-link-active': childActive(tab.to) }"
        :aria-current="childActive(tab.to) ? 'page' : undefined"
      >
        <i :class="tab.icon" aria-hidden="true" />{{ tab.label }}
      </RouterLink>
    </nav>
    <RouterView :key="projectId" />
    <ImpactGateHost />
  </template>
</template>
