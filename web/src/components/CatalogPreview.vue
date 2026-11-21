<script setup lang="ts">
// Full text of one catalogue requirement or test in the chosen language,
// with the catalogue items at the other end of its trace links.
import { computed, ref, watch } from 'vue'
import ProgressSpinner from 'primevue/progressspinner'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import StatusTag from './StatusTag.vue'
import { api, errorMessage } from '@/lib/api'
import { keywordSegments, type CatalogKind, type CatalogLinked, type CatalogRequirementDetail, type CatalogTestDetail } from '@/lib/catalog'
import { enumLabel } from '@/i18n/labels'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ kind: CatalogKind; id: string; lang: string; projectId: string }>()
const emit = defineEmits<{ open: [kind: CatalogKind, id: string] }>()
const { t } = useI18n()

const req = ref<CatalogRequirementDetail | null>(null)
const test = ref<CatalogTestDetail | null>(null)
const loading = ref(false)
const error = ref('')

async function load() {
  error.value = ''
  if (!props.id) return
  loading.value = true
  const path = props.kind === 'requirement' ? 'requirements' : 'tests'
  try {
    const d = await api.get<CatalogRequirementDetail & CatalogTestDetail>(`/catalog/${path}/${encodeURIComponent(props.id)}`, { lang: props.lang, project_id: props.projectId })
    if (props.kind === 'requirement') {
      req.value = d
      test.value = null
    } else {
      test.value = d
      req.value = null
    }
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => [props.kind, props.id, props.lang, props.projectId], load, { immediate: true })

const item = computed(() => req.value ?? test.value)
const linked = computed<CatalogLinked[]>(() => req.value?.linked_tests ?? test.value?.linked_requirements ?? [])
const otherKind = computed<CatalogKind>(() => (props.kind === 'requirement' ? 'test' : 'requirement'))
const segments = computed(() => keywordSegments(item.value?.description ?? ''))
const keys = (xs: { key: string }[] | undefined) => (xs ?? []).map((x) => x.key).join(', ')
</script>

<template>
  <div class="catalog-preview" :aria-busy="loading">
    <div v-if="loading && !item" class="empty"><ProgressSpinner style="width: 32px; height: 32px" :aria-label="t('common.loading')" /></div>
    <Message v-else-if="error" severity="error" :closable="false">{{ error }}</Message>
    <article v-else-if="item" :aria-labelledby="'cp-title'">
      <div class="row small" style="gap: 6px; flex-wrap: wrap">
        <span class="key">{{ item.id }}</span>
        <span class="muted">{{ item.area_name }}</span>
      </div>
      <h2 id="cp-title" class="cp-title">{{ item.title }}</h2>
      <div class="row" style="gap: 6px; flex-wrap: wrap; margin-bottom: 8px">
        <StatusTag v-if="req" :value="req.obligation" kind="obligation" />
        <StatusTag :value="item.priority" kind="priority" />
        <Tag v-if="req" :value="enumLabel('requirementType', req.type)" severity="secondary" />
        <Tag v-if="test" :value="enumLabel('executionType', test.execution)" severity="secondary" />
        <Tag v-for="tg in item.tags" :key="tg" :value="tg" severity="secondary" class="cp-tag" />
      </div>
      <Message v-if="item.in_project?.length" severity="info" :closable="false" class="small">{{ t('catalog.inProjectAs', { keys: keys(item.in_project) }) }}</Message>

      <h3>{{ t('catalog.description') }}</h3>
      <p class="text">
        <template v-for="(s, i) in segments" :key="i">
          <span v-if="s.mark" :class="['kw', s.kind]">{{ s.text }}</span>
          <template v-else>{{ s.text }}</template>
        </template>
      </p>

      <template v-if="req">
        <h3>{{ t('catalog.rationale') }}</h3>
        <p class="text">{{ req.rationale }}</p>
        <h3>{{ t('catalog.criteria') }}</h3>
        <ul class="cp-list">
          <li v-for="(c, i) in req.criteria" :key="i">
            <Tag :value="enumLabel('evaluationMethod', c.method)" severity="secondary" /> {{ c.text }}
            <span v-if="c.threshold" class="muted small"> · {{ t('catalog.threshold', { value: c.threshold }) }}</span>
          </li>
        </ul>
      </template>

      <template v-if="test">
        <template v-if="test.preconditions">
          <h3>{{ t('catalog.preconditions') }}</h3>
          <p class="text">{{ test.preconditions }}</p>
        </template>
        <h3>{{ t('catalog.stepList') }}</h3>
        <ol class="cp-steps">
          <li v-for="(s, i) in test.step_list" :key="i">
            <div>{{ s.action }}</div>
            <div class="muted small">→ {{ s.expected }}</div>
            <div v-if="s.data" class="small mono">{{ t('catalog.stepData', { data: s.data }) }}</div>
          </li>
        </ol>
        <h3>{{ t('catalog.expected') }}</h3>
        <p class="text">{{ test.expected }}</p>
      </template>

      <template v-if="item.refs.length">
        <h3>{{ t('catalog.refs') }}</h3>
        <ul class="cp-list">
          <li v-for="r in item.refs" :key="r">{{ r }}</li>
        </ul>
        <p class="small muted">{{ t('catalog.refsHint') }}</p>
      </template>

      <template v-if="linked.length">
        <h3>{{ req ? t('catalog.verifyingTests') : t('catalog.verifiedRequirements') }}</h3>
        <ul class="cp-list">
          <li v-for="l in linked" :key="l.id">
            <button type="button" class="linklike" @click="emit('open', otherKind, l.id)">
              <span class="key">{{ l.id }}</span> <span class="cp-link-title">{{ l.title }}</span>
            </button>
            <Tag v-if="l.in_project?.length" :value="t('catalog.inProject')" severity="success" class="small" />
          </li>
        </ul>
      </template>
    </article>
  </div>
</template>

<style scoped>
.cp-title { font-size: 17px; margin: 4px 0 8px; font-weight: 650; overflow-wrap: anywhere; }
h3 { font-size: 13px; text-transform: uppercase; letter-spacing: 0.03em; color: var(--qtf-muted); margin: 14px 0 4px; }
.text { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
.cp-list { margin: 0; padding-left: 18px; }
.cp-list li { margin: 3px 0; overflow-wrap: anywhere; }
.cp-steps { margin: 0; padding-left: 20px; }
.cp-steps li { margin: 6px 0; overflow-wrap: anywhere; }
.linklike { background: none; border: 0; padding: 0; cursor: pointer; color: inherit; font: inherit; }
.linklike { text-align: left; }
.linklike:hover .key, .linklike:focus-visible .key, .linklike:hover .cp-link-title, .linklike:focus-visible .cp-link-title { text-decoration: underline; }
.cp-tag { font-weight: 500; }
</style>
