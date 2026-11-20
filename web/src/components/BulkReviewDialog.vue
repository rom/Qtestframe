<script setup lang="ts">
// Bulk review: approve or reject many requirements, or mark many tests
// ready. Shows a preview (what changes, what is skipped and why) before
// anything is saved.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import ProgressSpinner from 'primevue/progressspinner'
import RadioButton from 'primevue/radiobutton'
import SelectButton from 'primevue/selectbutton'
import Textarea from 'primevue/textarea'
import StatusTag from './StatusTag.vue'
import { api, errorMessage } from '@/lib/api'
import { enumLabel, enumOptions } from '@/i18n/labels'
import { DEFAULT_FROM, decisionsFor, fromOptions, groupSkipped, ignorableChecks, scopeBody, undoGroups, type BulkReviewResult, type BulkScope, type ReviewKind } from '@/lib/bulkReview'
import { entry, recordUndo } from '@/lib/undoActions'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ kind: ReviewKind; projectId: string; selected: string[]; shown: string[] }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ done: [] }>()
const { t } = useI18n()
const notify = useNotify()

const scope = ref<BulkScope>('selected')
const decision = ref('')
const from = ref<string[]>([...DEFAULT_FROM])
const ignore = ref<string[]>([])
const comment = ref('')
const preview = ref<BulkReviewResult | null>(null)
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const openReasons = ref<Record<string, boolean>>({})

const path = computed(() => `/projects/${props.projectId}/${props.kind === 'requirement' ? 'requirements' : 'tests'}/review`)
const decisions = computed(() => decisionsFor(props.kind).map((d) => ({ label: t(`bulkReview.decision.${d}`), value: d })))
const fromOpts = computed(() => enumOptions('status', fromOptions(props.kind, decision.value)))
const checkOpts = computed(() => ignorableChecks(props.kind).map((c) => ({ label: t(`bulkReview.reason.${c}`), value: c })))
const scopes = computed(() =>
  [
    { value: 'selected' as const, n: props.selected.length },
    { value: 'shown' as const, n: props.shown.length },
    { value: 'all' as const, n: -1 },
  ].filter((s) => s.n !== 0),
)
const skippedGroups = computed(() => groupSkipped(preview.value?.skipped ?? []))
const count = computed(() => preview.value?.updated.length ?? 0)
const kindLabel = computed(() => (props.kind === 'requirement' ? 'requirements' : 'tests'))

watch(visible, (v) => {
  if (!v) return
  scope.value = props.selected.length ? 'selected' : 'shown'
  decision.value = decisionsFor(props.kind)[0]
  from.value = [...DEFAULT_FROM]
  ignore.value = []
  comment.value = ''
  openReasons.value = {}
  void load()
})

let seq = 0
async function load() {
  if (!visible.value || !decision.value) return
  const mine = ++seq
  loading.value = true
  error.value = ''
  try {
    const res = await api.post<BulkReviewResult>(path.value, { decision: decision.value, from: from.value, ignore: ignore.value, dry_run: true, ...scopeBody(scope.value, props.selected, props.shown) })
    if (mine === seq) preview.value = res
  } catch (e) {
    if (mine === seq) {
      preview.value = null
      error.value = errorMessage(e)
    }
  } finally {
    if (mine === seq) loading.value = false
  }
}
watch([scope, decision, from, ignore], load, { deep: true })
watch(decision, (d) => (from.value = from.value.filter((s) => s !== d)))

async function apply() {
  if (!preview.value || !count.value) return
  saving.value = true
  try {
    const res = await api.post<BulkReviewResult>(path.value, {
      decision: decision.value,
      ids: preview.value.updated.map((u) => u.id),
      from: from.value,
      ignore: ignore.value,
      comment: comment.value,
    })
    const groups = undoGroups(res.updated)
    const bulk = `/projects/${props.projectId}/${props.kind === 'requirement' ? 'requirements' : 'tests'}/bulk`
    const target = decision.value
    recordUndo(
      entry(
        t('bulkReview.undoLabel', { n: res.updated.length, status: enumLabel('status', target) }),
        async () => {
          for (const g of groups) await api.post(bulk, { ids: g.ids, set: { status: g.status } })
        },
        () => api.post(path.value, { decision: target, ids: res.updated.map((u) => u.id), from: groups.map((g) => g.status), ignore: ignorableChecks(props.kind) }),
      ),
    )
    notify.ok(t('bulkReview.done', { n: res.updated.length, status: enumLabel('status', target) }, res.updated.length), res.skipped.length ? t('bulkReview.skippedAfter', { n: res.skipped.length }, res.skipped.length) : undefined)
    visible.value = false
    emit('done')
  } catch (e) {
    notify.fail(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t(`bulkReview.title.${kind}`)" :style="{ width: 'min(760px, 96vw)' }">
    <div class="form">
      <fieldset class="scope">
        <legend>{{ t('bulkReview.scope') }}</legend>
        <div v-for="s in scopes" :key="s.value" class="row">
          <RadioButton v-model="scope" :input-id="`br-scope-${s.value}`" :value="s.value" name="br-scope" />
          <label :for="`br-scope-${s.value}`">{{ s.value === 'all' ? t(`bulkReview.scopeAll.${kindLabel}`) : t(`bulkReview.scope_${s.value}`, { n: s.n }, s.n) }}</label>
        </div>
      </fieldset>
      <div class="grid">
        <div>
          <span id="br-decision" class="lbl">{{ t('bulkReview.decisionLabel') }}</span>
          <SelectButton v-model="decision" :options="decisions" option-label="label" option-value="value" :allow-empty="false" aria-labelledby="br-decision" />
        </div>
        <div>
          <label for="br-from" class="lbl">{{ t('bulkReview.from') }}</label>
          <MultiSelect v-model="from" input-id="br-from" :options="fromOpts" option-label="label" option-value="value" display="chip" fluid />
        </div>
      </div>
      <div>
        <label for="br-ignore" class="lbl">{{ t('bulkReview.ignore') }}</label>
        <MultiSelect v-model="ignore" input-id="br-ignore" :options="checkOpts" option-label="label" option-value="value" display="chip" fluid :placeholder="t('bulkReview.ignoreNone')" aria-describedby="br-ignore-help" />
        <small id="br-ignore-help" class="muted">{{ t('bulkReview.ignoreHelp') }}</small>
      </div>
      <div>
        <label for="br-comment" class="lbl">{{ t('bulkReview.comment') }}</label>
        <Textarea id="br-comment" v-model="comment" rows="2" auto-resize fluid aria-describedby="br-comment-help" />
        <small id="br-comment-help" class="muted">{{ t('bulkReview.commentHelp') }}</small>
      </div>

      <section class="preview" aria-live="polite" :aria-busy="loading">
        <h3 class="plain-heading">{{ t('bulkReview.preview') }}</h3>
        <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
        <div v-else-if="loading && !preview" class="center"><ProgressSpinner style="width: 28px; height: 28px" :aria-label="t('common.loading')" /></div>
        <template v-else-if="preview">
          <p class="summary">
            <b>{{ t('bulkReview.willChange', { n: count, status: enumLabel('status', decision) }, count) }}</b>
            <span v-if="preview.skipped.length" class="muted"> · {{ t('bulkReview.willSkip', { n: preview.skipped.length }, preview.skipped.length) }}</span>
          </p>
          <ul v-if="count" class="items">
            <li v-for="it in preview.updated.slice(0, 50)" :key="it.id" class="row">
              <span class="key">{{ it.key }}</span>
              <span class="grow ellipsis">{{ it.title }}</span>
              <StatusTag :value="it.status" />
              <i class="pi pi-arrow-right muted small" aria-hidden="true" />
              <StatusTag :value="decision" />
            </li>
            <li v-if="count > 50" class="small muted">{{ t('bulkReview.more', { n: count - 50 }) }}</li>
          </ul>
          <div v-for="g in skippedGroups" :key="g.reason" class="skip">
            <button type="button" class="linklike" :aria-expanded="!!openReasons[g.reason]" @click="openReasons[g.reason] = !openReasons[g.reason]">
              <i :class="openReasons[g.reason] ? 'pi pi-chevron-down' : 'pi pi-chevron-right'" aria-hidden="true" />
              {{ t(`bulkReview.reason.${g.reason}`) }} ({{ g.items.length }})
            </button>
            <ul v-if="openReasons[g.reason]" class="items">
              <li v-for="it in g.items.slice(0, 100)" :key="it.id" class="row">
                <span class="key">{{ it.key }}</span>
                <span class="grow ellipsis">{{ it.title }}</span>
                <StatusTag :value="it.status" />
              </li>
            </ul>
          </div>
        </template>
      </section>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" severity="secondary" text @click="visible = false" />
      <Button
        :label="count ? t('bulkReview.apply', { n: count, status: enumLabel('status', decision) }, count) : t('bulkReview.nothingToDo')"
        :icon="decision === 'rejected' ? 'pi pi-times' : 'pi pi-check'"
        :severity="decision === 'rejected' ? 'danger' : 'success'"
        :disabled="!count || loading"
        :loading="saving"
        @click="apply"
      />
    </template>
  </Dialog>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.scope {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 6px 12px 10px;
}
.scope .row {
  gap: 8px;
  padding: 3px 0;
  flex-wrap: nowrap;
  align-items: flex-start;
}
.scope .row :deep(.p-radiobutton) {
  flex-shrink: 0;
  margin-top: 2px;
}
.grid {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 12px;
}
@media (max-width: 600px) {
  .grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
.lbl {
  display: block;
  font-weight: 600;
  font-size: 13px;
  margin-bottom: 4px;
}
.preview {
  border-top: 1px solid var(--qtf-border);
  padding-top: 8px;
}
.preview h3 {
  font-size: 14px;
  margin: 0 0 6px;
}
.summary {
  margin: 0 0 6px;
}
.items {
  list-style: none;
  margin: 0 0 8px;
  padding: 0;
  max-height: 240px;
  overflow: auto;
}
.items li {
  gap: 8px;
  padding: 3px 0;
  min-width: 0;
}
.ellipsis {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.linklike {
  border: 0;
  padding: 4px 0;
  background: none;
  color: var(--p-primary-color);
  cursor: pointer;
  font: inherit;
}
.center {
  display: flex;
  justify-content: center;
  padding: 12px;
}
</style>
