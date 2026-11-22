<script setup lang="ts">
// Bell in the top bar: unread count, the latest notifications, mark read.
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Popover from 'primevue/popover'
import { useNotificationsStore } from '@/stores/notifications'
import { badgeText, eventKey, notificationIcon, notificationTarget } from '@/lib/notifications'
import { formatDateTime } from '@/lib/format'
import type { AppNotification } from '@/lib/types'

const { t } = useI18n()
const store = useNotificationsStore()
const router = useRouter()
const panel = ref<InstanceType<typeof Popover>>()

onMounted(() => store.start())
onBeforeUnmount(() => store.stop())

function toggle(e: Event) {
  void store.load()
  panel.value?.toggle(e)
}

async function open(n: AppNotification) {
  panel.value?.hide()
  if (!n.read_at) await store.markRead([n.id]).catch(() => undefined)
  const to = notificationTarget(n)
  if (to) await router.push(to)
}
</script>

<template>
  <span class="bell">
    <Button
      icon="pi pi-bell"
      text
      rounded
      severity="secondary"
      aria-haspopup="dialog"
      :aria-label="store.unread ? t('notifications.bellUnread', { n: store.unread }, store.unread) : t('notifications.bell')"
      v-tooltip.bottom="t('notifications.bell')"
      @click="toggle"
    />
    <span v-if="store.unread" class="badge" aria-hidden="true">{{ badgeText(store.unread) }}</span>
    <Popover ref="panel" :aria-label="t('notifications.title')">
      <div class="panel">
        <div class="row head">
          <b class="grow">{{ t('notifications.title') }}</b>
          <Button v-if="store.unread" :label="t('notifications.markAll')" text size="small" @click="store.markRead()" />
        </div>
        <p v-if="!store.items.length" class="muted small">{{ t('notifications.empty') }}</p>
        <ul v-else class="list">
          <li v-for="n in store.items.slice(0, 10)" :key="n.id">
            <button type="button" class="item" :class="{ unread: !n.read_at }" @click="open(n)">
              <i :class="notificationIcon(n.event)" aria-hidden="true" />
              <span class="grow">
                <span class="small muted">{{ t(`notifications.event.${eventKey(n.event)}`, { actor: n.actor }) }}</span>
                <span class="title">{{ n.title }}</span>
                <span class="small muted">{{ formatDateTime(n.created_at) }}</span>
              </span>
              <span v-if="!n.read_at" class="sr-only">{{ t('notifications.unread') }}</span>
            </button>
          </li>
        </ul>
        <div class="row foot">
          <RouterLink :to="{ name: 'notifications' }" class="small" @click="panel?.hide()">{{ t('notifications.seeAll') }}</RouterLink>
          <RouterLink :to="{ name: 'myWork', query: { view: 'reviews' } }" class="small" @click="panel?.hide()">{{ t('nav.reviews') }}</RouterLink>
        </div>
      </div>
    </Popover>
  </span>
</template>

<style scoped>
.bell {
  position: relative;
  display: inline-flex;
}
.badge {
  position: absolute;
  top: 2px;
  right: 0;
  min-width: 18px;
  height: 18px;
  padding: 0 4px;
  border-radius: 9px;
  background: var(--p-red-600, #c62828);
  color: #fff;
  font-size: 11px;
  font-weight: 700;
  line-height: 18px;
  text-align: center;
  pointer-events: none;
}
.panel {
  width: min(360px, calc(100vw - 48px));
}
.head,
.foot {
  gap: 8px;
  justify-content: space-between;
}
.list {
  list-style: none;
  margin: 8px 0;
  padding: 0;
  max-height: 60vh;
  overflow: auto;
}
.item {
  display: flex;
  gap: 10px;
  width: 100%;
  padding: 8px;
  border: 0;
  border-radius: 6px;
  background: none;
  color: inherit;
  text-align: left;
  cursor: pointer;
  font: inherit;
}
.item:hover,
.item:focus-visible {
  background: var(--qtf-hover, rgba(127, 127, 127, 0.12));
}
.item.unread {
  border-left: 3px solid var(--p-primary-color);
}
.item .grow {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.title {
  overflow-wrap: anywhere;
}
</style>
