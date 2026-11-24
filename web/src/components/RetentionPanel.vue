<script setup lang="ts">
// The retention section of Administration → System settings (super
// admins): the dry-run preview of the purge per organisation and category
// (GET /admin/retention), the projects and releases under legal hold, and
// "Purge now" after a typed confirmation (POST /admin/retention/run).
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { useI18n } from 'vue-i18n'
import { api, errorMessage } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { formatDateTime } from '@/lib/format'
import { countOf, grandTotals, PURGE_WORD, RETENTION_CATEGORIES, type RetentionReport, type RetentionView } from '@/lib/privacy'
import { useNotify } from '@/composables/notify'

const { t } = useI18n()
const notify = useNotify()
const id = uid('ret')
const view = ref<RetentionView | null>(null)
const error = ref('')
const loading = ref(false)
const announce = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    view.value = await api.get<RetentionView>('/admin/retention')
    announce.value = t('privacy.retention.previewReady', { n: totals.value.count })
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

const totals = computed(() => grandTotals(view.value?.reports ?? []))
const tenantName = (r: RetentionReport) => (r.tenant_id ? r.tenant_name || r.tenant_id : t('privacy.retention.system'))
function cell(r: RetentionReport, c: (typeof RETENTION_CATEGORIES)[number]) {
  const x = countOf(r, c)
  if (!x) return { text: '—', label: t('privacy.retention.notApplicable') }
  if (!x.days) return { text: t('privacy.retention.forever'), label: t('privacy.retention.forever') }
  const text = x.held ? t('privacy.retention.cellHeld', { n: x.count, held: x.held }) : String(x.count)
  return { text, label: t('privacy.retention.cellLabel', { n: x.count, held: x.held, days: x.days }) }
}

const confirmOpen = ref(false)
const typed = ref('')
const purging = ref(false)
function openPurge() {
  typed.value = ''
  confirmOpen.value = true
}
async function purge() {
  purging.value = true
  try {
    view.value = await api.post<RetentionView>('/admin/retention/run', { confirm: typed.value.trim() })
    confirmOpen.value = false
    const msg = t('privacy.retention.purged', { n: totals.value.count })
    notify.ok(msg)
    announce.value = msg
  } catch (e) {
    notify.fail(e)
  } finally {
    purging.value = false
  }
}
</script>

<template>
  <section class="retention-panel" :aria-labelledby="`${id}-h`">
    <div class="row" style="gap: 8px; align-items: center; flex-wrap: wrap">
      <h3 :id="`${id}-h`" class="plain-heading sys-sub" style="margin: 0">{{ t('privacy.retention.title') }}</h3>
      <span class="grow" />
      <Button :label="t('privacy.retention.refresh')" icon="pi pi-refresh" severity="secondary" outlined size="small" :loading="loading" @click="load" />
      <Button :label="t('privacy.retention.purgeNow')" icon="pi pi-trash" severity="danger" outlined size="small" :disabled="!view || totals.count === 0" @click="openPurge" />
    </div>
    <p class="small muted">{{ t('privacy.retention.intro') }}</p>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <template v-if="view">
      <p class="small">
        <Tag :value="view.enabled ? t('privacy.retention.jobOn') : t('privacy.retention.jobOff')" :severity="view.enabled ? 'success' : 'secondary'" />
        <span v-if="view.enabled && view.next_run">&nbsp;{{ t('privacy.retention.nextRun', { date: formatDateTime(view.next_run) }) }}</span>
        <span v-if="view.last_run">&nbsp;{{ t('privacy.retention.lastRun', { date: formatDateTime(view.last_run) }) }}</span>
      </p>
      <div class="table-scroll">
        <table class="ret-table">
          <caption class="sr-only">{{ t('privacy.retention.caption') }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ t('privacy.retention.organisation') }}</th>
              <th v-for="c in RETENTION_CATEGORIES" :key="c" scope="col">{{ t(`privacy.category.${c}`) }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in view.reports" :key="r.tenant_id || '-'">
              <th scope="row">
                {{ tenantName(r) }}
                <Tag v-if="r.policy.overridden?.length" :value="t('privacy.retention.override')" severity="info" class="ret-tag" />
              </th>
              <td v-for="c in RETENTION_CATEGORIES" :key="c" :aria-label="cell(r, c).label">{{ cell(r, c).text }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="small muted">{{ t('privacy.retention.totals', { n: totals.count, held: totals.held }) }}</p>
      <Message severity="info" :closable="false"><i class="pi pi-lock" aria-hidden="true" /> {{ t('privacy.retention.auditNote') }}</Message>

      <h3 class="plain-heading sys-sub">{{ t('privacy.hold.listTitle') }}</h3>
      <p class="small muted">{{ t('privacy.hold.listIntro') }}</p>
      <ul v-if="view.holds.length" class="plain-list hold-list">
        <li v-for="h in view.holds" :key="h.kind + h.id">
          <i :class="h.kind === 'project' ? 'pi pi-folder' : 'pi pi-flag'" aria-hidden="true" />
          <b>{{ h.kind === 'project' ? h.project_name : `${h.project_key} › ${h.name}` }}</b>
          <span class="small muted">{{ h.tenant_name }} · {{ t(`privacy.hold.kind.${h.kind}`) }}</span>
          <div class="small">{{ h.hold.reason }}<span v-if="h.hold.by" class="muted"> — {{ t('privacy.hold.setBy', { who: h.hold.by, date: formatDateTime(h.hold.at) }) }}</span></div>
        </li>
      </ul>
      <p v-else class="small muted">{{ t('privacy.hold.none') }}</p>
    </template>
    <div class="sr-only" role="status" aria-live="polite">{{ announce }}</div>

    <Dialog v-model:visible="confirmOpen" :header="t('privacy.retention.confirmTitle')" modal style="width: min(480px, 95vw)">
      <form class="grid" style="gap: 12px" @submit.prevent="typed.trim() === PURGE_WORD && purge()">
        <p class="small">{{ t('privacy.retention.confirmText', { n: totals.count }) }}</p>
        <div v-field class="field">
          <label>{{ t('privacy.retention.confirmLabel', { word: PURGE_WORD }) }}</label>
          <InputText v-model="typed" autocomplete="off" class="mono" />
        </div>
        <div class="row" style="justify-content: flex-end; gap: 8px">
          <Button :label="t('common.cancel')" text severity="secondary" @click="confirmOpen = false" />
          <Button type="submit" :label="t('privacy.retention.purgeNow')" icon="pi pi-trash" severity="danger" :loading="purging" :disabled="typed.trim() !== PURGE_WORD" />
        </div>
      </form>
    </Dialog>
  </section>
</template>

<style scoped>
.retention-panel {
  margin-top: 16px;
  display: grid;
  gap: 8px;
}
.ret-table {
  border-collapse: collapse;
  width: 100%;
  font-size: 0.9rem;
}
.ret-table th,
.ret-table td {
  border-bottom: 1px solid var(--p-content-border-color);
  padding: 6px 8px;
  text-align: left;
  vertical-align: top;
}
.ret-table td {
  font-variant-numeric: tabular-nums;
}
.ret-tag {
  margin-left: 4px;
}
.hold-list li {
  padding: 6px 0;
  border-bottom: 1px solid var(--p-content-border-color);
}
.hold-list i {
  margin-right: 6px;
}
</style>
