<script setup lang="ts">
// Verification of the project: the requirement hierarchy (refines) with
// roll-up badges, the system elements with coverage and verification per
// element, and the inconsistencies of statuses with the evidence.
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import ItemRef from '@/components/ItemRef.vue'
import StatusTag from '@/components/StatusTag.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api, asItems, errorMessage } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { P } from '@/lib/permissions'
import { ELEMENT_KINDS, VERIFICATION_STATES, buildHierarchy, elementPercent, filterHierarchy, flattenHierarchy, reasonText, subtreeIds } from '@/lib/verification'
import type { ElementCoverage, ProjectVerification, RequirementVerification, SystemElement } from '@/lib/types'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useEscapeClose } from '@/composables/escapeClose'

const proj = useProjectStore()
const auth = useAuthStore()
const notify = useNotify()
const route = useRoute()
const router = useRouter()
const { t, te } = useI18n()
const id = uid('ver')

type Tab = 'hierarchy' | 'elements' | 'inconsistent'
const isTab = (v: unknown): v is Tab => v === 'hierarchy' || v === 'elements' || v === 'inconsistent'
const tab = ref<Tab>(isTab(route.query.tab) ? route.query.tab : 'hierarchy')
watch(tab, (v) => void router.replace({ query: { ...route.query, tab: v === 'hierarchy' ? undefined : v } }))
const tabOptions = computed(() => [
  { value: 'hierarchy', label: t('verificationView.hierarchy'), icon: 'pi pi-sitemap' },
  { value: 'elements', label: t('verificationView.elements'), icon: 'pi pi-box' },
  { value: 'inconsistent', label: t('verificationView.inconsistent'), icon: 'pi pi-exclamation-triangle' },
])

const data = ref<ProjectVerification | null>(null)
const coverage = ref<ElementCoverage[]>([])
const elements = ref<SystemElement[]>([])
const loading = ref(false)
const canWrite = computed(() => auth.can(P.requirementWrite))

async function load() {
  if (!proj.pid) return
  loading.value = true
  try {
    const [v, cov, els] = await Promise.all([
      api.get<ProjectVerification>(`/projects/${proj.pid}/verification`),
      api.get<{ items: ElementCoverage[] }>(`/projects/${proj.pid}/elements/coverage`),
      api.get<{ items: SystemElement[] }>(`/projects/${proj.pid}/elements`),
    ])
    data.value = v
    coverage.value = asItems(cov)
    elements.value = asItems(els)
  } catch (e) {
    notify.fail(e, t('verificationView.loadFailed'))
  } finally {
    loading.value = false
  }
}
onMounted(load)
watch(() => proj.pid, load)

const total = computed(() => VERIFICATION_STATES.reduce((n, s) => n + (data.value?.counts[s] ?? 0), 0))

// ---------- hierarchy ----------
const filter = ref('')
const onlyOpen = ref(false)
const collapsed = ref(new Set<string>())
const tree = computed(() => {
  let nodes = buildHierarchy(data.value?.requirements ?? [])
  const f = filter.value.trim().toLowerCase()
  if (f || onlyOpen.value)
    nodes = filterHierarchy(nodes, (r) => (!onlyOpen.value || !r.verified) && (!f || `${r.key} ${r.title}`.toLowerCase().includes(f)))
  return nodes
})
const rows = computed(() => flattenHierarchy(tree.value, collapsed.value, (r) => r.requirement_id))
function toggle(rid: string) {
  const s = new Set(collapsed.value)
  if (s.has(rid)) s.delete(rid)
  else s.add(rid)
  collapsed.value = s
}
function activeChildren(r: RequirementVerification) {
  return r.children.filter((c) => c.status !== 'rejected' && c.status !== 'obsolete').length
}

// ---------- elements ----------
const elementById = computed(() => new Map(elements.value.map((e) => [e.id, e])))
const elementQuery = (c: ElementCoverage) => `element:"${c.path}"`
const dialog = ref(false)
useEscapeClose(dialog)
const form = ref({ id: '', parent_id: '' as string, name: '', kind: 'component', description: '' })
const formError = ref('')
const saving = ref(false)
const kindOptions = computed(() => ELEMENT_KINDS.map((k) => ({ label: t(`verificationView.kind.${k}`), value: k })))
const parentOptions = computed(() => {
  const exclude = form.value.id ? subtreeIds(elements.value, form.value.id) : new Set<string>()
  return [{ label: t('verificationView.noParent'), value: '' }, ...elements.value.filter((e) => !exclude.has(e.id)).map((e) => ({ label: e.path || e.name, value: e.id }))]
})
function openElement(e: SystemElement | null, parent?: string) {
  formError.value = ''
  form.value = e
    ? { id: e.id, parent_id: e.parent_id ?? '', name: e.name, kind: e.kind, description: e.description }
    : { id: '', parent_id: parent ?? '', name: '', kind: parent ? 'component' : 'system', description: '' }
  dialog.value = true
}
async function saveElement() {
  saving.value = true
  formError.value = ''
  const body = { parent_id: form.value.parent_id || null, name: form.value.name.trim(), kind: form.value.kind, description: form.value.description }
  try {
    if (form.value.id) await api.patch(`/elements/${form.value.id}`, body)
    else await api.post(`/projects/${proj.pid}/elements`, body)
    dialog.value = false
    await load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}
function deleteElement(c: ElementCoverage) {
  notify.confirmDanger(t('verificationView.deleteConfirm', { name: c.path }), async () => {
    try {
      await api.del(`/elements/${c.element_id}`)
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('verificationView.title') }}</h1>
      <div v-if="data" class="sub">
        {{ t('verificationView.summary', { n: data.counts.verified ?? 0, total }) }}
        <template v-if="data.criteria_total"> · {{ t('verification.criteriaVerified', { n: data.criteria_verified, total: data.criteria_total }) }}</template>
        <template v-if="data.strict"> · {{ t('verification.strict') }}</template>
      </div>
    </div>
    <div class="actions">
      <SelectButton v-model="tab" :options="tabOptions" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('verificationView.view')">
        <template #option="{ option }"><i :class="option.icon" aria-hidden="true" /><span>{{ option.label }}</span></template>
      </SelectButton>
      <HelpLink page="traceability-and-coverage" :anchor="{ en: 'verification', sv: 'verifiering' }" />
      <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" @click="load" />
    </div>
  </div>

  <div v-if="loading && !data" class="empty" aria-busy="true"><ProgressSpinner style="width: 36px; height: 36px" :aria-label="t('common.loading')" /></div>
  <template v-else-if="data">
    <ul class="plain-list ver-counts" :aria-label="t('verificationView.counts')">
      <li v-for="s in VERIFICATION_STATES" :key="s" class="card stat">
        <StatusTag :value="s" kind="verification" />
        <span class="value">{{ data.counts[s] ?? 0 }}</span>
      </li>
    </ul>

    <!-- hierarchy -->
    <section v-if="tab === 'hierarchy'" class="card" :aria-busy="loading">
      <h2 class="plain-heading">{{ t('verificationView.hierarchy') }}</h2>
      <p class="small muted" style="margin-top: 0">{{ t('verificationView.hierarchyHint') }}</p>
      <div class="filterbar">
        <IconField>
          <InputIcon class="pi pi-filter" aria-hidden="true" />
          <InputText v-model="filter" :placeholder="t('verificationView.filter')" :aria-label="t('verificationView.filter')" />
        </IconField>
        <span class="row small nowrap"><ToggleSwitch v-model="onlyOpen" :input-id="`${id}-open`" /><label :for="`${id}-open`">{{ t('verificationView.onlyOpen') }}</label></span>
      </div>
      <p v-if="!rows.length" class="muted">{{ t('verificationView.noRequirements') }}</p>
      <ul class="plain-list tree" :aria-label="t('verificationView.hierarchy')">
        <li v-for="n in rows" :key="n.item.requirement_id + ':' + n.depth" class="tree-row" :style="{ paddingLeft: `${n.depth * 20 + 4}px` }">
          <span v-if="n.depth" class="sr-only">{{ t('verificationView.level', { n: n.depth + 1 }) }}</span>
          <Button
            v-if="n.children.length"
            :icon="collapsed.has(n.item.requirement_id) ? 'pi pi-chevron-right' : 'pi pi-chevron-down'"
            text
            rounded
            size="small"
            severity="secondary"
            :aria-expanded="!collapsed.has(n.item.requirement_id)"
            :aria-label="t('verificationView.children', { key: n.item.key })"
            @click="toggle(n.item.requirement_id)"
          />
          <span v-else class="tree-spacer" aria-hidden="true" />
          <ItemRef :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: n.item.requirement_id } }" :item-key="n.item.key" :title="n.item.title" title-class="ellipsis" grow />
          <span class="tree-badges">
            <span v-if="n.item.criteria_total" class="small muted nowrap">{{ t('verification.criteriaShort', { n: n.item.criteria_verified, total: n.item.criteria_total }) }}</span>
            <Tag v-if="n.item.children.length" severity="secondary" class="nowrap" :value="t('verification.childrenShort', { n: n.item.children_verified, total: activeChildren(n.item) })" />
            <StatusTag :value="n.item.status" kind="status" />
            <StatusTag :value="n.item.state" kind="verification" />
            <i v-if="n.item.inconsistencies.length" class="pi pi-exclamation-triangle warn-text" :title="t('verification.inconsistent')" aria-hidden="true" />
            <span v-if="n.item.inconsistencies.length" class="sr-only">{{ t('verification.inconsistent') }}</span>
          </span>
        </li>
      </ul>
    </section>

    <!-- elements -->
    <section v-else-if="tab === 'elements'" class="card" :aria-busy="loading">
      <div class="row" style="justify-content: space-between; flex-wrap: wrap; gap: 8px">
        <h2 class="plain-heading">{{ t('verificationView.elements') }}</h2>
        <Button v-if="canWrite" size="small" icon="pi pi-plus" :label="t('verificationView.addElement')" @click="openElement(null)" />
      </div>
      <p class="small muted" style="margin-top: 0">{{ t('verificationView.elementsHint') }}</p>
      <p v-if="!coverage.length" class="muted">{{ t('verificationView.noElements') }}</p>
      <ul class="plain-list tree" :aria-label="t('verificationView.elements')">
        <li v-for="c in coverage" :key="c.element_id" class="tree-row elem-row" :style="{ paddingLeft: `${c.depth * 20 + 4}px` }">
          <span v-if="c.depth" class="sr-only">{{ t('verificationView.level', { n: c.depth + 1 }) }}</span>
          <div class="grow elem-main">
            <div class="row nowrap" style="gap: 6px; min-width: 0">
              <i class="pi pi-box" aria-hidden="true" />
              <b class="ellipsis">{{ c.name }}</b>
              <Tag :value="t(`verificationView.kind.${c.kind}`)" severity="secondary" />
            </div>
            <div v-if="elementById.get(c.element_id)?.description" class="small muted ellipsis">{{ elementById.get(c.element_id)?.description }}</div>
          </div>
          <div class="elem-stats">
            <RouterLink :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { q: elementQuery(c) } }" class="small nowrap">
              {{ t('verificationView.verifiedOf', { n: c.verified, total: c.requirements }) }}
            </RouterLink>
            <div class="meter" role="img" :aria-label="t('verificationView.meter', { pct: elementPercent(c) })">
              <span :style="{ width: elementPercent(c) + '%' }" />
            </div>
            <span class="small muted nowrap">{{ t('verificationView.withEvidence', { n: c.covered }) }}<template v-if="c.failed"> · {{ t('verificationView.failedN', { n: c.failed }) }}</template></span>
            <RouterLink :to="{ name: 'tests', params: { projectId: proj.pid }, query: { q: elementQuery(c) } }" class="small muted nowrap">{{ t('verificationView.testsPassed', { n: c.tests_passed, total: c.tests }) }}</RouterLink>
          </div>
          <div v-if="canWrite" class="row nowrap" style="gap: 2px">
            <Button icon="pi pi-plus" text rounded size="small" severity="secondary" :aria-label="t('verificationView.addChild', { name: c.name })" v-tooltip="t('verificationView.addChild', { name: c.name })" @click="openElement(null, c.element_id)" />
            <Button icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="t('verificationView.editElement', { name: c.name })" v-tooltip="t('common.edit')" @click="openElement(elementById.get(c.element_id) ?? null)" />
            <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('verificationView.deleteElement', { name: c.name })" v-tooltip="t('common.delete')" @click="deleteElement(c)" />
          </div>
        </li>
      </ul>
    </section>

    <!-- inconsistencies -->
    <section v-else class="card" :aria-busy="loading">
      <h2 class="plain-heading">{{ t('verificationView.inconsistent') }}</h2>
      <p class="small muted" style="margin-top: 0">{{ t('verificationView.inconsistentHint') }}</p>
      <p v-if="!data.inconsistent.length" class="muted"><i class="pi pi-check-circle" aria-hidden="true" /> {{ t('verificationView.noInconsistencies') }}</p>
      <ul class="plain-list">
        <li v-for="r in data.inconsistent" :key="r.requirement_id" class="tree-row" style="align-items: flex-start">
          <div class="grow" style="min-width: 0">
            <ItemRef :to="{ name: 'requirements', params: { projectId: proj.pid }, query: { item: r.requirement_id } }" :item-key="r.key" :title="r.title" title-class="ellipsis" />
            <ul class="small" style="margin: 4px 0 0; padding-left: 18px">
              <li v-for="(x, i) in r.inconsistencies" :key="i">{{ reasonText(t, te, x) }}</li>
            </ul>
          </div>
          <StatusTag :value="r.status" kind="status" />
          <StatusTag :value="r.state" kind="verification" />
        </li>
      </ul>
    </section>
  </template>

  <Dialog v-model:visible="dialog" :close-on-escape="false" modal :header="form.id ? t('verificationView.editElementTitle') : t('verificationView.addElement')" :style="{ width: 'min(520px, calc(100vw - 16px))' }" :breakpoints="{ '640px': '100vw' }">
    <form class="grid" style="gap: 12px" @submit.prevent="saveElement">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="field">
        <label :for="`${id}-name`">{{ t('common.name') }}</label>
        <InputText :id="`${id}-name`" v-model="form.name" maxlength="200" required autofocus />
      </div>
      <div class="field">
        <label :for="`${id}-kind`">{{ t('verificationView.kindLabel') }}</label>
        <Select v-model="form.kind" :input-id="`${id}-kind`" :options="kindOptions" option-label="label" option-value="value" />
      </div>
      <div class="field">
        <label :for="`${id}-parent`">{{ t('verificationView.parent') }}</label>
        <Select v-model="form.parent_id" :input-id="`${id}-parent`" :options="parentOptions" option-label="label" option-value="value" filter />
      </div>
      <div class="field">
        <label :for="`${id}-desc`">{{ t('common.description') }}</label>
        <Textarea :id="`${id}-desc`" v-model="form.description" rows="3" auto-resize maxlength="10000" />
      </div>
      <div class="row" style="justify-content: flex-end; gap: 8px">
        <Button :label="t('common.cancel')" severity="secondary" text @click="dialog = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="saving" :disabled="!form.name.trim()" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.ver-counts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 10px;
  margin-bottom: 16px;
}
.ver-counts .stat {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 12px;
}
.ver-counts .value {
  font-size: 20px;
  font-weight: 600;
}
.tree {
  margin-top: 8px;
}
.tree-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 4px;
  border-bottom: 1px solid var(--qtf-border);
  min-width: 0;
}
.tree-spacer {
  display: inline-block;
  width: 32px;
  flex: none;
}
.tree-badges {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  justify-content: flex-end;
}
.elem-main {
  min-width: 0;
}
.elem-stats {
  display: grid;
  grid-template-columns: auto 120px;
  gap: 2px 10px;
  align-items: center;
}
.meter {
  height: 8px;
  border-radius: 4px;
  background: var(--qtf-border);
  overflow: hidden;
}
.meter span {
  display: block;
  height: 100%;
  background: var(--p-green-500, #22c55e);
}
@media (max-width: 640px) {
  .tree-row {
    flex-wrap: wrap;
  }
  .tree-badges {
    width: 100%;
    justify-content: flex-start;
    padding-left: 40px;
  }
  .elem-stats {
    width: 100%;
    grid-template-columns: 1fr;
  }
}
</style>
