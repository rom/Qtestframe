<script setup lang="ts">
// Administration › Electronic records: the organisation's
// electronic-records mode (21 CFR Part 11 / EU GMP Annex 11) and the
// "Part 11 / Annex 11 controls" report with what the tool provides and what
// the organisation's procedures must cover.
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import HelpLink from '@/guide/HelpLink.vue'
import { api, errorMessage } from '@/lib/api'
import { controlSeverity, parseLines, utcStamp, type ComplianceReport, type ERecordsInfo, type ERecordsSettings } from '@/lib/erecords'
import { P } from '@/lib/permissions'
import { useAuthStore } from '@/stores/auth'
import { useERecordsStore } from '@/stores/erecords'
import { useNotify } from '@/composables/notify'

const { t, te } = useI18n()
const auth = useAuthStore()
const er = useERecordsStore()
const notify = useNotify()
const info = ref<ERecordsInfo | null>(null)
const form = ref<ERecordsSettings | null>(null)
const meaningsText = ref('')
const saving = ref(false)
const error = ref('')
const report = ref<ComplianceReport | null>(null)
const reportBusy = ref(false)
const canManage = computed(() => auth.can(P.tenantManage))
const canReport = computed(() => auth.can(P.auditRead))

async function load() {
  error.value = ''
  try {
    info.value = await api.get<ERecordsInfo>('/erecords')
    form.value = { ...info.value.settings, signature_actions: [...info.value.settings.signature_actions], reason_edits: [...info.value.settings.reason_edits] }
    meaningsText.value = info.value.settings.meanings.join('\n')
  } catch (e) {
    error.value = errorMessage(e)
  }
}
onMounted(load)

const label = (group: string, k: string) => (te(`erecords.${group}.${k}`) ? t(`erecords.${group}.${k}`) : k)

async function save() {
  if (!form.value) return
  saving.value = true
  try {
    const st = await api.put<ERecordsSettings>('/erecords', { ...form.value, meanings: parseLines(meaningsText.value) })
    er.set(st)
    notify.ok(t('erecords.admin.saved'))
    await load()
    if (report.value) await loadReport()
  } catch (e) {
    notify.fail(e)
  } finally {
    saving.value = false
  }
}

async function loadReport() {
  reportBusy.value = true
  try {
    report.value = await api.get<ComplianceReport>('/erecords/report')
  } catch (e) {
    notify.fail(e)
  } finally {
    reportBusy.value = false
  }
}
async function downloadReport() {
  try {
    await api.download('/erecords/report', { format: 'md' }, 'part11-annex11-controls.md')
  } catch (e) {
    notify.fail(e)
  }
}
</script>

<template>
  <div class="card er-admin">
    <div class="row" style="gap: 8px">
      <h2 class="grow" style="margin: 0">{{ t('erecords.admin.title') }}</h2>
      <HelpLink page="administration" :anchor="{ en: 'electronic-records-mode', sv: 'läge-för-elektroniska-poster' }" />
    </div>
    <p class="small muted">{{ t('erecords.admin.intro') }}</p>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <form v-if="form" class="er-grid" @submit.prevent="save">
      <div class="row" style="gap: 10px">
        <ToggleSwitch v-model="form.enabled" input-id="er-enabled" :disabled="!canManage" />
        <label for="er-enabled"><b>{{ t('erecords.admin.enabled') }}</b></label>
        <Tag :value="form.enabled ? t('erecords.admin.on') : t('erecords.admin.off')" :severity="form.enabled ? 'success' : 'secondary'" />
      </div>
      <p class="small muted" style="margin: 0">{{ t('erecords.admin.enabledHint') }}</p>

      <fieldset class="er-set">
        <legend>{{ t('erecords.admin.signatureActions') }}</legend>
        <p class="small muted">{{ t('erecords.admin.signatureActionsHint') }}</p>
        <div v-for="a in info?.signature_actions ?? []" :key="a" class="row" style="gap: 8px">
          <Checkbox v-model="form.signature_actions" :input-id="`er-act-${a}`" :value="a" :disabled="!canManage" />
          <label :for="`er-act-${a}`">{{ label('actionName', a) }}</label>
        </div>
      </fieldset>

      <fieldset class="er-set">
        <legend>{{ t('erecords.admin.meanings') }}</legend>
        <label for="er-meanings" class="small">{{ t('erecords.admin.meaningsHint') }}</label>
        <Textarea id="er-meanings" v-model="meaningsText" rows="5" auto-resize fluid :disabled="!canManage" />
      </fieldset>

      <fieldset class="er-set">
        <legend>{{ t('erecords.admin.reasonEdits') }}</legend>
        <p class="small muted">{{ t('erecords.admin.reasonEditsHint') }}</p>
        <div class="er-columns">
          <div v-for="k in info?.reason_edits ?? []" :key="k" class="row" style="gap: 8px">
            <Checkbox v-model="form.reason_edits" :input-id="`er-edit-${k}`" :value="k" :disabled="!canManage" />
            <label :for="`er-edit-${k}`">{{ label('edit', k) }}</label>
          </div>
        </div>
      </fieldset>

      <fieldset class="er-set">
        <legend>{{ t('erecords.admin.sessions') }}</legend>
        <div class="er-columns">
          <div v-field class="field">
            <label for="er-idle">{{ t('erecords.admin.idle') }}</label>
            <InputNumber v-model="form.idle_timeout_minutes" input-id="er-idle" :min="1" :max="480" show-buttons :disabled="!canManage" :suffix="' ' + t('erecords.admin.minutes')" />
          </div>
          <div v-field class="field">
            <label for="er-sso">{{ t('erecords.admin.sso') }}</label>
            <InputNumber v-model="form.sso_reauth_minutes" input-id="er-sso" :min="1" :max="60" show-buttons :disabled="!canManage" :suffix="' ' + t('erecords.admin.minutes')" />
          </div>
        </div>
      </fieldset>
      <div v-if="canManage" class="row" style="justify-content: flex-end; gap: 8px">
        <span v-if="form.updated_by" class="small muted grow">{{ t('erecords.admin.updated', { by: form.updated_by, at: form.updated_at ? utcStamp(form.updated_at) : '' }) }}</span>
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="saving" />
      </div>
    </form>
  </div>

  <div v-if="canReport" class="card er-admin">
    <div class="row" style="gap: 8px; flex-wrap: wrap">
      <h2 class="grow" style="margin: 0">{{ t('erecords.report.title') }}</h2>
      <Button :label="t('erecords.report.run')" icon="pi pi-list-check" :loading="reportBusy" outlined @click="loadReport" />
      <Button :label="t('erecords.report.download')" icon="pi pi-download" severity="secondary" outlined @click="downloadReport" />
    </div>
    <p class="small muted">{{ t('erecords.report.intro') }}</p>
    <template v-if="report">
      <Message severity="warn" :closable="false">{{ report.disclaimer }}</Message>
      <p class="small">{{ t('erecords.report.generated', { at: utcStamp(report.generated_at), by: report.generated_by }) }}</p>
      <div class="table-scroll" tabindex="0" role="region" :aria-label="t('erecords.report.title')">
        <table class="er-table">
          <caption class="sr-only">{{ t('erecords.report.title') }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ t('erecords.report.control') }}</th>
              <th scope="col">{{ t('erecords.report.coveredBy') }}</th>
              <th scope="col">{{ t('erecords.report.status') }}</th>
              <th scope="col">{{ t('erecords.report.tool') }}</th>
              <th scope="col">{{ t('erecords.report.customer') }}</th>
              <th scope="col">{{ t('erecords.report.evidence') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in report.controls" :key="c.id">
              <th scope="row">
                <div>{{ c.title }}</div>
                <div class="small muted">{{ c.references.join(' · ') }}</div>
              </th>
              <td>{{ label('provider', c.provider) }}</td>
              <td><Tag :value="label('status', c.status)" :severity="controlSeverity(c.status)" /></td>
              <td class="small">{{ c.tool }}</td>
              <td class="small">{{ c.customer }}</td>
              <td class="small">
                <ul v-if="c.evidence?.length" class="plain-list">
                  <li v-for="(ev, i) in c.evidence" :key="i">{{ ev }}</li>
                </ul>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>

<style scoped>
.er-admin {
  margin-bottom: 16px;
}
.er-grid {
  display: grid;
  gap: 12px;
  max-width: 900px;
}
.er-set {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 8px 12px;
  display: grid;
  gap: 6px;
}
.er-set legend {
  font-weight: 600;
  padding: 0 4px;
}
.er-set p {
  margin: 0;
}
.er-columns {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 240px), 1fr));
  gap: 6px 16px;
}
.table-scroll {
  overflow-x: auto;
}
.er-table {
  border-collapse: collapse;
  width: 100%;
  min-width: 760px;
}
.er-table th,
.er-table td {
  text-align: left;
  vertical-align: top;
  padding: 6px 8px;
  border-bottom: 1px solid var(--qtf-border);
}
.er-table tbody th {
  font-weight: 600;
  width: 22%;
}
.plain-list {
  margin: 0;
  padding-left: 16px;
}
</style>
