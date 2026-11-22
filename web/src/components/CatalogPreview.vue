<script setup lang="ts">
// Full text of one catalogue requirement, test or risk in the chosen
// language, with the catalogue items at the other end of its links (tests of
// a requirement and the risks it mitigates, requirements of a test, the
// mitigating requirements of a risk).
import { computed, ref, watch } from 'vue'
import ProgressSpinner from 'primevue/progressspinner'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import StatusTag from './StatusTag.vue'
import ObligationText from './ObligationText.vue'
import { api, errorMessage } from '@/lib/api'
import { refLinks, type CatalogKind, type CatalogLinked, type CatalogRequirementDetail, type CatalogRiskDetail, type CatalogTestDetail } from '@/lib/catalog'
import { riskLevelSeverity } from '@/lib/risks'
import { enumLabel } from '@/i18n/labels'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ kind: CatalogKind; id: string; lang: string; projectId: string }>()
const emit = defineEmits<{ open: [kind: CatalogKind, id: string]; technique: [id: string]; framework: [kind: 'd3fend' | 'emb3d', id: string] }>()
const { t } = useI18n()

const req = ref<CatalogRequirementDetail | null>(null)
const test = ref<CatalogTestDetail | null>(null)
const risk = ref<CatalogRiskDetail | null>(null)
const loading = ref(false)
const error = ref('')

async function load() {
  error.value = ''
  if (!props.id) return
  loading.value = true
  const path = props.kind === 'requirement' ? 'requirements' : props.kind === 'risk' ? 'risks' : 'tests'
  try {
    const d = await api.get<CatalogRequirementDetail & CatalogTestDetail & CatalogRiskDetail>(`/catalog/${path}/${encodeURIComponent(props.id)}`, { lang: props.lang, project_id: props.projectId })
    req.value = props.kind === 'requirement' ? d : null
    test.value = props.kind === 'test' ? d : null
    risk.value = props.kind === 'risk' ? d : null
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => [props.kind, props.id, props.lang, props.projectId], load, { immediate: true })

const item = computed(() => req.value ?? test.value ?? risk.value)
const linked = computed<CatalogLinked[]>(() => req.value?.linked_tests ?? test.value?.linked_requirements ?? risk.value?.mitigating_requirements ?? [])
const otherKind = computed<CatalogKind>(() => (props.kind === 'requirement' ? 'test' : 'requirement'))
const linkedTitle = computed(() => (req.value ? t('catalog.verifyingTests') : risk.value ? t('catalog.risk.mitigatedBy') : t('catalog.verifiedRequirements')))
// the cited standards, linked to their text where the source registry knows it
const refs = computed(() => refLinks(item.value?.refs ?? [], item.value?.ref_targets))
const keys = (xs: { key: string }[] | undefined) => (xs ?? []).map((x) => x.key).join(', ')
// MITRE D3FEND and EMB3D
const d3fend = computed(() => req.value?.d3fend_techniques ?? test.value?.d3fend_techniques ?? [])
const emb3d = computed(() => item.value?.emb3d_items ?? [])
const defenses = computed(() => risk.value?.defenses ?? [])
const gaps = computed(() => defenses.value.filter((d) => !d.covered).length)
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
        <StatusTag v-if="item.priority" :value="item.priority" kind="priority" />
        <template v-if="risk">
          <Tag :value="t(`risksView.level.${risk.level}`)" :severity="riskLevelSeverity(risk.level)" />
          <Tag :value="risk.category_name" severity="secondary" />
          <span class="small muted">{{ t('catalog.risk.score', { likelihood: risk.likelihood, impact: risk.impact, score: risk.score }) }}</span>
        </template>
        <Tag v-if="req" :value="enumLabel('requirementType', req.type)" severity="secondary" />
        <Tag v-if="test" :value="enumLabel('executionType', test.execution)" severity="secondary" />
        <Tag v-for="tg in item.tags" :key="tg" :value="tg" severity="secondary" class="cp-tag" />
      </div>
      <Message v-if="item.in_project?.length" severity="info" :closable="false" class="small">{{ t('catalog.inProjectAs', { keys: keys(item.in_project) }) }}</Message>

      <h3>{{ risk ? t('catalog.risk.description') : t('catalog.description') }}</h3>
      <p v-if="risk" class="text">{{ risk.description }}</p>
      <p v-else class="text"><ObligationText :text="item.description" :lang="lang" /></p>

      <template v-if="risk">
        <h3>{{ t('catalog.risk.mitigation') }}</h3>
        <p class="text">{{ risk.mitigation }}</p>
      </template>

      <template v-if="risk?.techniques?.length">
        <h3 id="cp-attack">{{ t('attack.techniques') }}</h3>
        <ul class="cp-list" aria-labelledby="cp-attack">
          <li v-for="x in risk.techniques" :key="x.id">
            <a :href="x.url" target="_blank" rel="noopener noreferrer"
              ><span class="key">{{ x.id }}</span> {{ x.name }} <i class="pi pi-external-link cp-ext" aria-hidden="true" /><span class="sr-only"> ({{ t('links.newTab') }})</span></a
            >
            <div class="small muted">
              {{ t(`attack.matrix.${x.matrix}`) }} · {{ x.tactics.map((ta) => ta.name).join(', ') }} ·
              <button type="button" class="linklike cp-filter" @click="emit('technique', x.id)">{{ t('attack.showRisks') }}</button>
            </div>
          </li>
        </ul>
      </template>

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

      <template v-if="d3fend.length">
        <h3 id="cp-d3fend">{{ t('frameworks.d3fend.techniques') }}</h3>
        <ul class="cp-list" aria-labelledby="cp-d3fend">
          <li v-for="x in d3fend" :key="x.id">
            <a :href="x.url" target="_blank" rel="noopener noreferrer"
              ><span class="key">{{ x.id }}</span> {{ x.name }} <i class="pi pi-external-link cp-ext" aria-hidden="true" /><span class="sr-only"> ({{ t('links.newTab') }})</span></a
            >
            <div class="small muted">
              {{ x.tactic }}<template v-if="x.parent_name"> · {{ x.parent_name }}</template> ·
              <button type="button" class="linklike cp-filter" @click="emit('framework', 'd3fend', x.id)">{{ t('frameworks.showItems') }}</button>
            </div>
          </li>
        </ul>
      </template>

      <template v-if="emb3d.length">
        <h3 id="cp-emb3d">{{ risk ? t('frameworks.threats.label') : t('frameworks.emb3d.label') }}</h3>
        <ul class="cp-list" aria-labelledby="cp-emb3d">
          <li v-for="x in emb3d" :key="x.id">
            <a :href="x.url" target="_blank" rel="noopener noreferrer"
              ><span class="key">{{ x.id }}</span> {{ x.name }} <i class="pi pi-external-link cp-ext" aria-hidden="true" /><span class="sr-only"> ({{ t('links.newTab') }})</span></a
            >
            <div class="small muted">
              {{ x.kind === 'threat' ? t('frameworks.emb3d.threat') + ' · ' + x.category : t('frameworks.emb3d.mitigation') }} ·
              <button type="button" class="linklike cp-filter" @click="emit('framework', 'emb3d', x.id)">{{ t('frameworks.showItems') }}</button>
            </div>
          </li>
        </ul>
      </template>

      <template v-if="defenses.length">
        <h3 id="cp-defenses">{{ t('frameworks.defenses.title') }}</h3>
        <p class="small muted">{{ gaps ? t('frameworks.defenses.gaps', { n: gaps }, gaps) : t('frameworks.defenses.allCovered') }}</p>
        <ul class="cp-list cp-defenses" aria-labelledby="cp-defenses">
          <li v-for="d in defenses" :key="d.technique.id">
            <span class="key">{{ d.technique.id }}</span> {{ d.technique.name }}
            <Tag :value="d.covered ? t('frameworks.defenses.covered') : t('frameworks.defenses.gap')" :severity="d.covered ? 'success' : 'warn'" class="small" />
            <div v-if="d.d3fend.length" class="small muted">{{ t('frameworks.defenses.counteredBy', { list: d.d3fend.map((x) => x.id).join(', ') }) }}</div>
            <div v-else class="small muted">{{ t('frameworks.defenses.noCounter') }}</div>
            <ul v-if="d.requirements.length" class="cp-list">
              <li v-for="r in d.requirements" :key="r.id">
                <button type="button" class="linklike" @click="emit('open', 'requirement', r.id)">
                  <span class="key">{{ r.id }}</span> <span class="cp-link-title">{{ r.title }}</span>
                </button>
                <span class="small muted"> · {{ r.d3fend.join(', ') }}<template v-if="r.mitigates"> · {{ t('frameworks.defenses.mitigating') }}</template></span>
              </li>
            </ul>
          </li>
        </ul>
      </template>

      <template v-if="item.refs.length">
        <h3>{{ t('catalog.refs') }}</h3>
        <ul class="cp-list">
          <li v-for="r in refs" :key="r.ref">
            <a v-if="r.url" :href="r.url" target="_blank" rel="noopener noreferrer"
              >{{ r.ref }} <i class="pi pi-external-link cp-ext" aria-hidden="true" /><span class="sr-only"> ({{ t('links.newTab') }})</span></a
            >
            <template v-else>{{ r.ref }}</template>
          </li>
        </ul>
        <p class="small muted">{{ t('catalog.refsHint') }}</p>
      </template>

      <template v-if="linked.length">
        <h3>{{ linkedTitle }}</h3>
        <ul class="cp-list">
          <li v-for="l in linked" :key="l.id">
            <button type="button" class="linklike" @click="emit('open', otherKind, l.id)">
              <span class="key">{{ l.id }}</span> <span class="cp-link-title">{{ l.title }}</span>
            </button>
            <Tag v-if="l.in_project?.length" :value="t('catalog.inProject')" severity="success" class="small" />
          </li>
        </ul>
      </template>
      <template v-if="risk?.linked_tests.length">
        <h3>{{ t('catalog.risk.tests') }}</h3>
        <ul class="cp-list">
          <li v-for="l in risk.linked_tests" :key="l.id">
            <button type="button" class="linklike" @click="emit('open', 'test', l.id)">
              <span class="key">{{ l.id }}</span> <span class="cp-link-title">{{ l.title }}</span>
            </button>
            <Tag v-if="l.in_project?.length" :value="t('catalog.inProject')" severity="success" class="small" />
          </li>
        </ul>
      </template>
      <template v-if="req?.mitigates_risks?.length">
        <h3>{{ t('catalog.risk.mitigates') }}</h3>
        <ul class="cp-list">
          <li v-for="l in req.mitigates_risks" :key="l.id">
            <button type="button" class="linklike" @click="emit('open', 'risk', l.id)">
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
.cp-ext { font-size: 0.75em; }
.cp-filter { text-decoration: underline; }
</style>
