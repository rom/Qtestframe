<script setup lang="ts">
// The session runner of an exploratory test session: the charter and its
// scope, the timer (kept by the server: start, pause, stop), timestamped
// notes typed observation / question / issue / idea / bug with keyboard
// shortcuts, screenshots and files, defects raised from notes, the debrief
// (summary, coverage, SBTM time split, outcome), the report and the history.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Textarea from 'primevue/textarea'
import Select from 'primevue/select'
import MultiSelect from 'primevue/multiselect'
import Message from 'primevue/message'
import ProgressBar from 'primevue/progressbar'
import ProgressSpinner from 'primevue/progressspinner'
import Tag from 'primevue/tag'
import AttachmentsPanel from '@/components/AttachmentsPanel.vue'
import HistoryPanel from '@/components/HistoryPanel.vue'
import ItemRef from '@/components/ItemRef.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { P } from '@/lib/permissions'
import { DEFECT_SEVERITIES } from '@/lib/defectTracker'
import { contentUrl } from '@/lib/files'
import type { FileInfo } from '@/lib/types'
import {
  NOTE_TYPES,
  OUTCOMES,
  SESSION_SHORTCUTS,
  canTakeNotes,
  debriefBody,
  elapsedNow,
  formatClock,
  isSaveKey,
  noteDefectTitle,
  noteIcon,
  noteSeverity,
  outcomeSeverity,
  scopeGroups,
  sessionAction,
  splitTotal,
  statusIcon,
  statusSeverity,
  suggestSplit,
  timeboxUse,
  timerActions,
  type ExploratoryNote,
  type ExploratorySession,
  type NoteType,
} from '@/lib/exploratory'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string; sessionId: string }>()
const proj = useProjectStore()
const auth = useAuthStore()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()

const x = ref<ExploratorySession | null>(null)
const notes = ref<ExploratoryNote[]>([])
const loading = ref(true)
const error = ref('')
const receivedAt = ref(Date.now())
const now = ref(Date.now())
let tick: ReturnType<typeof setInterval> | undefined

const me = computed(() => (auth.user?.email ?? '').toLowerCase())
const isTester = computed(() => !!x.value && (!x.value.tester || x.value.tester === me.value))
// the tester runs the session; others need run.write as well
const canRun = computed(() => auth.can(P.runExecute) && (isTester.value || auth.can(P.runWrite)))
const canPlan = computed(() => auth.can(P.runWrite))
const canExport = computed(() => auth.can(P.export))
const statusLabel = (s: string) => t(`exploratory.status.${s}`)
const noteLabel = (s: string) => t(`exploratory.noteType.${s}`)
const outcomeLabel = (s: string) => t(`exploratory.outcome.${s}`)

function setSession(s: ExploratorySession) {
  x.value = s
  receivedAt.value = Date.now()
  now.value = receivedAt.value
}
async function load() {
  error.value = ''
  try {
    const [s, n] = await Promise.all([api.get<ExploratorySession>(`/exploratory-sessions/${props.sessionId}`), api.get<{ items: ExploratoryNote[] }>(`/exploratory-sessions/${props.sessionId}/notes`)])
    setSession(s)
    notes.value = asItems(n)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(() => {
  void load()
  tick = setInterval(() => (now.value = Date.now()), 1000)
  window.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  if (tick) clearInterval(tick)
  window.removeEventListener('keydown', onKey)
})
watch(() => props.sessionId, load)

// ---------- timer ----------
const elapsed = computed(() => (x.value ? elapsedNow(x.value, receivedAt.value, now.value) : 0))
const box = computed(() => timeboxUse(elapsed.value, x.value?.timebox_minutes ?? 60))
const actions = computed(() => timerActions(x.value?.status ?? ''))
const timerBusy = ref(false)
async function timer(action: 'start' | 'pause' | 'stop') {
  if (!x.value) return
  timerBusy.value = true
  try {
    setSession(await api.post<ExploratorySession>(`/exploratory-sessions/${x.value.id}/timer`, { action }))
    announce.value = t(`exploratory.timerDone.${action}`, {
      time: formatClock(elapsed.value),
    })
    if (action === 'stop') openDebrief()
  } catch (e) {
    notify.fail(e)
  } finally {
    timerBusy.value = false
  }
}
function toggleTimer() {
  if (actions.value.pause) void timer('pause')
  else if (actions.value.start) void timer('start')
}
function stop() {
  notify.confirmDanger(t('exploratory.confirmStop'), () => timer('stop'), t('exploratory.stopHeader'), t('exploratory.stop'))
}

// ---------- notes ----------
const noteType = ref<NoteType>('observation')
const noteBody = ref('')
const noteFiles = ref<string[]>([])
const noteBusy = ref(false)
const attachments = ref<FileInfo[]>([])
const attachmentsKey = ref(0)
async function loadAttachments() {
  if (!x.value) return
  try {
    attachments.value = asItems(await api.get<{ items: FileInfo[] }>(`/exploratory-sessions/${x.value.id}/attachments`))
  } catch {
    attachments.value = []
  }
}
watch(() => x.value?.id, loadAttachments)
const fileOptions = computed(() => attachments.value.map((f) => ({ label: f.name, value: f.id })))
const noteTypeOptions = computed(() =>
  NOTE_TYPES.map((v) => ({
    value: v,
    label: noteLabel(v),
    key: shortcutOf(v),
  })),
)
const shortcutOf = (v: string) => SESSION_SHORTCUTS.find((s) => s.action === v)?.keys[0] ?? ''
function focusNote() {
  void nextTick(() => {
    const el = document.getElementById('xs-note')
    el?.focus()
  })
}
async function addNote() {
  if (!x.value || !noteBody.value.trim()) return
  noteBusy.value = true
  try {
    const n = await api.post<ExploratoryNote>(`/exploratory-sessions/${x.value.id}/notes`, { type: noteType.value, body: noteBody.value, file_ids: noteFiles.value })
    notes.value = [...notes.value, n]
    x.value = {
      ...x.value,
      note_count: x.value.note_count + 1,
      bug_count: x.value.bug_count + (n.type === 'bug' ? 1 : 0),
    }
    noteBody.value = ''
    noteFiles.value = []
    announce.value = t('exploratory.noteAdded', {
      type: noteLabel(n.type),
      time: formatClock(n.elapsed_seconds),
    })
    focusNote()
  } catch (e) {
    notify.fail(e)
  } finally {
    noteBusy.value = false
  }
}
function onNoteKey(e: KeyboardEvent) {
  if (isSaveKey(e)) {
    e.preventDefault()
    void addNote()
  } else if (e.key === 'Escape') {
    ;(e.target as HTMLElement).blur()
  }
}
const canNote = computed(() => !!x.value && canRun.value && canTakeNotes(x.value.status))
const mayEditNote = (n: ExploratoryNote) => !!x.value && x.value.status !== 'debriefed' && (n.created_by === me.value || canPlan.value) && auth.can(P.runExecute)

// edit
const editOpen = ref(false)
const editing = ref<ExploratoryNote | null>(null)
const editForm = ref({
  type: 'observation',
  body: '',
  file_ids: [] as string[],
})
function openEdit(n: ExploratoryNote) {
  editing.value = n
  editForm.value = { type: n.type, body: n.body, file_ids: [...n.file_ids] }
  editOpen.value = true
}
async function saveEdit() {
  if (!x.value || !editing.value) return
  try {
    const n = await api.patch<ExploratoryNote>(`/exploratory-sessions/${x.value.id}/notes/${editing.value.id}`, editForm.value)
    notes.value = notes.value.map((o) => (o.id === n.id ? n : o))
    editOpen.value = false
    notify.ok(t('exploratory.noteSaved'))
  } catch (e) {
    notify.fail(e)
  }
}
function removeNote(n: ExploratoryNote) {
  if (!x.value) return
  const sid = x.value.id
  notify.confirmDanger(t('exploratory.confirmDeleteNote'), async () => {
    try {
      await api.del(`/exploratory-sessions/${sid}/notes/${n.id}`)
      notes.value = notes.value.filter((o) => o.id !== n.id)
      notify.ok(t('exploratory.noteDeleted'))
    } catch (e) {
      notify.fail(e)
    }
  })
}

// raise a defect from a note
const defectOpen = ref(false)
const defectNote = ref<ExploratoryNote | null>(null)
const defectForm = ref({ title: '', severity: 'major', description: '' })
const defectBusy = ref(false)
const severityOptions = computed(() =>
  DEFECT_SEVERITIES.map((v) => ({
    value: v,
    label: t(`defectsView.severity.${v}`),
  })),
)
function openDefect(n: ExploratoryNote) {
  if (!x.value) return
  defectNote.value = n
  const charter = x.value.mission ? `\n${t('exploratory.defectCharter')}: ${x.value.mission}` : ''
  defectForm.value = {
    title: noteDefectTitle(n.body),
    severity: n.type === 'bug' ? 'major' : 'minor',
    description: `${n.body.trim()}\n\n---\n${t('exploratory.defectFoundIn', { key: x.value.key, title: x.value.title, time: formatClock(n.elapsed_seconds) })}${charter}`,
  }
  defectOpen.value = true
}
async function raiseDefect() {
  if (!x.value || !defectNote.value) return
  defectBusy.value = true
  try {
    const d = await api.post<{ id: string; key: string }>(`/exploratory-sessions/${x.value.id}/notes/${defectNote.value.id}/defect`, defectForm.value)
    defectOpen.value = false
    notify.ok(t('exploratory.defectRaised'), d.key)
    await load()
  } catch (e) {
    notify.fail(e)
  } finally {
    defectBusy.value = false
  }
}

// ---------- keyboard ----------
const helpOpen = ref(false)
const announce = ref('')
function onKey(e: KeyboardEvent) {
  if ((e.target as Element | null)?.closest?.('.p-dialog, .p-overlay-mask, .p-select-overlay, .p-multiselect-overlay')) return
  const a = sessionAction(e)
  if (!a) return
  if (a.type === 'help') {
    e.preventDefault()
    helpOpen.value = !helpOpen.value
    return
  }
  if (a.type === 'timer') {
    if (!canRun.value) return
    e.preventDefault()
    toggleTimer()
    return
  }
  if (!canNote.value) return
  e.preventDefault()
  if (a.noteType) {
    noteType.value = a.noteType
    announce.value = t('exploratory.typeChosen', {
      type: noteLabel(a.noteType),
    })
  }
  focusNote()
}
const shortcutHelp = computed(() =>
  SESSION_SHORTCUTS.map((s) => ({
    keys: s.keys,
    text: t(`exploratory.keys.${s.action}`),
  })),
)

// ---------- debrief ----------
const debriefOpen = ref(false)
const debriefForm = ref({
  summary: '',
  coverage_pct: 50 as number | null,
  setup_pct: 10 as number | null,
  testing_pct: 80 as number | null,
  bug_pct: 10 as number | null,
  outcome: 'passed',
  comment: '',
})
const debriefError = ref('')
const debriefBusy = ref(false)
const total = computed(() => splitTotal(debriefForm.value))
const outcomeOptions = computed(() => OUTCOMES.map((v) => ({ value: v, label: outcomeLabel(v) })))
const isReviewer = computed(() => !!x.value && !!x.value.tester && x.value.tester !== me.value)
function openDebrief() {
  if (!x.value || !canRun.value) return
  const d = x.value.debrief
  const s = suggestSplit(x.value.note_types, x.value.note_count)
  debriefForm.value = d
    ? {
        summary: d.summary,
        coverage_pct: d.coverage_pct,
        setup_pct: d.setup_pct,
        testing_pct: d.testing_pct,
        bug_pct: d.bug_pct,
        outcome: d.outcome,
        comment: d.comment,
      }
    : {
        summary: '',
        coverage_pct: 50,
        ...s,
        outcome: x.value.bug_count > 0 ? 'failed' : 'passed',
        comment: '',
      }
  debriefError.value = ''
  debriefOpen.value = true
}
async function saveDebrief() {
  if (!x.value) return
  debriefBusy.value = true
  debriefError.value = ''
  try {
    setSession(await api.put<ExploratorySession>(`/exploratory-sessions/${x.value.id}/debrief`, debriefBody(debriefForm.value)))
    debriefOpen.value = false
    notify.ok(t('exploratory.debriefed'), x.value?.key)
  } catch (e) {
    debriefError.value = errorMessage(e)
  } finally {
    debriefBusy.value = false
  }
}
function reopen() {
  if (!x.value) return
  const id = x.value.id
  notify.confirmDanger(
    t('exploratory.confirmReopen'),
    async () => {
      try {
        setSession(await api.del<ExploratorySession>(`/exploratory-sessions/${id}/debrief`))
        notify.ok(t('exploratory.reopened'))
      } catch (e) {
        notify.fail(e)
      }
    },
    t('exploratory.reopenHeader'),
    t('exploratory.reopen'),
  )
}

// ---------- report ----------
const reportBusy = ref('')
async function report(format: 'md' | 'pdf') {
  if (!x.value) return
  reportBusy.value = format
  try {
    await api.download(`/exploratory-sessions/${x.value.id}/report`, { format }, `${x.value.key}.${format}`)
  } catch (e) {
    notify.fail(e)
  } finally {
    reportBusy.value = ''
  }
}

const groups = computed(() => (x.value ? scopeGroups(x.value.scope) : []))
const scopeRoute = (type: string, id: string) =>
  type === 'requirement'
    ? {
        name: 'requirements',
        params: { projectId: proj.pid },
        query: { item: id },
      }
    : type === 'test'
      ? { name: 'tests', params: { projectId: proj.pid }, query: { item: id } }
      : {
          name: 'requirements',
          params: { projectId: proj.pid },
          query: { folder: id },
        }
const historyOpen = ref(false)
const back = () => router.push({ name: 'exploratory', params: { projectId: proj.pid } })
</script>

<template>
  <div v-if="loading" class="empty" aria-busy="true">
    <ProgressSpinner style="width: 40px; height: 40px" :aria-label="t('common.loading')" />
  </div>
  <Message v-else-if="error" severity="error" :closable="false">{{ error }}</Message>
  <template v-else-if="x">
    <div class="page-header">
      <div>
        <nav class="small" :aria-label="t('nav.breadcrumb')">
          <RouterLink :to="{ name: 'exploratory', params: { projectId: proj.pid } }">{{ t('exploratory.title') }}</RouterLink>
        </nav>
        <h1>
          <span class="key" style="margin-right: 6px">{{ x.key }}</span
          >{{ x.title }}
        </h1>
        <div class="row small" style="gap: 6px">
          <Tag :value="statusLabel(x.status)" :severity="statusSeverity(x.status)" :icon="statusIcon(x.status)" />
          <Tag v-if="x.debrief" :value="outcomeLabel(x.debrief.outcome)" :severity="outcomeSeverity(x.debrief.outcome)" />
          <span class="muted">{{
            t('exploratory.testerIs', {
              tester: x.tester || t('exploratory.unassigned'),
            })
          }}</span>
        </div>
      </div>
      <div class="actions">
        <HelpLink page="exploratory-testing" />
        <Button v-if="canExport" :label="t('exploratory.reportMd')" icon="pi pi-file" size="small" severity="secondary" outlined :loading="reportBusy === 'md'" @click="report('md')" />
        <Button v-if="canExport" :label="t('exploratory.reportPdf')" icon="pi pi-file-pdf" size="small" severity="secondary" outlined :loading="reportBusy === 'pdf'" @click="report('pdf')" />
        <Button icon="pi pi-arrow-left" :label="t('exploratory.back')" size="small" text severity="secondary" @click="back" />
      </div>
    </div>
    <div class="sr-only" role="status" aria-live="polite">{{ announce }}</div>

    <div class="xs-layout">
      <div class="xs-main">
        <!-- timer -->
        <section class="card" aria-labelledby="xs-timer-h">
          <h2 id="xs-timer-h" class="plain-heading small">
            {{ t('exploratory.timer') }}
          </h2>
          <div class="row" style="gap: 16px; align-items: center">
            <div
              class="xs-clock mono"
              role="timer"
              :aria-label="
                t('exploratory.elapsedOf', {
                  time: formatClock(elapsed),
                  box: x.timebox_minutes,
                })
              "
            >
              {{ formatClock(elapsed) }}
            </div>
            <div class="grow" style="min-width: 160px">
              <ProgressBar :value="Math.min(100, Math.round(box.ratio * 100))" :show-value="false" style="height: 8px" :class="{ 'xs-over': box.over }" :aria-label="t('exploratory.timeboxUsed')" />
              <div class="small muted" style="margin-top: 4px">
                {{ t('exploratory.timeboxOf', { n: x.timebox_minutes }) }}
                <Tag v-if="box.over" :value="t('exploratory.overTime')" severity="warn" icon="pi pi-hourglass" style="margin-left: 4px" />
              </div>
            </div>
            <div v-if="canRun" class="row" style="gap: 6px">
              <Button
                v-if="actions.start"
                :label="x.status === 'planned' ? t('exploratory.start') : t('exploratory.resume')"
                icon="pi pi-play"
                :loading="timerBusy"
                aria-keyshortcuts="T"
                @click="timer('start')"
              />
              <Button v-if="actions.pause" :label="t('exploratory.pause')" icon="pi pi-pause" severity="secondary" :loading="timerBusy" aria-keyshortcuts="T" @click="timer('pause')" />
              <Button v-if="actions.stop" :label="t('exploratory.stop')" icon="pi pi-stop" severity="danger" outlined :disabled="timerBusy" @click="stop" />
            </div>
          </div>
          <p class="small muted" style="margin: 6px 0 0">
            <span v-if="x.started_at">{{ t('exploratory.startedAt', { at: formatDateTime(x.started_at) }) }}</span>
            <span v-if="x.ended_at">
              ·
              {{ t('exploratory.endedAt', { at: formatDateTime(x.ended_at) }) }}</span
            >
          </p>
        </section>

        <!-- note composer -->
        <section v-if="canNote" class="card" aria-labelledby="xs-compose-h" style="margin-top: 12px">
          <h2 id="xs-compose-h" class="plain-heading small">
            {{ t('exploratory.newNote') }}
          </h2>
          <div role="radiogroup" :aria-label="t('exploratory.noteTypeLabel')" class="row" style="gap: 4px; margin-bottom: 6px">
            <Button
              v-for="o in noteTypeOptions"
              :key="o.value"
              role="radio"
              :aria-checked="noteType === o.value"
              :aria-keyshortcuts="o.key"
              size="small"
              :severity="noteType === o.value ? undefined : 'secondary'"
              :outlined="noteType !== o.value"
              :icon="noteIcon(o.value)"
              :label="`${o.label} (${o.key})`"
              @click="noteType = o.value"
            />
          </div>
          <label for="xs-note" class="sr-only">{{ t('exploratory.noteText', { type: noteLabel(noteType) }) }}</label>
          <Textarea
            id="xs-note"
            v-model="noteBody"
            rows="3"
            auto-resize
            style="width: 100%"
            :placeholder="t('exploratory.notePlaceholder')"
            aria-keyshortcuts="N"
            aria-describedby="xs-note-hint"
            @keydown="onNoteKey"
          />
          <div class="row" style="gap: 8px; margin-top: 6px">
            <MultiSelect
              v-if="fileOptions.length"
              v-model="noteFiles"
              :options="fileOptions"
              option-label="label"
              option-value="value"
              :placeholder="t('exploratory.noteFiles')"
              :aria-label="t('exploratory.noteFiles')"
              display="chip"
              style="min-width: 200px"
            />
            <small id="xs-note-hint" class="muted grow">{{ t('exploratory.noteHint') }}</small>
            <Button :label="t('exploratory.addNote')" icon="pi pi-plus" :loading="noteBusy" :disabled="!noteBody.trim()" aria-keyshortcuts="Control+Enter" @click="addNote" />
          </div>
          <div class="row small" style="margin-top: 6px">
            <Button
              :label="t('exploratory.shortcuts')"
              icon="pi pi-question-circle"
              text
              size="small"
              severity="secondary"
              :aria-expanded="helpOpen"
              aria-controls="xs-keys"
              aria-keyshortcuts="?"
              @click="helpOpen = !helpOpen"
            />
          </div>
          <div v-show="helpOpen" id="xs-keys" role="region" :aria-label="t('exploratory.shortcuts')">
            <dl class="xs-keys small">
              <template v-for="h in shortcutHelp" :key="h.text">
                <dt>
                  <kbd v-for="k in h.keys" :key="k">{{ k }}</kbd>
                </dt>
                <dd>{{ h.text }}</dd>
              </template>
            </dl>
          </div>
        </section>
        <Message v-else-if="x.status === 'planned' && canRun" severity="info" :closable="false" style="margin-top: 12px">{{ t('exploratory.startFirst') }}</Message>

        <!-- notes -->
        <section class="card" aria-labelledby="xs-notes-h" style="margin-top: 12px">
          <h2 id="xs-notes-h" class="plain-heading small">
            {{ t('exploratory.notesCount', { count: notes.length }) }}
          </h2>
          <p v-if="!notes.length" class="small muted">
            {{ t('exploratory.noNotes') }}
          </p>
          <ol v-else class="plain-list xs-notes">
            <li v-for="n in notes" :key="n.id">
              <div class="row nowrap" style="gap: 8px; align-items: flex-start">
                <span class="mono small muted" style="min-width: 52px">{{ formatClock(n.elapsed_seconds) }}</span>
                <Tag :value="noteLabel(n.type)" :severity="noteSeverity(n.type)" :icon="noteIcon(n.type)" />
                <div class="grow">
                  <div class="pre">{{ n.body }}</div>
                  <div v-if="n.files.length" class="row small" style="gap: 6px; margin-top: 2px">
                    <a v-for="f in n.files" :key="f.id" :href="contentUrl(f.id)" target="_blank" rel="noopener"><i class="pi pi-image" aria-hidden="true" /> {{ f.name }}</a>
                  </div>
                  <div class="small muted">{{ n.created_by }} · {{ formatDateTime(n.created_at) }}</div>
                </div>
                <RouterLink
                  v-if="n.defect_id"
                  :to="{
                    name: 'defects',
                    params: { projectId: proj.pid },
                    query: { defect: n.defect_id },
                  }"
                  class="key small"
                  >{{ n.defect_key }}</RouterLink
                >
                <Button
                  v-else-if="mayEditNote(n)"
                  icon="pi pi-exclamation-circle"
                  :label="t('exploratory.raiseDefect')"
                  size="small"
                  text
                  severity="danger"
                  :aria-label="
                    t('exploratory.raiseDefectOf', {
                      time: formatClock(n.elapsed_seconds),
                    })
                  "
                  @click="openDefect(n)"
                />
                <Button
                  v-if="mayEditNote(n)"
                  icon="pi pi-pencil"
                  text
                  rounded
                  size="small"
                  severity="secondary"
                  :aria-label="
                    t('exploratory.editNoteOf', {
                      time: formatClock(n.elapsed_seconds),
                    })
                  "
                  @click="openEdit(n)"
                />
                <Button
                  v-if="mayEditNote(n)"
                  icon="pi pi-trash"
                  text
                  rounded
                  size="small"
                  severity="danger"
                  :aria-label="
                    t('exploratory.deleteNoteOf', {
                      time: formatClock(n.elapsed_seconds),
                    })
                  "
                  @click="removeNote(n)"
                />
              </div>
            </li>
          </ol>
        </section>

        <!-- debrief -->
        <section v-if="x.status === 'completed' || x.status === 'debriefed'" class="card" aria-labelledby="xs-debrief-h" style="margin-top: 12px">
          <div class="row">
            <h2 id="xs-debrief-h" class="plain-heading small grow">
              {{ t('exploratory.debrief') }}
            </h2>
            <Button v-if="canRun" :label="x.debrief ? t('exploratory.editDebrief') : t('exploratory.doDebrief')" icon="pi pi-check-square" size="small" @click="openDebrief" />
            <Button v-if="x.debrief && canPlan" :label="t('exploratory.reopen')" icon="pi pi-undo" size="small" severity="secondary" outlined @click="reopen" />
          </div>
          <p v-if="!x.debrief" class="small muted">
            {{ t('exploratory.notDebriefed') }}
          </p>
          <dl v-else class="xs-fields">
            <dt>{{ t('exploratory.outcomeLabel') }}</dt>
            <dd>
              <Tag :value="outcomeLabel(x.debrief.outcome)" :severity="outcomeSeverity(x.debrief.outcome)" />
            </dd>
            <dt>{{ t('exploratory.summary') }}</dt>
            <dd class="pre">{{ x.debrief.summary || '—' }}</dd>
            <dt>{{ t('exploratory.coverage') }}</dt>
            <dd>{{ x.debrief.coverage_pct }}%</dd>
            <dt>{{ t('exploratory.split') }}</dt>
            <dd>
              <div
                class="xs-split"
                role="img"
                :aria-label="
                  t('exploratory.splitOf', {
                    setup: x.debrief.setup_pct,
                    testing: x.debrief.testing_pct,
                    bug: x.debrief.bug_pct,
                  })
                "
              >
                <span class="s" :style="{ width: x.debrief.setup_pct + '%' }" />
                <span class="t" :style="{ width: x.debrief.testing_pct + '%' }" />
                <span class="b" :style="{ width: x.debrief.bug_pct + '%' }" />
              </div>
              <div class="small">
                {{
                  t('exploratory.splitOf', {
                    setup: x.debrief.setup_pct,
                    testing: x.debrief.testing_pct,
                    bug: x.debrief.bug_pct,
                  })
                }}
              </div>
            </dd>
            <dt>
              {{ x.debrief.reviewed ? t('exploratory.reviewedBy') : t('exploratory.debriefedBy') }}
            </dt>
            <dd>
              {{ x.debrief.name || x.debrief.by }} ·
              {{ formatDateTime(x.debrief.at) }}
              <div v-if="x.debrief.comment" class="pre small">
                {{ x.debrief.comment }}
              </div>
            </dd>
          </dl>
        </section>
      </div>

      <aside class="xs-side">
        <section class="card" aria-labelledby="xs-charter-h">
          <h2 id="xs-charter-h" class="plain-heading small">
            {{ t('exploratory.charter') }}
          </h2>
          <p class="pre" style="margin: 0 0 8px">
            {{ x.mission || t('exploratory.noMission') }}
          </p>
          <div v-for="g in groups" :key="g.type" style="margin-top: 6px">
            <h3 class="plain-heading small muted" style="margin: 0 0 2px">
              {{ t(`exploratory.scopeType.${g.type}`) }}
            </h3>
            <div v-for="s in g.items" :key="s.id">
              <ItemRef v-if="g.type !== 'folder'" :to="scopeRoute(g.type, s.id)" :item-key="s.key ?? ''" :title="s.title ?? ''" title-class="ellipsis" />
              <span v-else class="small"><i class="pi pi-folder" aria-hidden="true" /> {{ s.title }}</span>
            </div>
          </div>
          <p v-if="!groups.length" class="small muted">
            {{ t('exploratory.noScope') }}
          </p>
          <div v-if="x.defects?.length" style="margin-top: 8px">
            <h3 class="plain-heading small muted" style="margin: 0 0 2px">
              {{ t('exploratory.defects') }}
            </h3>
            <div v-for="d in x.defects" :key="d.entity_id">
              <ItemRef
                :to="{
                  name: 'defects',
                  params: { projectId: proj.pid },
                  query: { defect: d.entity_id },
                }"
                :item-key="d.key ?? ''"
                :title="d.title ?? ''"
                title-class="ellipsis"
              />
            </div>
          </div>
        </section>
        <section class="card" aria-labelledby="xs-files-h" style="margin-top: 12px">
          <h2 id="xs-files-h" class="plain-heading small">
            {{ t('exploratory.files') }}
          </h2>
          <AttachmentsPanel :key="attachmentsKey" type="exploratory_session" :entity-id="x.id" :project-id="proj.pid" :can-write="canRun && x.status !== 'debriefed'" @count="loadAttachments" />
        </section>
        <section class="card" aria-labelledby="xs-hist-h" style="margin-top: 12px">
          <div class="row">
            <h2 id="xs-hist-h" class="plain-heading small grow">
              {{ t('exploratory.history') }}
            </h2>
            <Button :label="historyOpen ? t('exploratory.hide') : t('exploratory.show')" size="small" text :aria-expanded="historyOpen" aria-controls="xs-hist" @click="historyOpen = !historyOpen" />
          </div>
          <div v-if="historyOpen" id="xs-hist">
            <HistoryPanel entity="exploratory_session" :entity-id="x.id" :can-restore="false" />
          </div>
        </section>
      </aside>
    </div>

    <!-- edit note -->
    <Dialog v-model:visible="editOpen" :header="t('exploratory.editNote')" modal style="width: min(560px, 96vw)">
      <form class="grid" style="gap: 8px" @submit.prevent="saveEdit">
        <div v-field class="field">
          <label for="xs-en-type">{{ t('exploratory.noteTypeLabel') }}</label>
          <Select v-model="editForm.type" input-id="xs-en-type" :options="noteTypeOptions" option-label="label" option-value="value" />
        </div>
        <div v-field class="field">
          <label for="xs-en-body">{{ t('exploratory.noteTextPlain') }}</label
          ><Textarea id="xs-en-body" v-model="editForm.body" rows="4" auto-resize />
        </div>
        <div v-if="fileOptions.length" v-field class="field">
          <label for="xs-en-files">{{ t('exploratory.noteFiles') }}</label>
          <MultiSelect v-model="editForm.file_ids" input-id="xs-en-files" :options="fileOptions" option-label="label" option-value="value" display="chip" />
        </div>
        <div class="row" style="justify-content: flex-end">
          <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="editOpen = false" />
          <Button type="submit" :label="t('common.save')" icon="pi pi-check" :disabled="!editForm.body.trim()" />
        </div>
      </form>
    </Dialog>

    <!-- raise defect -->
    <Dialog v-model:visible="defectOpen" :header="t('exploratory.raiseDefect')" modal style="width: min(620px, 96vw)">
      <form class="grid" style="gap: 8px" @submit.prevent="raiseDefect">
        <p class="small muted" style="margin: 0">
          {{ t('exploratory.defectExplain') }}
        </p>
        <div v-field class="field">
          <label for="xs-d-title">{{ t('common.title') }}</label
          ><InputText id="xs-d-title" v-model="defectForm.title" autofocus />
        </div>
        <div v-field class="field">
          <label for="xs-d-sev">{{ t('exploratory.severity') }}</label>
          <Select v-model="defectForm.severity" input-id="xs-d-sev" :options="severityOptions" option-label="label" option-value="value" />
        </div>
        <div v-field class="field">
          <label for="xs-d-desc">{{ t('exploratory.description') }}</label
          ><Textarea id="xs-d-desc" v-model="defectForm.description" rows="6" auto-resize />
        </div>
        <div class="row" style="justify-content: flex-end">
          <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="defectOpen = false" />
          <Button type="submit" :label="t('exploratory.raiseDefect')" icon="pi pi-exclamation-circle" severity="danger" :loading="defectBusy" :disabled="!defectForm.title.trim()" />
        </div>
      </form>
    </Dialog>

    <!-- debrief -->
    <Dialog
      v-model:visible="debriefOpen"
      :header="t('exploratory.debriefHeader', { key: x.key })"
      modal
      maximizable
      :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }"
      style="width: min(680px, 96vw)"
    >
      <form class="grid" style="gap: 10px" @submit.prevent="saveDebrief">
        <Message v-if="debriefError" severity="error" :closable="false">{{ debriefError }}</Message>
        <Message v-if="isReviewer" severity="info" :closable="false">{{ t('exploratory.reviewerNote', { tester: x.tester }) }}</Message>
        <div v-field class="field">
          <label for="xs-db-sum">{{ t('exploratory.summary') }}</label
          ><Textarea id="xs-db-sum" v-model="debriefForm.summary" rows="4" auto-resize autofocus />
        </div>
        <div class="form-grid">
          <div v-field class="field">
            <label for="xs-db-out">{{ t('exploratory.outcomeLabel') }}</label>
            <Select v-model="debriefForm.outcome" input-id="xs-db-out" :options="outcomeOptions" option-label="label" option-value="value" aria-describedby="xs-db-out-hint" />
            <small id="xs-db-out-hint" class="muted">{{ t(`exploratory.outcomeHint.${debriefForm.outcome}`) }}</small>
          </div>
          <div v-field class="field">
            <label for="xs-db-cov">{{ t('exploratory.coverage') }}</label>
            <InputNumber v-model="debriefForm.coverage_pct" input-id="xs-db-cov" :min="0" :max="100" suffix=" %" />
          </div>
        </div>
        <fieldset class="xs-fieldset">
          <legend>{{ t('exploratory.split') }}</legend>
          <div class="form-grid">
            <div v-field class="field">
              <label for="xs-db-setup">{{ t('exploratory.setup') }}</label
              ><InputNumber v-model="debriefForm.setup_pct" input-id="xs-db-setup" :min="0" :max="100" suffix=" %" />
            </div>
            <div v-field class="field">
              <label for="xs-db-test">{{ t('exploratory.testing') }}</label
              ><InputNumber v-model="debriefForm.testing_pct" input-id="xs-db-test" :min="0" :max="100" suffix=" %" />
            </div>
            <div v-field class="field">
              <label for="xs-db-bug">{{ t('exploratory.bugInvestigation') }}</label
              ><InputNumber v-model="debriefForm.bug_pct" input-id="xs-db-bug" :min="0" :max="100" suffix=" %" />
            </div>
          </div>
          <p class="small" :class="total === 100 ? 'muted' : 'xs-error'" role="status">
            {{ t('exploratory.splitTotal', { total }) }}
          </p>
        </fieldset>
        <div v-field class="field">
          <label for="xs-db-com">{{ t('exploratory.reviewComment') }}</label
          ><Textarea id="xs-db-com" v-model="debriefForm.comment" rows="2" auto-resize />
        </div>
        <div class="row" style="justify-content: flex-end">
          <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="debriefOpen = false" />
          <Button type="submit" :label="t('exploratory.saveDebrief')" icon="pi pi-check" :loading="debriefBusy" :disabled="total !== 100" />
        </div>
      </form>
    </Dialog>
  </template>
</template>

<style scoped>
.xs-layout {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(260px, 1fr);
  gap: 12px;
  align-items: start;
}
@media (max-width: 900px) {
  .xs-layout {
    grid-template-columns: minmax(0, 1fr);
  }
}
.xs-clock {
  font-size: 2.2rem;
  font-weight: 600;
  min-width: 7ch;
}
.xs-over :deep(.p-progressbar-value) {
  background: var(--qtf-warn, #b45309);
}
.xs-notes li {
  padding: 6px 0;
  border-bottom: 1px solid var(--qtf-border);
}
.pre {
  white-space: pre-wrap;
}
.xs-keys {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 4px 12px;
  margin: 0;
}
.xs-keys dd {
  margin: 0;
}
.xs-keys kbd {
  border: 1px solid var(--qtf-border);
  border-radius: 4px;
  padding: 0 4px;
  margin-right: 2px;
  font-family: inherit;
}
.xs-fields {
  display: grid;
  grid-template-columns: minmax(120px, max-content) minmax(0, 1fr);
  gap: 6px 14px;
  margin: 0;
}
.xs-fields dt {
  font-weight: 600;
  color: var(--qtf-muted);
}
.xs-fields dd {
  margin: 0;
}
.xs-split {
  display: flex;
  height: 10px;
  border-radius: 4px;
  overflow: hidden;
  max-width: 320px;
  border: 1px solid var(--qtf-border);
}
.xs-split .s {
  background: #64748b;
}
.xs-split .t {
  background: #0d9488;
}
.xs-split .b {
  background: #dc2626;
}
.xs-fieldset {
  border: 1px solid var(--qtf-border);
  border-radius: 6px;
  padding: 8px 12px;
}
.xs-error {
  color: var(--p-red-600, #b91c1c);
}
</style>
