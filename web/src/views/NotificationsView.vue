<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import SelectButton from 'primevue/selectbutton'
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
      </div>
    </div>
    <p v-if="!shown.length" class="muted">{{ t('notifications.empty') }}</p>
    <ul v-else class="list card">
      <li v-for="n in shown" :key="n.id" class="row" :class="{ unread: !n.read_at }">
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
