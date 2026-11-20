<script setup lang="ts">
// Tag cloud of a project (GET /projects/{pid}/tags): every tag sized by how
// often it is used. Select one or more tags to list the requirements, tests
// and documents that carry all (or any) of them.
import { tagClass } from '@/lib/tagColors'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import SelectButton from 'primevue/selectbutton'
import HelpLink from '@/guide/HelpLink.vue'
import StatusTag from '@/components/StatusTag.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { contentUrl } from '@/lib/files'
import { cloudSize, sortTags, tagQuery, type TagCount } from '@/lib/tagCloud'
import type { FileInfo, Requirement, TestCase } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const proj = useProjectStore()
const route = useRoute()
const router = useRouter()

const tags = ref<TagCount[]>([])
const loading = ref(false)
const error = ref('')
const filter = ref('')
const sortBy = ref<'count' | 'name'>('count')
const mode = ref<'all' | 'any'>(route.query.mode === 'any' ? 'any' : 'all')
const selected = ref<string[]>(typeof route.query.tag === 'string' ? [route.query.tag] : Array.isArray(route.query.tag) ? (route.query.tag.filter(Boolean) as string[]) : [])

async function load() {
  loading.value = true
  error.value = ''
  try {
    tags.value = asItems(await api.get<{ items: TagCount[] }>(`/projects/${proj.pid}/tags`))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(() => proj.pid, load)

const shown = computed(() => {
  const f = filter.value.trim().toLowerCase()
  return sortTags(tags.value, sortBy.value).filter((x) => !f || x.tag.toLowerCase().includes(f))
})
const minMax = computed(() => {
  const totals = tags.value.map((x) => x.total)
  return { min: Math.min(...totals, 1), max: Math.max(...totals, 1) }
})
const isOn = (tag: string) => selected.value.includes(tag)
function toggle(tag: string) {
  selected.value = isOn(tag) ? selected.value.filter((x) => x !== tag) : [...selected.value, tag]
}

// ---------- items with the selected tags ----------
const q = computed(() => tagQuery(selected.value, mode.value))
const reqs = ref<Requirement[]>([])
const tests = ref<TestCase[]>([])
const docs = ref<FileInfo[]>([])
const itemsLoading = ref(false)
watch(
  q,
  async (query) => {
    router.replace({ query: { ...route.query, tag: selected.value.length ? selected.value : undefined, mode: mode.value === 'any' ? 'any' : undefined } })
    if (!query) {
      reqs.value = tests.value = docs.value = []
      return
    }
    itemsLoading.value = true
    const pid = proj.pid
    const get = async <T,>(path: string) => {
      try {
        return asItems(await api.get<{ items: T[] }>(path, { q: query }))
      } catch {
        return [] as T[] // e.g. no permission for one kind
      }
    }
    const [r, ts, d] = await Promise.all([get<Requirement>(`/projects/${pid}/requirements`), get<TestCase>(`/projects/${pid}/tests`), get<FileInfo>(`/projects/${pid}/documents`)])
    reqs.value = r
    tests.value = ts
    docs.value = d
    itemsLoading.value = false
  },
  { immediate: true },
)
const sortOptions = computed(() => [
  { label: t('tagCloud.byUse'), value: 'count' },
  { label: t('tagCloud.byName'), value: 'name' },
])
const modeOptions = computed(() => [
  { label: t('tagCloud.all'), value: 'all' },
  { label: t('tagCloud.any'), value: 'any' },
])
const countText = (x: TagCount) =>
  t('tagCloud.counts', { requirements: x.requirements, tests: x.tests, documents: x.documents })
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('tagCloud.title') }}</h1>
      <div class="sub">{{ t('tagCloud.intro') }}</div>
    </div>
    <div class="actions"><HelpLink page="search-and-filters" :anchor="{ en: 'tag-cloud', sv: 'taggmoln' }" /></div>
  </div>

  <div class="card">
    <div class="filterbar">
      <InputText v-model="filter" :placeholder="t('tagCloud.filter')" :aria-label="t('tagCloud.filter')" style="min-width: 200px" />
      <SelectButton v-model="sortBy" :options="sortOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('tagCloud.sort')" />
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="loading && !tags.length" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <div v-else-if="!tags.length" class="empty"><i class="pi pi-hashtag" aria-hidden="true" /><div>{{ t('tagCloud.none') }}</div></div>
    <ul v-else class="tag-cloud plain-list" :aria-label="t('tagCloud.title')">
      <li v-for="x in shown" :key="x.tag">
        <button
          type="button"
          class="cloud-tag tag-colored"
          :class="[tagClass(x.tag), { on: isOn(x.tag) }]"
          :aria-pressed="isOn(x.tag)"
          :style="{ fontSize: cloudSize(x.total, minMax.min, minMax.max) + 'px' }"
          :title="countText(x)"
          @click="toggle(x.tag)"
        >
          {{ x.tag }}<span class="cloud-count">{{ x.total }}</span>
          <span class="sr-only">({{ countText(x) }})</span>
        </button>
      </li>
      <li v-if="!shown.length" class="muted small">{{ t('tagCloud.noMatch') }}</li>
    </ul>
  </div>

  <div v-if="selected.length" class="card" style="margin-top: 16px" aria-live="polite" :aria-busy="itemsLoading">
    <div class="row" style="justify-content: space-between; gap: 8px; flex-wrap: wrap">
      <div class="row" style="gap: 6px; flex-wrap: wrap">
        <b>{{ t('tagCloud.selected') }}</b>
        <span v-for="s in selected" :key="s" class="tag-chip" :class="tagClass(s)">
          <i class="pi pi-tag small muted" aria-hidden="true" />{{ s }}
          <button type="button" class="tag-remove" :aria-label="t('tagsInput.remove', { tag: s })" @click="toggle(s)"><i class="pi pi-times small" aria-hidden="true" /></button>
        </span>
      </div>
      <div class="row" style="gap: 6px">
        <SelectButton v-if="selected.length > 1" v-model="mode" :options="modeOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('tagCloud.mode')" />
        <Button :label="t('common.clear')" text severity="secondary" @click="selected = []" />
      </div>
    </div>
    <div class="tag-results">
      <section>
        <h2 class="plain-heading row" style="gap: 6px">
          <i class="pi pi-list-check" aria-hidden="true" />{{ t('nav.requirements') }} <span class="count">{{ reqs.length }}</span>
        </h2>
        <RouterLink v-if="reqs.length" :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { q } }" class="small">{{ t('tagCloud.openList') }}</RouterLink>
        <ul class="plain-list">
          <li v-for="r in reqs" :key="r.id" class="row nowrap result-row">
            <RouterLink :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: r.id } }" class="key">{{ r.key }}</RouterLink>
            <span class="grow ellipsis">{{ r.title }}</span>
            <StatusTag :value="r.status" />
          </li>
          <li v-if="!reqs.length && !itemsLoading" class="muted small">{{ t('tagCloud.noItems') }}</li>
        </ul>
      </section>
      <section>
        <h2 class="plain-heading row" style="gap: 6px">
          <i class="pi pi-check-square" aria-hidden="true" />{{ t('nav.tests') }} <span class="count">{{ tests.length }}</span>
        </h2>
        <RouterLink v-if="tests.length" :to="{ name: 'tests', params: { projectId: proj.pid }, query: { q } }" class="small">{{ t('tagCloud.openList') }}</RouterLink>
        <ul class="plain-list">
          <li v-for="x in tests" :key="x.id" class="row nowrap result-row">
            <RouterLink :to="{ name: 'tests', params: { projectId: proj.pid }, query: { item: x.id } }" class="key">{{ x.key }}</RouterLink>
            <span class="grow ellipsis">{{ x.title }}</span>
            <StatusTag :value="x.last_result" kind="result" />
          </li>
          <li v-if="!tests.length && !itemsLoading" class="muted small">{{ t('tagCloud.noItems') }}</li>
        </ul>
      </section>
      <section>
        <h2 class="plain-heading row" style="gap: 6px">
          <i class="pi pi-folder" aria-hidden="true" />{{ t('nav.documents') }} <span class="count">{{ docs.length }}</span>
        </h2>
        <ul class="plain-list">
          <li v-for="d in docs" :key="d.id" class="row nowrap result-row">
            <i class="pi pi-file muted" aria-hidden="true" />
            <a :href="contentUrl(d.id)" target="_blank" rel="noopener" class="grow ellipsis">{{ d.name }}</a>
          </li>
          <li v-if="!docs.length && !itemsLoading" class="muted small">{{ t('tagCloud.noItems') }}</li>
        </ul>
      </section>
    </div>
  </div>
  <p v-else-if="tags.length" class="small muted" style="margin-top: 12px">{{ t('tagCloud.pick') }}</p>
</template>

<style>
.tag-cloud {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 6px 14px;
  padding: 8px 0;
}
.cloud-tag {
  border: 1px solid transparent;
  border-radius: 999px;
  padding: 2px 10px;
  background: none;
  color: var(--p-primary-color);
  font-weight: 600;
  line-height: 1.3;
  cursor: pointer;
}
.cloud-tag:hover {
  background: var(--qtf-hover);
}
.cloud-tag.on {
  background: var(--p-primary-color);
  color: var(--p-primary-contrast-color);
}
.cloud-tag:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 2px;
}
.cloud-count {
  font-size: 11px;
  font-weight: 500;
  margin-left: 4px;
  opacity: 0.8;
  vertical-align: super;
}
.tag-results {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 18px;
  margin-top: 12px;
}
.result-row {
  padding: 4px 0;
  border-bottom: 1px solid var(--qtf-border);
}
</style>
