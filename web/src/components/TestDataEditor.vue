<script setup lang="ts">
// The "Test data" section of the test editor: parameters and data rows of a
// data-driven test. Steps use parameters as {{name}}; a run executes the
// test once per row (an iteration).
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import HelpTip from '@/guide/HelpTip.vue'
import { moveItem } from '@/lib/collections'
import {
  MAX_PARAMETERS,
  MAX_ROWS,
  blankRow,
  dataToTSV,
  parameterProblem,
  parseTable,
  placeholderOf,
  removeParameter,
  renameParameter,
  tableToData,
  unknownPlaceholders,
} from '@/lib/testData'
import type { DataRow } from '@/lib/types'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ canWrite: boolean; texts: string[] }>()
const parameters = defineModel<string[]>('parameters', { required: true })
const rows = defineModel<DataRow[]>('rows', { required: true })
const { t } = useI18n()
const notify = useNotify()

const example = placeholderOf('username')
const unknown = computed(() => unknownPlaceholders(props.texts, parameters.value))
const problems = computed(() => parameters.value.map((_, i) => parameterProblem(parameters.value, i)))

function addParameter() {
  let n = parameters.value.length + 1
  while (parameters.value.includes(`param${n}`)) n++
  const name = `param${n}`
  parameters.value = [...parameters.value, name]
  rows.value = rows.value.map((r) => ({ ...r, values: { ...r.values, [name]: '' } }))
}
function renameAt(i: number, to: string) {
  const from = parameters.value[i]
  const name = to.trim()
  if (name === from) return
  parameters.value = parameters.value.map((p, j) => (j === i ? name : p))
  rows.value = renameParameter(rows.value, from, name)
}
function removeAt(i: number) {
  const name = parameters.value[i]
  parameters.value = parameters.value.filter((_, j) => j !== i)
  rows.value = removeParameter(rows.value, name)
}
function moveParam(i: number, d: number) {
  parameters.value = moveItem(parameters.value, i, i + d)
}
function addRow() {
  rows.value = [...rows.value, blankRow(parameters.value)]
}
function removeRow(i: number) {
  rows.value = rows.value.filter((_, j) => j !== i)
}
function moveRow(i: number, d: number) {
  rows.value = moveItem(rows.value, i, i + d)
}
function setCell(i: number, p: string, v: string) {
  rows.value[i].values = { ...rows.value[i].values, [p]: v }
}

// ---------- paste from a spreadsheet or CSV ----------
const pasteOpen = ref(false)
const pasteText = ref('')
const pasteHeader = ref(true)
const pasteNames = ref(false)
const pasteReplace = ref(true)
function openPaste() {
  pasteText.value = ''
  pasteHeader.value = !parameters.value.length
  pasteNames.value = false
  pasteReplace.value = !rows.value.length || !parameters.value.length
  pasteOpen.value = true
}
const pasted = computed(() =>
  tableToData(parseTable(pasteText.value), { header: pasteHeader.value, nameColumn: pasteNames.value, parameters: parameters.value }),
)
const pasteProblem = computed(() => {
  const p = pasted.value
  if (!p.parameters.length) return t('testData.pasteNoParameters')
  if (p.parameters.some((_, i) => parameterProblem(p.parameters, i))) return t('testData.pasteBadHeader')
  if (p.parameters.length > MAX_PARAMETERS) return t('testData.tooManyParameters', { max: MAX_PARAMETERS })
  if ((pasteReplace.value ? 0 : rows.value.length) + p.rows.length > MAX_ROWS) return t('testData.tooManyRows', { max: MAX_ROWS })
  return ''
})
function applyPaste() {
  const p = pasted.value
  if (pasteReplace.value) {
    parameters.value = p.parameters
    rows.value = p.rows
  } else {
    const params = [...parameters.value, ...p.parameters.filter((x) => !parameters.value.includes(x))]
    parameters.value = params
    rows.value = [...rows.value, ...p.rows].map((r) => ({ ...r, values: Object.fromEntries(params.map((x) => [x, r.values?.[x] ?? ''])) }))
  }
  pasteOpen.value = false
  notify.ok(t('testData.pasted', { n: p.rows.length }, p.rows.length))
}
async function copyTSV() {
  try {
    await navigator.clipboard.writeText(dataToTSV(parameters.value, rows.value))
    notify.ok(t('testData.copied'))
  } catch {
    notify.info(t('errors.copyFailed'), t('errors.copyManually'))
  }
}
</script>

<template>
  <section aria-labelledby="test-data-heading">
    <div class="section-title">
      <i class="pi pi-table" aria-hidden="true" /> <span id="test-data-heading">{{ t('testData.title', { n: rows.length }, rows.length) }}</span>
      <HelpTip id="test.data" :label="t('testData.title', { n: rows.length }, rows.length)" />
      <span class="grow" />
      <Button v-if="canWrite" size="small" :label="t('testData.paste')" icon="pi pi-clipboard" text aria-haspopup="dialog" @click="openPaste" />
      <Button v-if="rows.length" size="small" :label="t('testData.copy')" icon="pi pi-copy" text @click="copyTSV" />
      <Button v-if="canWrite" size="small" :label="t('testData.addParameter')" icon="pi pi-plus" text :disabled="parameters.length >= MAX_PARAMETERS" @click="addParameter" />
      <Button v-if="canWrite && parameters.length" size="small" :label="t('testData.addRow')" icon="pi pi-plus" text :disabled="rows.length >= MAX_ROWS" @click="addRow" />
    </div>
    <p v-if="!parameters.length" class="muted small" style="margin: 0">{{ t('testData.none', { example }) }}</p>
    <template v-else>
      <p class="small muted" style="margin: 0 0 6px">{{ t('testData.hint', { example: placeholderOf(parameters[0] || 'name') }) }}</p>
      <div class="table-scroll">
        <table class="data-table">
          <caption class="sr-only">{{ t('testData.caption', { params: parameters.length, rows: rows.length }) }}</caption>
          <thead>
            <tr>
              <th scope="col" style="width: 2.5rem">#</th>
              <th scope="col" style="min-width: 120px">{{ t('testData.rowName') }}</th>
              <th v-for="(p, i) in parameters" :key="i" scope="col" style="min-width: 140px">
                <div class="row nowrap" style="gap: 2px">
                  <InputText
                    :model-value="p"
                    size="small"
                    class="grow mono"
                    :disabled="!canWrite"
                    :invalid="!!problems[i]"
                    :aria-label="t('testData.parameterN', { n: i + 1 })"
                    :aria-describedby="problems[i] ? `param-problem-${i}` : undefined"
                    @change="(e: Event) => renameAt(i, (e.target as HTMLInputElement).value)"
                  />
                  <template v-if="canWrite">
                    <Button icon="pi pi-angle-left" text rounded size="small" severity="secondary" :disabled="i === 0" :aria-label="t('testData.moveParameterLeft', { name: p })" @click="moveParam(i, -1)" />
                    <Button icon="pi pi-angle-right" text rounded size="small" severity="secondary" :disabled="i === parameters.length - 1" :aria-label="t('testData.moveParameterRight', { name: p })" @click="moveParam(i, 1)" />
                    <Button icon="pi pi-times" text rounded size="small" severity="danger" :aria-label="t('testData.removeParameter', { name: p })" v-tooltip="t('common.remove')" @click="removeAt(i)" />
                  </template>
                </div>
                <small v-if="problems[i]" :id="`param-problem-${i}`" class="warn-text">{{ problems[i] === 'duplicate' ? t('testData.duplicate') : t('testData.invalidName') }}</small>
              </th>
              <th v-if="canWrite" scope="col" style="width: 7rem"><span class="sr-only">{{ t('common.actions') }}</span></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(r, i) in rows" :key="i">
              <th scope="row">{{ i + 1 }}</th>
              <td><InputText v-model="r.name" size="small" class="w-full" :disabled="!canWrite" :placeholder="t('testData.rowN', { n: i + 1 })" :aria-label="t('testData.rowNameN', { n: i + 1 })" /></td>
              <td v-for="p in parameters" :key="p">
                <InputText :model-value="r.values?.[p] ?? ''" size="small" class="w-full" :disabled="!canWrite" :aria-label="t('testData.cell', { param: p, n: i + 1 })" @update:model-value="(v) => setCell(i, p, v ?? '')" />
              </td>
              <td v-if="canWrite">
                <div class="row nowrap" style="gap: 0">
                  <Button icon="pi pi-arrow-up" text rounded size="small" severity="secondary" :disabled="i === 0" :aria-label="t('testData.moveRowUp', { n: i + 1 })" v-tooltip="t('common.moveUp')" @click="moveRow(i, -1)" />
                  <Button icon="pi pi-arrow-down" text rounded size="small" severity="secondary" :disabled="i === rows.length - 1" :aria-label="t('testData.moveRowDown', { n: i + 1 })" v-tooltip="t('common.moveDown')" @click="moveRow(i, 1)" />
                  <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('testData.removeRow', { n: i + 1 })" v-tooltip="t('common.remove')" @click="removeRow(i)" />
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="!rows.length" class="muted small" style="margin: 6px 0 0">{{ t('testData.noRows') }}</p>
    </template>
    <Message v-if="unknown.length" severity="warn" :closable="false" style="margin-top: 8px">{{ t('testData.unknown', { names: unknown.join(', ') }) }}</Message>

    <Dialog v-model:visible="pasteOpen" :header="t('testData.pasteHeader')" modal style="width: min(640px, 96vw)">
      <div class="grid" style="gap: 10px">
        <label for="td-paste" class="small" style="font-weight: 600">{{ t('testData.pasteLabel') }}</label>
        <Textarea id="td-paste" v-model="pasteText" rows="8" class="mono" aria-describedby="td-paste-help" autofocus />
        <small id="td-paste-help" class="muted">{{ t('testData.pasteHelp') }}</small>
        <div class="row"><Checkbox v-model="pasteHeader" binary input-id="td-header" /><label for="td-header">{{ t('testData.pasteHeaderRow') }}</label></div>
        <div class="row"><Checkbox v-model="pasteNames" binary input-id="td-names" /><label for="td-names">{{ t('testData.pasteNameColumn') }}</label></div>
        <div class="row"><Checkbox v-model="pasteReplace" binary input-id="td-replace" /><label for="td-replace">{{ t('testData.pasteReplace') }}</label></div>
        <div aria-live="polite" class="small">
          <Message v-if="pasteText.trim() && pasteProblem" severity="warn" :closable="false">{{ pasteProblem }}</Message>
          <span v-else-if="pasteText.trim()">{{ t('testData.pastePreview', { rows: pasted.rows.length, params: pasted.parameters.join(', ') }, pasted.rows.length) }}</span>
        </div>
      </div>
      <template #footer>
        <Button :label="t('common.cancel')" text severity="secondary" @click="pasteOpen = false" />
        <Button :label="t('testData.pasteApply')" icon="pi pi-check" :disabled="!pasteText.trim() || !!pasteProblem" @click="applyPaste" />
      </template>
    </Dialog>
  </section>
</template>

<style scoped>
.table-scroll {
  overflow-x: auto;
  max-width: 100%;
}
.data-table {
  border-collapse: collapse;
  width: 100%;
}
.data-table th,
.data-table td {
  border-bottom: 1px solid var(--qtf-border);
  padding: 4px;
  text-align: left;
  vertical-align: top;
}
.data-table thead th {
  font-size: 12px;
  font-weight: 600;
}
</style>
