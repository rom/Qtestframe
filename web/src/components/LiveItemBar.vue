<script setup lang="ts">
// Live updates in an item editor: who else is viewing or editing the item
// (presence avatars), and a non-blocking banner when someone saved a newer
// version meanwhile (reload, or compare with the unsaved changes). The
// banner is announced once, politely; presence changes are not announced.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import UserAvatar from './UserAvatar.vue'
import { usePresence } from '@/composables/live'
import { newerVersion, type LivePresenceInfo, type NewerVersion } from '@/lib/live'
import { useAuthStore } from '@/stores/auth'
import { useLiveStore } from '@/stores/live'

const props = withDefaults(
  defineProps<{
    /** entity type of the live events (requirement, test, execution, …) */
    type: string
    itemId: string | null
    /** the version shown (requirements, tests); null for unversioned items */
    version?: number | null
    /** unsaved changes: the user counts as editing, and compare is offered */
    dirty?: boolean
    /** compare is available (the editor can show both versions) */
    canCompare?: boolean
  }>(),
  { version: null, dirty: false, canCompare: true },
)
const emit = defineEmits<{ reload: []; compare: [by: string] }>()
const { t } = useI18n()
const auth = useAuthStore()
const live = useLiveStore()

const { others } = usePresence(
  () => props.type,
  () => props.itemId,
  () => props.dirty,
)

function names(list: LivePresenceInfo[]): string {
  const n = list.map((p) => p.name || '?')
  if (n.length === 1) return n[0]
  if (n.length === 2) return t('live.and', { a: n[0], b: n[1] })
  return t('live.othersCount', { first: n[0], n: n.length - 1 })
}
const editors = computed(() => others.value.filter((p) => p.state === 'editing'))
const viewers = computed(() => others.value.filter((p) => p.state !== 'editing'))
const presenceText = computed(() => {
  const parts: string[] = []
  if (editors.value.length) parts.push(t('live.editing', { names: names(editors.value) }, editors.value.length))
  if (viewers.value.length) parts.push(t('live.viewing', { names: names(viewers.value) }, viewers.value.length))
  return parts.join(' · ')
})

const newer = ref<NewerVersion | null>(null)
let off: (() => void) | null = null
onMounted(() => {
  off = live.subscribe((ev) => {
    if (!props.itemId) return
    const n = newerVersion(ev, props.type, props.itemId, props.version, auth.user?.id)
    if (n) newer.value = n
  })
})
onBeforeUnmount(() => off?.())
// another item, or the newer version was loaded: the banner goes
watch(
  () => [props.itemId, props.version] as const,
  ([id, v], [oldId]) => {
    if (id !== oldId || (newer.value && v && newer.value.version && v >= newer.value.version)) newer.value = null
  },
)

const bannerText = computed(() => {
  const n = newer.value
  if (!n) return ''
  if (n.deleted) return t('live.deletedBy', { name: n.by || '?' })
  if (!n.version) return t('live.changedUnversioned', { name: n.by || '?' })
  return n.by ? t('live.changedBy', { name: n.by, version: n.version }) : t('live.changedBySomeone', { version: n.version })
})
const announced = computed(() => (bannerText.value ? bannerText.value + (props.dirty ? ' ' + t('live.unsavedHint') : '') : ''))

function reload() {
  newer.value = null
  emit('reload')
}
</script>

<template>
  <div v-if="itemId" class="live-item-bar">
    <!-- polite: the newer version is announced once, presence is not announced -->
    <span class="sr-only" aria-live="polite" aria-atomic="true">{{ announced }}</span>
    <div v-if="others.length" class="live-presence" role="group" :aria-label="t('live.presenceLabel')" data-testid="live-presence">
      <span class="live-avatars" aria-hidden="true">
        <span v-for="p in others.slice(0, 5)" :key="p.user_id" class="live-avatar" :class="{ editing: p.state === 'editing' }" :title="p.name">
          <UserAvatar :avatar="p.avatar ?? null" :name="p.name" :user-id="p.user_id" :size="24" />
        </span>
      </span>
      <span class="small">{{ presenceText }}</span>
    </div>
    <section v-if="newer" class="live-banner" :aria-label="t('live.bannerLabel')" data-testid="live-banner">
      <i class="pi pi-info-circle" aria-hidden="true" />
      <span class="live-banner-text">{{ bannerText }}<template v-if="dirty"> {{ t('live.unsavedHint') }}</template></span>
      <span class="live-banner-actions">
        <Button v-if="!newer.deleted" size="small" :label="t('live.reload')" icon="pi pi-refresh" severity="secondary" outlined @click="reload" />
        <Button v-if="!newer.deleted && dirty && canCompare" size="small" :label="t('live.compare')" icon="pi pi-clone" severity="secondary" outlined @click="emit('compare', newer.by)" />
        <Button size="small" :label="t('live.dismiss')" text severity="secondary" @click="newer = null" />
      </span>
    </section>
  </div>
</template>

<style scoped>
.live-item-bar {
  display: grid;
  gap: 8px;
  margin-bottom: 8px;
}
.live-item-bar:empty {
  display: none;
}
.live-presence {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.live-avatars {
  display: inline-flex;
}
.live-avatar {
  display: inline-flex;
  border-radius: 50%;
  box-shadow: 0 0 0 2px var(--qtf-surface, #fff);
  margin-left: -6px;
}
.live-avatar:first-child {
  margin-left: 0;
}
.live-avatar.editing {
  box-shadow: 0 0 0 2px var(--p-primary-color, #2563eb);
}
.live-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 8px 10px;
  border: 1px solid var(--qtf-border);
  border-left: 4px solid var(--p-primary-color, #2563eb);
  border-radius: 6px;
}
.live-banner-text {
  flex: 1 1 240px;
}
.live-banner-actions {
  display: inline-flex;
  gap: 6px;
  flex-wrap: wrap;
}
</style>
