<script setup lang="ts">
// Test configurations of a project (Settings → Configurations): named
// combinations of dimension values such as Browser = Chrome, OS = Windows.
import { computed, onMounted, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import HelpLink from '@/guide/HelpLink.vue'
import { api, errorMessage } from '@/lib/api'
import { combinations, configurationName, dimensionsOf, existingNames, splitValues } from '@/lib/configurations'
import { entry, recordUndo } from '@/lib/undoActions'
import type { Configuration, DimensionValue } from '@/lib/types'
import { useConfigurations } from '@/composables/configurations'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string; canEdit: boolean }>()
const notify = useNotify()
const { t } = useI18n()
const cfg = useConfigurations(() => props.projectId)
const list = cfg.configurations
const loading = ref(false)
async function load() {
  loading.value = true
  await cfg.load()
  loading.value = false
}
onMounted(load)

const base = computed(() => `/projects/${props.projectId}/configurations`)
const selection = ref<Configuration[]>([])

// ---------- create / edit ----------
const dialog = ref(false)
const editing = ref<Configuration | null>(null)
const form = ref({ name: '', description: '', values: [] as DimensionValue[] })
const formError = ref('')
const busy = ref(false)
const knownDimensions = computed(() => dimensionsOf(list.value).map((d) => d.dimension))
function openCreate() {
  editing.value = null
  const dims = knownDimensions.value.length ? knownDimensions.value : ['']
  form.value = { name: '', description: '', values: dims.map((d) => ({ dimension: d, value: '' })) }
  formError.value = ''
  dialog.value = true
}
function openEdit(c: Configuration) {
  editing.value = c
  form.value = { name: c.name, description: c.description, values: (c.values ?? []).map((v) => ({ ...v })) }
  formError.value = ''
  dialog.value = true
}
const filledValues = () => form.value.values.filter((v) => v.dimension.trim() || v.value.trim())
const derivedName = computed(() => configurationName(filledValues()))
async function save() {
  busy.value = true
  formError.value = ''
  const body = { name: form.value.name.trim(), description: form.value.description, values: filledValues() }
  try {
    if (editing.value) {
      const before = { name: editing.value.name, description: editing.value.description, values: editing.value.values }
      const id = editing.value.id
      await api.patch(`/configurations/${id}`, body)
      recordUndo(
        entry(
          t('configurations.undoEdit', { name: before.name }),
          async () => {
            await api.patch(`/configurations/${id}`, before)
            await cfg.load()
          },
          async () => {
            await api.patch(`/configurations/${id}`, body)
            await cfg.load()
          },
        ),
      )
      notify.ok(t('configurations.saved'))
    } else {
      const c = await api.post<Configuration>(base.value, body)
      recordCreated(c ? [c] : [])
      notify.ok(t('configurations.created'), c?.name)
    }
    dialog.value = false
    await cfg.load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
/** undo deletes what was just created (again), redo creates it anew */
function recordCreated(created: Configuration[]) {
  if (!created.length) return
  let ids = created.map((c) => c.id)
  const bodies = created.map((c) => ({ name: c.name, description: c.description, values: c.values }))
  recordUndo(
    entry(
      t('configurations.undoCreate', { n: created.length }, created.length),
      async () => {
        await api.del(`${base.value}/bulk`, { ids })
        await cfg.load()
      },
      async () => {
        const res = await api.post<{ created: Configuration[] }>(`${base.value}/bulk`, { configurations: bodies })
        ids = (res?.created ?? []).map((c) => c.id)
        await cfg.load()
      },
    ),
  )
}

// ---------- generate combinations ----------
const genDialog = ref(false)
const genDims = ref<{ dimension: string; values: string }[]>([])
const skipExisting = ref(true)
function openGenerate() {
  const dims = dimensionsOf(list.value)
  genDims.value = dims.length
    ? dims.map((d) => ({ dimension: d.dimension, values: d.values.join(', ') }))
    : [
        { dimension: '', values: '' },
        { dimension: '', values: '' },
      ]
  skipExisting.value = true
  formError.value = ''
  genDialog.value = true
}
const combos = computed(() => combinations(genDims.value.map((d) => ({ dimension: d.dimension, values: splitValues(d.values) }))))
const comboNames = computed(() => combos.value.map(configurationName))
const comboExisting = computed(() => existingNames(list.value, comboNames.value))
const MAX_COMBOS = 200
async function generate() {
  busy.value = true
  formError.value = ''
  try {
    const res = await api.post<{ created: Configuration[]; skipped: string[] }>(`${base.value}/bulk`, {
      skip_existing: skipExisting.value,
      configurations: combos.value.map((values) => ({ values })),
    })
    const n = res?.created?.length ?? 0
    recordCreated(res?.created ?? [])
    notify.ok(t('configurations.generated', { n }, n), res?.skipped?.length ? t('configurations.skipped', { n: res.skipped.length }, res.skipped.length) : undefined)
    genDialog.value = false
    await cfg.load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}

// ---------- delete ----------
function remove(c: Configuration) {
  notify.confirmDanger(t('configurations.confirmDelete', { name: c.name }), async () => {
    try {
      await api.del(`/configurations/${c.id}`)
      notify.ok(t('configurations.deleted', { n: 1 }, 1))
      await cfg.load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
function bulkDelete() {
  const ids = selection.value.map((c) => c.id)
  notify.confirmDanger(t('configurations.confirmBulkDelete', { n: ids.length }, ids.length), async () => {
    try {
      await api.del(`${base.value}/bulk`, { ids })
      notify.ok(t('configurations.deleted', { n: ids.length }, ids.length))
      selection.value = []
      await cfg.load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
async function move(c: Configuration, d: number) {
  const i = list.value.indexOf(c)
  const other = list.value[i + d]
  if (!other) return
  try {
    // positions follow the list order
    const order = [...list.value]
    order.splice(i, 1)
    order.splice(i + d, 0, c)
    for (const [pos, x] of order.entries()) if (x.position !== pos) await api.patch(`/configurations/${x.id}`, { position: pos })
    await cfg.load()
  } catch (e) {
    notify.fail(e)
  }
}
</script>

<template>
  <section class="card" aria-labelledby="h-configs">
    <div class="row" style="justify-content: space-between; margin-bottom: 8px">
      <h2 id="h-configs" class="plain-heading">{{ t('configurations.title') }}</h2>
      <span class="row nowrap">
        <HelpLink page="administration" anchor="test-configurations" />
        <Button v-if="canEdit" :label="t('configurations.generate')" icon="pi pi-th-large" severity="secondary" outlined aria-haspopup="dialog" @click="openGenerate" />
        <Button v-if="canEdit" :label="t('configurations.new')" icon="pi pi-plus" aria-haspopup="dialog" @click="openCreate" />
      </span>
    </div>
    <p class="small muted" style="margin-top: 0">{{ t('configurations.intro') }}</p>
    <div v-if="canEdit && selection.length" class="bulkbar" role="toolbar" :aria-label="t('configurations.bulkToolbar')">
      <b>{{ t('common.selected', { count: selection.length }) }}</b>
      <Button :label="t('common.delete')" icon="pi pi-trash" size="small" severity="danger" outlined @click="bulkDelete" />
      <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
    </div>
    <DataTable v-model:selection="selection" v-stack :value="list" data-key="id" :loading="loading" :aria-busy="loading" size="small">
      <Column v-if="canEdit" selection-mode="multiple" style="width: 2.6rem" />
      <Column field="name" :header="t('common.name')" body-class="stack-main">
        <template #body="{ data }">
          <div>
            <b>{{ data.name }}</b>
            <div v-if="data.description" class="small muted">{{ data.description }}</div>
          </div>
        </template>
      </Column>
      <Column :header="t('configurations.values')">
        <template #body="{ data }">
          <span class="row" style="gap: 4px">
            <Tag v-for="v in data.values" :key="v.dimension" severity="secondary" :value="`${v.dimension}: ${v.value}`" />
          </span>
        </template>
      </Column>
      <Column field="run_count" :header="t('configurations.runs')" style="width: 90px" />
      <Column v-if="canEdit" style="width: 170px">
        <template #body="{ data, index }">
          <span class="row nowrap" style="gap: 0">
            <Button icon="pi pi-arrow-up" text rounded size="small" severity="secondary" :disabled="index === 0" :aria-label="t('configurations.moveUp', { name: data.name })" v-tooltip="t('common.moveUp')" @click="move(data, -1)" />
            <Button icon="pi pi-arrow-down" text rounded size="small" severity="secondary" :disabled="index === list.length - 1" :aria-label="t('configurations.moveDown', { name: data.name })" v-tooltip="t('common.moveDown')" @click="move(data, 1)" />
            <Button icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="t('configurations.editNamed', { name: data.name })" v-tooltip="t('common.edit')" aria-haspopup="dialog" @click="openEdit(data)" />
            <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('configurations.deleteNamed', { name: data.name })" v-tooltip="t('common.delete')" @click="remove(data)" />
          </span>
        </template>
      </Column>
      <template #empty><div class="empty"><i class="pi pi-sliders-h" aria-hidden="true" />{{ t('configurations.none') }}</div></template>
    </DataTable>
  </section>

  <Dialog v-model:visible="dialog" :header="editing ? t('configurations.editHeader') : t('configurations.newHeader')" modal style="width: min(560px, 96vw)">
    <form id="cfg-form" class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="field">
        <label for="cfg-name">{{ t('common.name') }}</label>
        <InputText id="cfg-name" v-model="form.name" :placeholder="derivedName || t('configurations.namePlaceholder')" aria-describedby="cfg-name-help" autofocus />
        <small id="cfg-name-help" class="muted">{{ t('configurations.nameHelp') }}</small>
      </div>
      <fieldset class="dim-set">
        <legend class="field-label">{{ t('configurations.values') }}</legend>
        <div v-for="(v, i) in form.values" :key="i" class="row nowrap dim-row">
          <InputText v-model="v.dimension" class="grow" list="cfg-dims" :placeholder="t('configurations.dimensionPlaceholder')" :aria-label="t('configurations.dimensionN', { n: i + 1 })" />
          <InputText v-model="v.value" class="grow" :placeholder="t('configurations.valuePlaceholder')" :aria-label="t('configurations.valueN', { n: i + 1 })" />
          <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('configurations.removeValue', { n: i + 1 })" @click="form.values.splice(i, 1)" />
        </div>
        <datalist id="cfg-dims"><option v-for="d in knownDimensions" :key="d" :value="d" /></datalist>
        <Button :label="t('configurations.addValue')" icon="pi pi-plus" text size="small" @click="form.values.push({ dimension: '', value: '' })" />
      </fieldset>
      <div class="field">
        <label for="cfg-desc">{{ t('common.description') }}</label>
        <Textarea id="cfg-desc" v-model="form.description" rows="2" auto-resize />
      </div>
    </form>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
      <Button type="submit" form="cfg-form" :label="editing ? t('common.save') : t('common.create')" icon="pi pi-check" :loading="busy" :disabled="!form.name.trim() && !derivedName" />
    </template>
  </Dialog>

  <Dialog v-model:visible="genDialog" :header="t('configurations.generateHeader')" modal style="width: min(640px, 96vw)">
    <div class="grid" style="gap: 12px">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <p class="small muted" style="margin: 0">{{ t('configurations.generateIntro') }}</p>
      <div v-for="(d, i) in genDims" :key="i" class="row nowrap dim-row">
        <InputText v-model="d.dimension" style="width: 34%" :placeholder="t('configurations.dimensionPlaceholder')" :aria-label="t('configurations.dimensionN', { n: i + 1 })" />
        <InputText v-model="d.values" class="grow" :placeholder="t('configurations.valuesPlaceholder')" :aria-label="t('configurations.valuesOfN', { n: i + 1 })" />
        <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('configurations.removeDimension', { n: i + 1 })" @click="genDims.splice(i, 1)" />
      </div>
      <Button :label="t('configurations.addDimension')" icon="pi pi-plus" text size="small" style="justify-self: start" @click="genDims.push({ dimension: '', values: '' })" />
      <div class="row">
        <Checkbox v-model="skipExisting" binary input-id="cfg-skip" />
        <label for="cfg-skip">{{ t('configurations.skipExisting') }}</label>
      </div>
      <div aria-live="polite">
        <b class="small">{{ t('configurations.preview', { n: combos.length }, combos.length) }}</b>
        <span v-if="comboExisting.length" class="small muted"> · {{ t('configurations.previewExisting', { n: comboExisting.length }, comboExisting.length) }}</span>
        <div class="row preview" style="gap: 4px; margin-top: 6px">
          <Tag v-for="n in comboNames.slice(0, 40)" :key="n" :value="n" :severity="comboExisting.includes(n) ? 'secondary' : 'info'" />
          <span v-if="comboNames.length > 40" class="small muted">…</span>
        </div>
        <Message v-if="combos.length > MAX_COMBOS" severity="warn" :closable="false" style="margin-top: 8px">{{ t('configurations.tooMany', { max: MAX_COMBOS }) }}</Message>
      </div>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="genDialog = false" />
      <Button :label="t('configurations.create', { n: combos.length }, combos.length)" icon="pi pi-check" :loading="busy" :disabled="!combos.length || combos.length > MAX_COMBOS" @click="generate" />
    </template>
  </Dialog>
</template>

<style scoped>
.dim-set {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 8px 10px;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.dim-row {
  gap: 6px;
}
.preview {
  max-height: 160px;
  overflow: auto;
}
</style>
