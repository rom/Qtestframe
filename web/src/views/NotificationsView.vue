<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import SelectButton from 'primevue/selectbutton'
import Checkbox from 'primevue/checkbox'
import { useNotificationsStore } from '@/stores/notifications'
import { eventKey, notificationIcon, notificationTarget } from '@/lib/notifications'
import { formatDateTime } from '@/lib/format'
import { useNotify } from '@/composables/notify'
import type { AppNotification } from '@/lib/types'

const { t } = useI18n()
const store = useNotificationsStore()
const router = useRouter()
const notify = useNotify()
const filter = ref<'all' | 'unread'>('all')
const filters = computed(() => [
  { label: t('notifications.all'), value: 'all' },
  { label: t('notifications.unreadOnly'), value: 'unread' },
])
const shown = computed(() => (filter.value === 'unread' ? store.items.filter((n) => !n.read_at) : store.items))

onMounted(() => store.load())

async function open(n: AppNotification) {
  if (!n.read_at) await store.markRead([n.id]).catch(() => undefined)
  const to = notificationTarget(n)
  if (to) await router.push(to)
}

// ---------- bulk ----------
const selected = ref<string[]>([])
watch(shown, (list) => {
  const ids = new Set(list.map((n) => n.id))
  selected.value = selected.value.filter((id) => ids.has(id))
})
const allSelected = computed({
  get: () => shown.value.length > 0 && selected.value.length === shown.value.length,
  set: (v: boolean) => (selected.value = v ? shown.value.map((n) => n.id) : []),
})
const hasRead = computed(() => store.items.some((n) => n.read_at))
async function bulk(action: 'read' | 'unread' | 'delete') {
  const ids = [...selected.value]
  try {
    if (action === 'read') await store.markRead(ids)
    else if (action === 'unread') await store.markUnread(ids)
    else {
      const n = await store.remove(ids)
      notify.ok(t('notifications.bulk.deleted', { n }, n))
    }
    selected.value = []
  } catch (e) {
    notify.fail(e)
  }
}
function deleteRead() {
  notify.confirmDanger(t('notifications.bulk.confirmDeleteRead'), async () => {
    try {
      const n = await store.remove('read')
      notify.ok(t('notifications.bulk.deleted', { n }, n))
    } catch (e) {
      notify.fail(e)
    }
  })
}

async function markAll() {
  try {
    await store.markRead()
  } catch (e) {
    notify.fail(e)
  }
}
</script>

<template>
  <section>
    <div class="page-header">
      <div>
        <h1>{{ t('notifications.title') }}</h1>
        <div class="sub">{{ t('notifications.intro') }}</div>
      </div>
      <div class="actions">
      <SelectButton v-model="filter" :options="filters" option-label="label" option-value="value" :allow-empty="false" :aria-label="t('notifications.filter')" />
      <Button :label="t('notifications.markAll')" icon="pi pi-check" outlined :disabled="!store.unread" @click="markAll" />
      <Button :label="t('notifications.bulk.deleteRead')" icon="pi pi-trash" severity="danger" outlined :disabled="!hasRead" @click="deleteRead" />
      <RouterLink :to="{ name: 'profile', hash: '#email' }" class="p-button p-button-text p-button-secondary"><i class="pi pi-envelope" aria-hidden="true" />&nbsp;{{ t('email.profileTitle') }}</RouterLink>
      </div>
    </div>
    <p v-if="!shown.length" class="muted">{{ t('notifications.empty') }}</p>
    <div v-if="shown.length" class="row select-all">
      <Checkbox v-model="allSelected" binary input-id="nt-all" />
      <label for="nt-all" class="small">{{ t('notifications.bulk.selectAll') }}</label>
    </div>
    <div v-if="selected.length" class="bulkbar" role="toolbar" :aria-label="t('notifications.bulk.toolbar')">
      <b>{{ t('common.selected', { count: selected.length }) }}</b>
      <Button :label="t('notifications.markRead')" icon="pi pi-check" size="small" severity="secondary" outlined @click="bulk('read')" />
      <Button :label="t('notifications.bulk.markUnread')" icon="pi pi-envelope" size="small" severity="secondary" outlined @click="bulk('unread')" />
      <Button :label="t('common.delete')" icon="pi pi-trash" size="small" severity="danger" outlined @click="bulk('delete')" />
      <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selected = []" />
    </div>
    <ul v-if="shown.length" class="list card">
      <li v-for="n in shown" :key="n.id" class="row" :class="{ unread: !n.read_at }">
        <Checkbox v-model="selected" :value="n.id" :input-id="`nt-${n.id}`" :aria-label="t('notifications.bulk.selectOne', { title: n.title })" />
        <i :class="notificationIcon(n.event)" aria-hidden="true" />
        <div class="grow">
          <div class="small muted">{{ t(`notifications.event.${eventKey(n.event)}`, { actor: n.actor }) }} · {{ formatDateTime(n.created_at) }}</div>
          <button type="button" class="linklike" @click="open(n)">{{ n.title }}</button>
          <div v-if="n.body" class="small body">{{ n.body }}</div>
        </div>
        <Button v-if="!n.read_at" icon="pi pi-check" text rounded size="small" :aria-label="t('notifications.markRead')" v-tooltip.left="t('notifications.markRead')" @click="store.markRead([n.id])" />
      </li>
    </ul>
  </section>
</template>

<style scoped>
.select-all {
  gap: 8px;
  margin: 0 0 8px 17px;
}
.list {
  list-style: none;
  padding: 0;
}
.list li {
  gap: 12px;
  align-items: flex-start;
  padding: 12px 16px;
  border-bottom: 1px solid var(--qtf-border);
}
.list li:last-child {
  border-bottom: 0;
}
.list li.unread {
  border-left: 3px solid var(--p-primary-color);
}
.linklike {
  border: 0;
  padding: 0;
  background: none;
  color: var(--p-primary-color);
  cursor: pointer;
  font: inherit;
  text-align: left;
  overflow-wrap: anywhere;
}
.linklike:hover {
  text-decoration: underline;
}
.body {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
