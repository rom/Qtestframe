<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ProgressSpinner from 'primevue/progressspinner'
import { useConfirm } from 'primevue/useconfirm'
import { api, asItems, errorMessage } from '@/lib/api'
import { diffSnapshots, parseSnapshot } from '@/lib/diff'
import { formatDateTime, formatFullDateTime } from '@/lib/format'
import type { Revision } from '@/lib/types'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'

const props = defineProps<{
  entity: 'requirement' | 'test'
  entityId: string
  canRestore: boolean
  /** Current version of the item (restoring it would be a no-op). */
  currentVersion?: number
  /** Resolve ids (category_ids, folder_id) to names. */
  resolve?: (field: string, id: string) => string | undefined
}>()
const emit = defineEmits<{ restored: [item: unknown] }>()
const notify = useNotify()
const confirm = useConfirm()
const { t, te } = useI18n()
const fieldLabel = (f: string) => {
  const k = `fields.${f}`
  return te(k) ? t(k) : f.replace(/_/g, ' ')
}

const revisions = ref<Revision[]>([])
const loading = ref(false)
const error = ref('')
const selectedId = ref<string | null>(null)
const restoring = ref(false)
const path = computed(() => `/${props.entity === 'requirement' ? 'requirements' : 'tests'}/${props.entityId}`)

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
  return diffSnapshots(before, after, before && Array.isArray(cur.changed_fields) ? cur.changed_fields : null, { resolve: props.resolve })
})

const ACTION_SEVERITY: Record<string, string> = { create: 'success', update: 'info', restore: 'warn', move: 'secondary', bulk_update: 'secondary', import: 'secondary' }

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
        const item = await api.post(`${path.value}/restore`, { revision_id: r.id })
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
  <div v-else-if="loading && !revisions.length" class="empty"><ProgressSpinner style="width: 30px; height: 30px" /></div>
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
        <div class="small muted ellipsis">{{ r.user_email || t('common.system') }}</div>
        <div v-if="r.changed_fields?.length">
          <span v-for="f in r.changed_fields.slice(0, 6)" :key="f" class="field-chip">{{ fieldLabel(f) }}</span>
          <span v-if="r.changed_fields.length > 6" class="small muted">+{{ r.changed_fields.length - 6 }}</span>
        </div>
      </button>
    </div>
    <div v-if="selected" class="grid" style="gap: 10px">
      <div class="row">
        <b>{{ t('history.version', { version: selected.version }) }}</b>
        <span class="muted small">{{ t('history.byAt', { action: enumLabel('historyAction', selected.action), user: selected.user_email || t('common.system'), date: formatFullDateTime(selected.created_at) }) }}</span>
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
        <label>{{ fieldLabel(c.field) }}</label>
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
