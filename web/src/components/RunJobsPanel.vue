<script setup lang="ts">
// Progress of "Run automated tests" on the run page: the counts of the
// current background jobs (GET /runs/{id}/jobs), polled while jobs are
// queued or running, the queue and "Cancel queued".
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import ProgressBar from 'primevue/progressbar'
import Tag from 'primevue/tag'
import HelpTip from '@/guide/HelpTip.vue'
import { api } from '@/lib/api'
import { EMPTY_COUNTS, jobProgress, pollDelay, type RunJobs } from '@/lib/webTests'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ runId: string; canCancel: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const notify = useNotify()

const jobs = ref<RunJobs | null>(null)
const counts = computed(() => jobs.value?.counts ?? EMPTY_COUNTS)
const active = computed(() => !!jobs.value?.active)
const expanded = ref(false)
/** the latest batch stays visible (with its results) until closed */
const closed = ref(false)
const shown = computed(() => counts.value.total > 0 && (active.value || !closed.value))
const progress = computed(() => jobProgress(counts.value))
const other = computed(() => counts.value.done - counts.value.passed - counts.value.failed)
const STATE_SEVERITY: Record<string, string> = { passed: 'success', failed: 'danger', running: 'info', queued: 'secondary', error: 'warn', interrupted: 'warn' }

let timer: ReturnType<typeof setTimeout> | undefined
let attempt = 0
let lastDone = -1
async function load() {
  timer = undefined
  try {
    const j = await api.get<RunJobs>(`/runs/${props.runId}/jobs`)
    jobs.value = j
    if (j.active) closed.value = false
    if (lastDone >= 0 && j.counts.done !== lastDone) emit('changed')
    lastDone = j.counts.done
    if (j.active) timer = setTimeout(load, pollDelay(Math.min(attempt++, 2)))
    else attempt = 0
  } catch {
    if (active.value) timer = setTimeout(load, pollDelay(attempt++))
  }
}
/** starts following new jobs (after "Run automated tests") */
function refresh() {
  if (timer) clearTimeout(timer)
  closed.value = false
  attempt = 0
  void load()
}
defineExpose({ refresh })
onMounted(load)
watch(() => props.runId, refresh)
onBeforeUnmount(() => timer && clearTimeout(timer))

async function cancelQueued() {
  try {
    const r = await api.post<{ cancelled: number }>(`/runs/${props.runId}/jobs/cancel`, {})
    notify.ok(t('webTests.cancelledN', { n: r?.cancelled ?? 0 }, r?.cancelled ?? 0))
    refresh()
    emit('changed')
  } catch (e) {
    notify.fail(e, t('webTests.cancelFailed'))
  }
}
const label = (job: { test_key?: string; configuration_name?: string }) => (job.configuration_name ? `${job.test_key} (${job.configuration_name})` : job.test_key ?? '')
</script>

<template>
  <section v-if="shown" class="card run-jobs" aria-labelledby="h-run-jobs" style="margin-bottom: 16px">
    <div class="row" style="justify-content: space-between; flex-wrap: wrap">
      <h2 id="h-run-jobs" class="plain-heading">
        <i class="pi pi-bolt" aria-hidden="true" /> {{ t('webTests.jobsTitle') }}
        <HelpTip id="run.automated" :label="t('webTests.runAll')" />
      </h2>
      <span class="row nowrap">
        <Button v-if="canCancel && counts.queued" :label="t('webTests.cancelQueued')" icon="pi pi-times" size="small" severity="secondary" outlined @click="cancelQueued" />
        <Button :label="expanded ? t('webTests.hideJobs') : t('webTests.showJobs')" :icon="expanded ? 'pi pi-chevron-up' : 'pi pi-chevron-down'" size="small" text :aria-expanded="expanded" aria-controls="run-jobs-list" @click="expanded = !expanded" />
        <Button v-if="!active" icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('common.close')" v-tooltip="t('common.close')" @click="closed = true" />
      </span>
    </div>
    <p class="small" role="status" style="margin: 6px 0">
      {{ t('webTests.jobsCounts', { queued: counts.queued, running: counts.running, done: counts.done, total: counts.total }) }}
      <span class="muted">— {{ t('webTests.jobsResults', { passed: counts.passed, failed: counts.failed, other }) }}</span>
    </p>
    <ProgressBar :value="progress" :show-value="false" :aria-label="t('webTests.jobsProgress')" style="height: 6px" />
    <ul v-if="expanded" id="run-jobs-list" class="plain-list job-list" :aria-label="t('webTests.jobsList')">
      <li v-for="j in jobs?.items ?? []" :key="j.id" class="job-row">
        <Tag :value="t(`webTests.state.${j.state}`)" :severity="STATE_SEVERITY[j.state] ?? 'secondary'" />
        <span class="mono small">{{ label(j) }}</span>
        <span class="ellipsis grow small">{{ j.test_title }}</span>
        <span v-if="j.message && j.state !== 'passed'" class="small muted job-message">{{ j.message.split('\n')[0] }}</span>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.job-list {
  margin-top: 10px;
  max-height: 320px;
  overflow: auto;
}
.job-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 10px;
  padding: 6px 0;
  border-bottom: 1px solid var(--qtf-border);
}
.job-message {
  flex-basis: 100%;
  white-space: pre-wrap;
}
</style>
