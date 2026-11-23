<script setup lang="ts">
import { useUndoStore } from '@/stores/undo'
import { useProjectStore } from '@/stores/project'
import { impactGateFor } from '@/composables/impact'
import type { ImpactDryRun } from '@/lib/correlations'
import { entry, recordUndo } from '@/lib/undoActions'
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ProgressSpinner from 'primevue/progressspinner'
import UserAvatar from './UserAvatar.vue'
import { usePeopleStore } from '@/stores/people'
import { useConfirm } from 'primevue/useconfirm'
import { api, asItems, errorMessage } from '@/lib/api'
import { applicabilityText, diffSnapshots, parseSnapshot } from '@/lib/diff'
import { formatDateTime, formatFullDateTime } from '@/lib/format'
import type { Revision } from '@/lib/types'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'

const props = defineProps<{
  entity: 'requirement' | 'test' | 'exploratory_session' | 'cycle' | 'environment' | 'test_data_set' | 'solution_preamble'
  entityId: string
  canRestore: boolean
  /** Current version of the item (restoring it would be a no-op). */
  currentVersion?: number
  /** Resolve ids (category_ids, folder_id) to names. */
  resolve?: (field: string, id: string) => string | undefined
  /** Labels of the fields (else fields.<name> or the name). */
  fieldLabels?: Record<string, string>
}>()
const emit = defineEmits<{ restored: [item: unknown] }>()
const notify = useNotify()
usePeopleStore().ensure()
const confirm = useConfirm()
const { t, te } = useI18n()
const fieldLabel = (f: string) => {
  if (f.startsWith('custom_fields.')) {
    const key = f.slice(14)
    return useProjectStore().customFields.find((x) => x.key === key)?.label ?? key
  }
  if (props.fieldLabels?.[f]) return props.fieldLabels[f]
  const k = `fields.${f}`
  return te(k) ? t(k) : f.replace(/_/g, ' ')
}

const revisions = ref<Revision[]>([])
const loading = ref(false)
const error = ref('')
const selectedId = ref<string | null>(null)
const restoring = ref(false)
const HISTORY_PATHS: Record<string, string> = { requirement: 'requirements', exploratory_session: 'exploratory-sessions', cycle: 'cycles', environment: 'environments', test_data_set: 'test-data-sets', test: 'tests' }
// the solution preamble's history is the project's (entityId = the project id)
const path = computed(() => (props.entity === 'solution_preamble' ? `/projects/${props.entityId}/solution-preamble` : `/${HISTORY_PATHS[props.entity] ?? 'tests'}/${props.entityId}`))

const undoStore = useUndoStore()
watch(
  () => undoStore.version,
  () => void load(),
)
async function load() {
  loading.value = true
  error.value = ''
  try {
    revisions.value = asItems(await api.get<Revision[]>(`${path.value}/history`)).sort((a, b) => b.version - a.version || b.created_at.localeCompare(a.created_at))
    if (!revisions.value.some((r) => r.id === selectedId.value)) selectedId.value = revisions.value[0]?.id ?? null
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => props.entityId, load, { immediate: true })
defineExpose({ reload: load })

const index = computed(() => revisions.value.findIndex((r) => r.id === selectedId.value))
const selected = computed(() => revisions.value[index.value] ?? null)
const previous = computed(() => (index.value >= 0 ? revisions.value[index.value + 1] ?? null : null))
const changes = computed(() => {
  const cur = selected.value
  if (!cur) return []
  const after = parseSnapshot(cur.snapshot)
  const before = previous.value ? parseSnapshot(previous.value.snapshot) : null
  // the first revision has nothing to compare with: list its fields as added
  // server-computed changed_fields are authoritative when present
  return diffSnapshots(before, after, before && Array.isArray(cur.changed_fields) ? cur.changed_fields : null, { resolve: props.resolve, applicability: (a) => applicabilityText(a, t('variants.allVariants')) })
})

const ACTION_SEVERITY: Record<string, string> = { create: 'success', update: 'info', restore: 'warn', move: 'secondary', bulk_update: 'secondary', import: 'secondary', applicability: 'contrast' }

function restore() {
  const r = selected.value
  if (!r) return
  confirm.require({
    header: t('history.restoreHeader', { version: r.version }),
    message: props.entity === 'requirement' ? t('history.restoreRequirement', { version: r.version }) : t('history.restoreTest', { version: r.version }),
    icon: 'pi pi-history',
    rejectProps: { label: t('common.cancel'), severity: 'secondary', outlined: true },
    acceptProps: { label: t('common.restore'), icon: 'pi pi-replay' },
    accept: async () => {
      restoring.value = true
      try {
        const current = revisions.value[0]
        const p = path.value
        // what restoring the version affects: preview, then the chosen handling (impact gate)
        const params = await impactGateFor(useProjectStore().pid, (q) => api.post<ImpactDryRun>(`${p}/restore`, { revision_id: r.id }, { params: q }))
        if (!params) return
        const item = await api.post(`${p}/restore`, { revision_id: r.id }, { params })
        if (current && current.id !== r.id) {
          const key = (item as { key?: string } | undefined)?.key ?? ''
          recordUndo(entry(t('undo.a.restoreVersion', { version: r.version, key }), () => api.post(`${p}/restore`, { revision_id: current.id }), () => api.post(`${p}/restore`, { revision_id: r.id })))
        }
        notify.ok(t('history.restored'), t('common.versionShort', { version: r.version }))
        emit('restored', item)
        await load()
      } catch (e) {
        notify.fail(e, t('history.restoreFailed'))
      } finally {
        restoring.value = false
      }
    },
  })
}
</script>

<template>
  <Message v-if="error" severity="error" :closable="false">{{ t('history.unavailable', { error }) }}</Message>
  <div v-else-if="loading && !revisions.length" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
  <div v-else-if="!revisions.length" class="muted small">{{ t('history.none') }}</div>
  <div v-else class="history">
    <div class="rev-list" role="listbox" :aria-label="t('history.revisions')">
      <button v-for="r in revisions" :key="r.id" :class="{ active: r.id === selectedId }" role="option" :aria-selected="r.id === selectedId" @click="selectedId = r.id">
        <div class="row nowrap" style="gap: 6px">
          <b>v{{ r.version }}</b>
          <Tag :value="enumLabel('historyAction', r.action)" :severity="ACTION_SEVERITY[r.action] ?? 'secondary'" />
          <span class="grow" />
          <span class="small muted" :title="formatFullDateTime(r.created_at)">{{ formatDateTime(r.created_at) }}</span>
        </div>
        <div class="small muted row nowrap" style="gap: 6px; min-width: 0"><UserAvatar v-if="r.user_email" :avatar="r.user_avatar" :email="r.user_email" :name="r.user_name || r.user_email" :size="18" /><span class="ellipsis" :title="r.user_email">{{ r.user_name || r.user_email || t('common.system') }}</span></div>
        <div v-if="r.changed_fields?.length">
          <span v-for="f in r.changed_fields.slice(0, 6)" :key="f" class="field-chip">{{ fieldLabel(f) }}</span>
          <span v-if="r.changed_fields.length > 6" class="small muted">+{{ r.changed_fields.length - 6 }}</span>
        </div>
      </button>
    </div>
    <div v-if="selected" class="grid" style="gap: 10px">
      <div class="row">
        <b>{{ t('history.version', { version: selected.version }) }}</b>
        <span class="muted small row nowrap" style="gap: 6px"><UserAvatar v-if="selected.user_email" :avatar="selected.user_avatar" :email="selected.user_email" :name="selected.user_name || selected.user_email" :size="20" />{{ t('history.byAt', { action: enumLabel('historyAction', selected.action), user: selected.user_name || selected.user_email || t('common.system'), date: formatFullDateTime(selected.created_at) }) }}</span>
        <span class="grow" />
        <Button
          v-if="canRestore && index > 0"
          :label="t('history.restore')"
          icon="pi pi-replay"
          size="small"
          severity="secondary"
          outlined
          :loading="restoring"
          @click="restore"
        />
        <span v-else-if="index === 0" class="small muted">{{ t('history.current') }}</span>
      </div>
      <div class="small muted">{{ previous ? t('history.comparedWith', { version: previous.version }) : t('history.initial') }}</div>
      <div v-if="!changes.length" class="muted small">{{ t('history.noChanges') }}</div>
      <div v-for="c in changes" :key="c.field" class="field">
        <span class="field-label">{{ fieldLabel(c.field) }}</span>
        <div v-if="c.segments" class="diff">
          <template v-for="(s, i) in c.segments" :key="i">
            <ins v-if="s.op === 'insert'">{{ s.text }}</ins>
            <del v-else-if="s.op === 'delete'">{{ s.text }}</del>
            <template v-else>{{ s.text }}</template>
          </template>
          <span v-if="!c.segments.length" class="muted">{{ t('history.empty') }}</span>
        </div>
        <div v-else class="row small">
          <span v-if="c.before" class="diff-before">{{ c.before }}</span>
          <span v-else class="muted">{{ t('history.empty') }}</span>
          <i class="pi pi-arrow-right muted" style="font-size: 10px" />
          <span v-if="c.after" class="diff-after">{{ c.after }}</span>
          <span v-else class="muted">{{ t('history.empty') }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
