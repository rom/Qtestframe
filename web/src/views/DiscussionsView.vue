<script setup lang="ts">
// The project's discussions: its threads (open by default) on requirements,
// tests, defects, risks, runs and review requests, filtered by status,
// "I took part" / "mention me", kind of item and text. Opening a thread
// opens its item with the discussion.
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import ItemRef from '@/components/ItemRef.vue'
import UserAvatar from '@/components/UserAvatar.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDateTime, formatFullDateTime } from '@/lib/format'
import { threadTarget } from '@/lib/discussions'
import { commentText } from '@/lib/commentMarkdown'
import type { DiscussionThread } from '@/lib/types'

const props = defineProps<{ projectId?: string }>()
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const pid = computed(() => props.projectId ?? String(route.params.projectId ?? ''))

const KINDS = ['requirement', 'test', 'defect', 'risk', 'run', 'review'] as const
const q0 = route.query
const status = ref<string>(['open', 'resolved', 'all'].includes(String(q0.status)) ? String(q0.status) : 'open')
const show = ref<string>(['mine', 'mentioned'].includes(String(q0.filter)) ? String(q0.filter) : '')
const kind = ref<string>((KINDS as readonly string[]).includes(String(q0.type)) ? String(q0.type) : '')
const text = ref(typeof q0.q === 'string' ? q0.q : '')
const list = ref<DiscussionThread[]>([])
const loading = ref(false)
const error = ref('')

const statuses = computed(() => [
  { label: t('discussions.view.open'), value: 'open' },
  { label: t('discussions.view.resolved'), value: 'resolved' },
  { label: t('discussions.view.all'), value: 'all' },
])
const shows = computed(() => [
  { label: t('discussions.view.everyone'), value: '' },
  { label: t('discussions.view.mine'), value: 'mine' },
  { label: t('discussions.view.mentioned'), value: 'mentioned' },
])
const kinds = computed(() => [{ label: t('discussions.view.anyType'), value: '' }, ...KINDS.map((k) => ({ label: t(`discussions.view.kind.${k}`), value: k }))])

let seq = 0
async function load() {
  const n = ++seq
  loading.value = true
  error.value = ''
  try {
    const params: Record<string, string> = { status: status.value }
    if (show.value) params.filter = show.value
    if (kind.value) params.type = kind.value
    if (text.value.trim()) params.q = text.value.trim()
    const res = asItems(await api.get<{ items: DiscussionThread[] }>(`/projects/${pid.value}/discussions`, params))
    if (n === seq) list.value = res
  } catch (e) {
    if (n === seq) error.value = errorMessage(e)
  } finally {
    if (n === seq) loading.value = false
  }
}
onMounted(load)
let timer: ReturnType<typeof setTimeout> | undefined
watch([status, show, kind], () => {
  syncQuery()
  void load()
})
watch(text, () => {
  clearTimeout(timer)
  timer = setTimeout(() => {
    syncQuery()
    void load()
  }, 300)
})
function syncQuery() {
  const query: Record<string, string> = {}
  if (status.value !== 'open') query.status = status.value
  if (show.value) query.filter = show.value
  if (kind.value) query.type = kind.value
  if (text.value.trim()) query.q = text.value.trim()
  void router.replace({ query })
}

const author = (th: DiscussionThread) => th.root.user_name || th.root.user_email || t('common.unknown')
const excerpt = (th: DiscussionThread) => {
  const s = th.root.deleted ? t('discussions.removed') : commentText(th.root.body)
  return s.length > 160 ? s.slice(0, 159) + '…' : s
}
</script>

<template>
  <section>
    <div class="page-header">
      <div>
        <h1>{{ t('discussions.view.title') }}</h1>
        <div class="sub">{{ t('discussions.view.intro') }}</div>
      </div>
    </div>
    <div class="card">
      <div class="row toolbar" style="gap: 8px; flex-wrap: wrap; margin-bottom: 8px">
        <SelectButton v-model="status" :options="statuses" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('discussions.view.status')" />
        <SelectButton v-model="show" :options="shows" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('discussions.view.show')" />
        <Select v-model="kind" :options="kinds" option-label="label" option-value="value" :aria-label="t('discussions.view.type')" style="min-width: 160px" />
        <InputText v-model="text" type="search" :placeholder="t('discussions.view.search')" :aria-label="t('discussions.view.search')" style="min-width: 200px" />
      </div>
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <DataTable :value="list" :loading="loading" data-key="root.id" size="small" striped-rows>
        <Column :header="t('discussions.view.item')" style="min-width: 220px">
          <template #body="{ data }">
            <div class="row" style="gap: 6px; flex-wrap: wrap">
              <Tag :value="t(`discussions.view.kind.${data.root.entity_type}`)" severity="secondary" />
              <ItemRef :to="threadTarget(data)" :item-key="data.entity_key" :title="data.entity_title || t('discussions.view.open_item')" title-class="title" />
            </div>
          </template>
        </Column>
        <Column :header="t('discussions.view.thread')">
          <template #body="{ data }">
            <div class="row" style="gap: 6px; align-items: flex-start">
              <UserAvatar :avatar="data.root.user_avatar ?? ''" :user-id="data.root.user_id" :email="data.root.user_email" :name="author(data)" :size="22" />
              <div style="min-width: 0">
                <div class="small"><b>{{ author(data) }}</b> <Tag v-if="data.root.resolved_at" severity="success" :value="t('discussions.resolved')" /></div>
                <div class="small" :class="{ muted: data.root.deleted }">{{ excerpt(data) }}</div>
              </div>
            </div>
          </template>
        </Column>
        <Column field="replies" :header="t('discussions.view.replies')" style="width: 90px" />
        <Column field="last_at" :header="t('discussions.view.last')" style="width: 200px">
          <template #body="{ data }">
            <span class="small" :title="formatFullDateTime(data.last_at)">{{ t('discussions.view.lastBy', { date: formatDateTime(data.last_at), name: data.last_by }) }}</span>
          </template>
        </Column>
        <template #empty>
          <div class="empty">{{ t('discussions.view.empty') }}</div>
        </template>
      </DataTable>
    </div>
  </section>
</template>
