<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ProgressSpinner from 'primevue/progressspinner'
import QueryInput from '@/components/QueryInput.vue'
import { api, ApiError, asItems } from '@/lib/api'
import { groupBy } from '@/lib/collections'
import { queryTerms, splitByTerms } from '@/lib/highlight'
import type { Project, SearchHit } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'
import { usePrefsStore } from '@/stores/prefs'
import { rememberValues } from '@/lib/suggest'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const notify = useNotify()
const { t } = useI18n()

const q = ref((route.query.q as string) || '')
const types = ref<string[]>(((route.query.types as string) || 'requirement,test,run').split(','))
const projectId = ref<string | null>((route.query.project_id as string) || null)
const projects = ref<Project[]>([])
const hits = ref<SearchHit[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref<unknown>(null)
const searched = ref(false)
const prefs = usePrefsStore()
/** the query submitted from the search box, remembered for autocomplete when it works */
let submitted = ''

async function loadProjects() {
  if (!auth.tenant) return
  try {
    projects.value = asItems(await api.get<{ items: Project[] }>('/projects'))
  } catch {
    /* ignore */
  }
}
loadProjects()

async function search() {
  const query = q.value.trim()
  error.value = null
  if (!query) {
    hits.value = []
    searched.value = false
    return
  }
  loading.value = true
  try {
    const res = await api.get<{ items: SearchHit[]; total: number }>('/search', { q: query, types: types.value, project_id: projectId.value })
    hits.value = asItems(res)
    total.value = res?.total ?? hits.value.length
    searched.value = true
    if (submitted === query) void rememberValues(prefs.prefs, '', [{ field: 'search', value: query }])
    submitted = ''
  } catch (e) {
    error.value = e
    hits.value = []
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e, t('search.failed'))
  } finally {
    loading.value = false
  }
}
function submit() {
  submitted = q.value.trim()
  router.replace({ query: { q: q.value.trim() || undefined, types: types.value.join(','), project_id: projectId.value || undefined } })
}
watch(
  () => route.query,
  (qq) => {
    q.value = (qq.q as string) || ''
    if (qq.types) types.value = String(qq.types).split(',')
    projectId.value = (qq.project_id as string) || null
    search()
  },
  { immediate: true },
)

const typeMeta = computed<Record<string, { label: string; icon: string }>>(() => ({
  requirement: { label: enumLabel('searchType', 'requirement'), icon: 'pi pi-list-check' },
  test: { label: enumLabel('searchType', 'test'), icon: 'pi pi-check-square' },
  run: { label: enumLabel('searchType', 'run'), icon: 'pi pi-play-circle' },
}))
const groups = computed(() => groupBy(hits.value, (h) => h.type))
function link(h: SearchHit) {
  if (h.type === 'requirement') return { name: 'requirements', params: { projectId: h.project_id }, query: { item: h.id } }
  if (h.type === 'test') return { name: 'tests', params: { projectId: h.project_id }, query: { item: h.id } }
  return { name: 'run', params: { projectId: h.project_id, runId: h.id } }
}
const terms = computed(() => queryTerms(q.value))
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('search.title') }}</h1>
      <div class="sub">{{ t('search.subtitle', { tenant: auth.tenant?.name ?? t('search.currentTenant') }) }}</div>
    </div>
  </div>
  <div class="card" style="margin-bottom: 16px">
    <div class="filterbar" style="margin: 0">
      <QueryInput v-model="q" scope="requirement" suggest="search" :error="error" :placeholder="t('search.placeholder', { example: '&quot;time out&quot; title~/login/i status:approved' })" @submit="submit" />
      <Select v-model="projectId" :options="projects" option-value="id" :option-label="(p: Project) => `${p.key} · ${p.name}`" :placeholder="t('search.allProjects')" :aria-label="t('search.project')" show-clear style="min-width: 180px" />
      <fieldset class="row nowrap plain-fieldset">
        <legend class="sr-only">{{ t('search.types') }}</legend>
        <span v-for="(m, typ) in typeMeta" :key="typ" class="row nowrap">
          <Checkbox v-model="types" :input-id="'t-' + typ" :value="typ" />
          <label :for="'t-' + typ">{{ m.label }}</label>
        </span>
      </fieldset>
      <Button :label="t('common.search')" icon="pi pi-search" @click="submit" />
    </div>
  </div>
  <div v-if="loading" class="empty" aria-busy="true"><ProgressSpinner style="width: 36px; height: 36px" :aria-label="t('common.loading')" /></div>
  <template v-else-if="searched">
    <div class="small muted" style="margin-bottom: 8px" role="status">{{ t('counts.results', total) }}</div>
    <div v-if="!hits.length" class="card empty"><i class="pi pi-search" />{{ t('search.nothingFound') }}</div>
    <section v-for="g in groups" :key="g.key" class="card" style="margin-bottom: 16px" :aria-labelledby="'sg-' + g.key">
      <h2 :id="'sg-' + g.key"><i :class="typeMeta[g.key]?.icon ?? 'pi pi-file'" aria-hidden="true" /> {{ typeMeta[g.key]?.label ?? enumLabel('searchType', g.key) }} <span class="muted">({{ g.items.length }})</span></h2>
      <RouterLink v-for="h in g.items" :key="h.id" :to="link(h)" class="search-hit">
        <div class="row nowrap">
          <Tag :value="h.project_key" severity="secondary" />
          <span class="key">{{ h.key }}</span>
          <b class="ellipsis" style="min-width: 0">{{ h.title }}</b>
        </div>
        <div v-if="h.snippet" class="small muted" style="margin-top: 4px">
          <template v-for="(p, i) in splitByTerms(h.snippet, terms)" :key="i"><mark v-if="p.mark" class="hit">{{ p.text }}</mark><template v-else>{{ p.text }}</template></template>
        </div>
        <div v-if="h.matched_fields?.length" class="row" style="gap: 4px; margin-top: 4px">
          <span class="small muted">{{ t('search.matched') }}</span>
          <span v-for="f in h.matched_fields" :key="f" class="tag-chip" style="font-size: 11px; padding: 0 6px">{{ f }}</span>
        </div>
      </RouterLink>
    </section>
  </template>
  <div v-else class="card empty"><i class="pi pi-search" />{{ t('search.prompt') }}</div>
</template>
