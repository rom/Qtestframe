<script setup lang="ts">
// Variants: the coverage matrix (requirements × variants, optionally in one
// test configuration) and the project's variants with their hierarchy,
// feature attributes and test configurations.
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import Tabs from 'primevue/tabs'
import TabList from 'primevue/tablist'
import Tab from 'primevue/tab'
import Textarea from 'primevue/textarea'
import ProgressSpinner from 'primevue/progressspinner'
import ItemRef from '@/components/ItemRef.vue'
import QueryInput from '@/components/QueryInput.vue'
import StatusTag from '@/components/StatusTag.vue'
import VariantSelect from '@/components/VariantSelect.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { useRoute, useRouter } from 'vue-router'
import { api, ApiError, errorMessage } from '@/lib/api'
import { P } from '@/lib/permissions'
import { attributeRows, attributeText, descendants, parseAttributes, variantDepth, variantPassed, VARIANT_KINDS, type Variant, type VariantMatrix } from '@/lib/variants'
import { useVariants } from '@/composables/variants'
import { useConfigurations } from '@/composables/configurations'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { uid } from '@/lib/a11y'
import { useI18n } from 'vue-i18n'

const proj = useProjectStore()
const auth = useAuthStore()
const notify = useNotify()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const id = uid('var')
const tabsId = uid('var-tabs')
/** tab panel attributes for the content of a tab */
const panel = (value: string) => ({ role: 'tabpanel', id: `${tabsId}_tabpanel_${value}`, 'aria-labelledby': `${tabsId}_tab_${value}` })
const pid = computed(() => proj.pid)
const vs = useVariants(() => pid.value)
const cfg = useConfigurations(() => pid.value)
const canEdit = computed(() => auth.can(P.projectUpdate))

const tab = ref<string>((route.query.tab as string) === 'manage' ? 'manage' : 'matrix')
watch(tab, (v) => router.replace({ query: { ...route.query, tab: v === 'matrix' ? undefined : v } }))

// ---------- matrix ----------
const matrix = ref<VariantMatrix | null>(null)
const loading = ref(false)
const q = ref('')
const queryError = ref<unknown>(null)
const configuration = ref<string | null>(null)
const configOptions = computed(() => [{ label: t('configurations.withoutShort'), value: 'none' }, ...cfg.options.value])

async function loadMatrix() {
  if (!pid.value) return
  loading.value = true
  queryError.value = null
  try {
    matrix.value = await api.get<VariantMatrix>(`/projects/${pid.value}/variants/matrix`, { q: q.value || undefined, configuration: configuration.value || undefined })
  } catch (e) {
    queryError.value = e
    if (!(e instanceof ApiError && e.code === 'invalid_query')) notify.fail(e)
  } finally {
    loading.value = false
  }
}
watch(configuration, loadMatrix)
const columns = computed(() => matrix.value?.variants ?? [])
function cellLabel(cov: string) {
  return cov === 'not_applicable' ? t('enums.coverage.not_applicable') : ''
}

// ---------- manage ----------
const dialog = ref(false)
const editing = ref<Variant | null>(null)
const form = ref({ name: '', parent_id: null as string | null, kind: '', description: '', attrs: [] as { name: string; value: string }[], configuration_ids: [] as string[] })
const formError = ref('')
const busy = ref(false)
const kindOptions = computed(() => [{ label: t('variants.kindNone'), value: '' }, ...VARIANT_KINDS.map((k) => ({ label: t(`variants.kind.${k}`), value: k }))])
// a variant cannot be moved below itself or one of its descendants
const parentChoices = computed(() => {
  if (!editing.value) return vs.variants.value
  const no = descendants(editing.value.id, vs.variants.value)
  return vs.variants.value.filter((v) => !no.has(v.id))
})

function openCreate(parent: Variant | null = null) {
  editing.value = null
  form.value = { name: '', parent_id: parent?.id ?? null, kind: '', description: '', attrs: [{ name: '', value: '' }], configuration_ids: [] }
  formError.value = ''
  dialog.value = true
}
function openEdit(v: Variant) {
  editing.value = v
  const rows = attributeRows(v.attributes)
  form.value = { name: v.name, parent_id: v.parent_id, kind: v.kind, description: v.description, attrs: rows.length ? rows : [{ name: '', value: '' }], configuration_ids: [...(v.configuration_ids ?? [])] }
  formError.value = ''
  dialog.value = true
}
async function save() {
  const { attributes, problems } = parseAttributes(form.value.attrs)
  if (problems.length) {
    formError.value = t(`variants.attrProblem.${problems[0]}`)
    return
  }
  busy.value = true
  formError.value = ''
  const body = { name: form.value.name, parent_id: form.value.parent_id ?? '', kind: form.value.kind, description: form.value.description, attributes, configuration_ids: form.value.configuration_ids }
  try {
    if (editing.value) {
      await api.patch(`/variants/${editing.value.id}`, body)
      notify.ok(t('variants.saved'))
    } else {
      const v = await api.post<Variant>(`/projects/${pid.value}/variants`, body)
      notify.ok(t('variants.created'), v?.name)
    }
    dialog.value = false
    await reload()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
async function remove(v: Variant) {
  if (!window.confirm(t('variants.confirmDelete', { name: v.name }))) return
  try {
    await api.del(`/variants/${v.id}`)
    notify.ok(t('variants.deleted'), v.name)
    await reload()
  } catch (e) {
    notify.fail(e)
  }
}
const cfgNames = (ids: string[]) => ids.map((i) => cfg.names.value.get(i) ?? '').filter(Boolean).join(', ')

async function reload() {
  await vs.load()
  await loadMatrix()
}
onMounted(async () => {
  await Promise.all([vs.load(), cfg.load()])
  await loadMatrix()
})
watch(pid, () => void reload())
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('variants.title') }}</h1>
      <div class="sub">{{ t('variants.subtitle', { n: vs.variants.value.length }, vs.variants.value.length) }}</div>
    </div>
    <div class="actions">
      <HelpLink page="variants-and-code" />
      <Button v-if="canEdit" :label="t('variants.new')" icon="pi pi-plus" aria-haspopup="dialog" @click="openCreate()" />
    </div>
  </div>

  <Tabs :id="tabsId" v-model:value="tab" class="editor-tabs">
    <TabList>
      <Tab value="matrix"><i class="pi pi-th-large" aria-hidden="true" /> {{ t('variants.matrix') }}</Tab>
      <Tab value="manage"><i class="pi pi-clone" aria-hidden="true" /> {{ t('variants.manage') }}</Tab>
    </TabList>
  </Tabs>

  <div v-if="vs.loaded.value && !vs.variants.value.length" v-bind="panel(tab)" class="card empty-state" style="margin-top: 12px">
    <p>{{ t('variants.empty') }}</p>
    <Button v-if="canEdit" :label="t('variants.new')" icon="pi pi-plus" @click="openCreate()" />
  </div>

  <section v-else-if="tab === 'matrix'" v-bind="panel('matrix')" style="margin-top: 12px">
    <h2 id="h-matrix" class="sr-only">{{ t('variants.matrix') }}</h2>
    <div class="filterbar">
      <QueryInput v-model="q" scope="requirement" :error="queryError" @submit="loadMatrix" />
    </div>
    <div class="filterbar">
      <label :id="`${id}-cfg-label`" :for="`${id}-cfg`" class="small">{{ t('variants.inConfiguration') }}</label>
      <Select v-model="configuration" :input-id="`${id}-cfg`" :aria-labelledby="`${id}-cfg-label`" :options="configOptions" option-label="label" option-value="value" :placeholder="t('variants.allConfigurations')" show-clear style="min-width: 200px" />
    </div>
    <div v-if="loading && !matrix" class="empty" aria-busy="true"><ProgressSpinner style="width: 36px; height: 36px" :aria-label="t('common.loading')" /></div>
    <div v-else-if="matrix" class="table-scroll card" style="padding: 0">
      <table class="matrix">
        <caption class="sr-only">{{ t('variants.matrixCaption') }}</caption>
        <thead>
          <tr>
            <th scope="col" class="sticky">{{ t('nav.requirements') }}</th>
            <th v-for="v in columns" :key="v.id" scope="col">
              <span :style="{ paddingInlineStart: `${variantDepth(v, columns) * 8}px` }">{{ v.name }}</span>
            </th>
          </tr>
          <tr class="summary">
            <th scope="row" class="sticky small muted">{{ t('variants.passedOfApplicable') }}</th>
            <td v-for="v in columns" :key="v.id" class="small">
              <template v-if="variantPassed(matrix.summary[v.id]).percent !== null">
                {{ variantPassed(matrix.summary[v.id]).passed }}/{{ variantPassed(matrix.summary[v.id]).applicable }} ({{ variantPassed(matrix.summary[v.id]).percent }} %)
              </template>
              <span v-else class="muted">—</span>
            </td>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in matrix.requirements" :key="r.id">
            <th scope="row" class="sticky">
              <ItemRef :to="{ name: 'requirements', params: { projectId: pid }, query: { item: r.id } }" :item-key="r.key" :title="r.title" title-class="small muted ellipsis" />
            </th>
            <td v-for="v in columns" :key="v.id">
              <span v-if="r.coverage[v.id] === 'not_applicable'" class="muted" :title="cellLabel(r.coverage[v.id])">—<span class="sr-only">{{ cellLabel(r.coverage[v.id]) }}</span></span>
              <StatusTag v-else :value="r.coverage[v.id]" kind="coverage" />
            </td>
          </tr>
          <tr v-if="!matrix.requirements.length">
            <td :colspan="columns.length + 1" class="muted small">{{ t('variants.noRequirements') }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <section v-else v-bind="panel('manage')" style="margin-top: 12px">
    <h2 id="h-manage" class="sr-only">{{ t('variants.manage') }}</h2>
    <ul class="plain-list variant-list card" style="padding: 0">
      <li v-for="v in vs.variants.value" :key="v.id" class="variant-row" :style="{ paddingInlineStart: `${12 + variantDepth(v, vs.variants.value) * 20}px` }">
        <div class="grow" style="min-width: 0">
          <div class="row nowrap">
            <i class="pi pi-clone muted" aria-hidden="true" />
            <b class="ellipsis">{{ v.name }}</b>
            <span v-if="v.kind" class="small muted">{{ t(`variants.kind.${v.kind}`) }}</span>
          </div>
          <div class="small muted wrap">
            <span v-if="v.description">{{ v.description }}</span>
            <span v-if="attributeText(v.attributes)"> · {{ attributeText(v.attributes) }}</span>
            <span v-if="v.configuration_ids?.length"> · {{ t('variants.testedIn', { list: cfgNames(v.configuration_ids) }) }}</span>
          </div>
        </div>
        <span v-if="canEdit" class="row nowrap">
          <Button icon="pi pi-plus" text rounded size="small" severity="secondary" :aria-label="t('variants.addChild', { name: v.name })" v-tooltip="t('variants.addChildHint')" @click="openCreate(v)" />
          <Button icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="t('variants.editNamed', { name: v.name })" @click="openEdit(v)" />
          <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('variants.deleteNamed', { name: v.name })" @click="remove(v)" />
        </span>
      </li>
    </ul>
  </section>

  <Dialog v-model:visible="dialog" modal :header="editing ? t('variants.editTitle') : t('variants.newTitle')" style="width: min(620px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="field">
        <label :for="`${id}-name`">{{ t('common.name') }}</label>
        <InputText :id="`${id}-name`" v-model="form.name" maxlength="200" autofocus required placeholder="EU model" />
      </div>
      <div class="form-grid">
        <div class="field">
          <label :id="`${id}-parent-label`" :for="`${id}-parent`">{{ t('variants.parent') }}</label>
          <VariantSelect v-model="form.parent_id" :input-id="`${id}-parent`" :labelledby="`${id}-parent-label`" :variants="parentChoices" :placeholder="t('variants.noParent')" />
        </div>
        <div class="field">
          <label :id="`${id}-kind-label`" :for="`${id}-kind`">{{ t('variants.kindLabel') }}</label>
          <Select v-model="form.kind" :input-id="`${id}-kind`" :aria-labelledby="`${id}-kind-label`" :options="kindOptions" option-label="label" option-value="value" />
        </div>
      </div>
      <div class="field">
        <label :for="`${id}-desc`">{{ t('common.description') }}</label>
        <Textarea :id="`${id}-desc`" v-model="form.description" rows="2" auto-resize maxlength="4000" />
      </div>
      <fieldset class="field attrs">
        <legend class="field-label">{{ t('variants.attributes') }}</legend>
        <div v-for="(a, i) in form.attrs" :key="i" class="row nowrap">
          <InputText v-model="a.name" :aria-label="t('variants.attrName', { n: i + 1 })" :placeholder="t('variants.attrNamePlaceholder')" maxlength="100" class="grow" />
          <InputText v-model="a.value" :aria-label="t('variants.attrValue', { n: i + 1 })" :placeholder="t('variants.attrValuePlaceholder')" maxlength="100" class="grow" />
          <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('variants.attrRemove', { n: i + 1 })" @click="form.attrs.splice(i, 1)" />
        </div>
        <Button :label="t('variants.attrAdd')" icon="pi pi-plus" size="small" text type="button" @click="form.attrs.push({ name: '', value: '' })" />
        <small class="muted">{{ t('variants.attributesHelp') }}</small>
      </fieldset>
      <div class="field">
        <label :for="`${id}-cfgs`">{{ t('variants.configurations') }}</label>
        <MultiSelect v-model="form.configuration_ids" :input-id="`${id}-cfgs`" :options="cfg.options.value" option-label="label" option-value="value" display="chip" filter :placeholder="t('variants.allConfigurations')" :aria-describedby="`${id}-cfgs-help`" />
        <small :id="`${id}-cfgs-help`" class="muted">{{ t('variants.configurationsHelp') }}</small>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" severity="secondary" text @click="dialog = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="busy" :disabled="!form.name.trim()" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.table-scroll {
  overflow-x: auto;
}
.matrix {
  border-collapse: collapse;
  width: 100%;
}
.matrix th,
.matrix td {
  text-align: left;
  padding: 6px 10px;
  border-bottom: 1px solid var(--qtf-border);
  white-space: nowrap;
}
.matrix thead th {
  font-size: 12px;
  font-weight: 600;
}
.matrix .sticky {
  position: sticky;
  inset-inline-start: 0;
  background: var(--qtf-card);
  max-width: 320px;
  z-index: 1;
}
.summary td,
.summary th {
  background: var(--qtf-bg);
}
.variant-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--qtf-border);
}
.variant-row:last-child {
  border-bottom: 0;
}
.wrap {
  white-space: normal;
}
.attrs {
  border: 0;
  padding: 0;
  margin: 0;
}
.empty-state {
  text-align: center;
  padding: 24px;
}
</style>
