<script setup lang="ts">
// Undo / redo buttons in the top bar, with the history of the changes the
// user can undo (newest first). Ctrl+Z (⌘Z) undoes, Ctrl+Shift+Z or Ctrl+Y
// redoes, outside text fields (where the browser undoes typing).
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Menu from 'primevue/menu'
import { isTypingTarget } from '@/lib/a11y'
import { undoKey } from '@/lib/undo'
import { relativeTime } from '@/lib/format'
import { useUndoStore } from '@/stores/undo'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const undo = useUndoStore()
const auth = useAuthStore()
const notify = useNotify()
const menu = ref<InstanceType<typeof Menu> | null>(null)
const status = ref('')

watch(
  () => (auth.user?.id ?? '') + '|' + (auth.tenant?.id ?? ''),
  (scope) => undo.bind(scope),
  { immediate: true },
)

async function doUndo() {
  const label = undo.next?.label
  if (!label) return
  try {
    await undo.undo()
    announce(t('undo.undone', { action: label }))
  } catch (e) {
    notify.fail(e, t('undo.undoFailed', { action: label }))
  }
}
async function doRedo() {
  const label = undo.nextRedo?.label
  if (!label) return
  try {
    await undo.redo()
    announce(t('undo.redone', { action: label }))
  } catch (e) {
    notify.fail(e, t('undo.redoFailed', { action: label }))
  }
}
async function undoTo(id: number) {
  try {
    const done = await undo.undoTo(id)
    announce(t('undo.undoneMany', done.length))
  } catch (e) {
    notify.fail(e, t('undo.undoFailed', { action: '' }))
  }
}
function announce(msg: string) {
  status.value = msg
  notify.info(msg)
}

const items = computed(() => {
  const list = undo.history.map((h) => ({ label: h.label, icon: 'pi pi-undo', detail: relativeTime(new Date(h.at).toISOString()), command: () => undoTo(h.id) }))
  return list.length ? [{ label: t('undo.history'), items: list }] : [{ label: t('undo.nothing'), disabled: true }]
})

function onKeydown(e: KeyboardEvent) {
  const k = undoKey(e)
  if (!k || e.defaultPrevented || isTypingTarget(e.target)) return
  const el = e.target as Element | null
  if (el?.closest?.('[role="dialog"], .p-dialog')) return
  if (k === 'undo' && undo.canUndo) {
    e.preventDefault()
    void doUndo()
  } else if (k === 'redo' && undo.canRedo) {
    e.preventDefault()
    void doRedo()
  }
}
onMounted(() => document.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))

const undoTip = computed(() => (undo.next ? t('undo.undoAction', { action: undo.next.label }) : t('undo.undo')))
const redoTip = computed(() => (undo.nextRedo ? t('undo.redoAction', { action: undo.nextRedo.label }) : t('undo.redo')))
</script>

<template>
  <div class="undo-controls row nowrap" role="group" :aria-label="t('undo.group')">
    <Button icon="pi pi-undo" text rounded severity="secondary" :disabled="!undo.canUndo" :aria-label="undoTip" v-tooltip.bottom="undoTip" aria-keyshortcuts="Control+Z" @click="doUndo" />
    <Button icon="pi pi-replay" text rounded severity="secondary" :disabled="!undo.canRedo" :aria-label="redoTip" v-tooltip.bottom="redoTip" aria-keyshortcuts="Control+Shift+Z Control+Y" class="redo-button" @click="doRedo" />
    <Button icon="pi pi-history" text rounded severity="secondary" :aria-label="t('undo.history')" v-tooltip.bottom="t('undo.history')" aria-haspopup="menu" @click="menu?.toggle($event)" />
    <Menu ref="menu" :model="items" popup :aria-label="t('undo.history')" class="undo-menu">
      <template #item="{ item, props: p }">
        <a v-bind="p.action" :aria-disabled="item.disabled === true ? 'true' : undefined">
          <span v-if="item.icon" :class="item.icon" v-bind="p.icon" />
          <span v-bind="p.label" class="grow ellipsis">{{ item.label }}</span>
          <span v-if="item.detail" class="muted small" style="margin-left: 8px">{{ item.detail }}</span>
        </a>
      </template>
    </Menu>
    <span class="sr-only" role="status" aria-live="polite">{{ status }}</span>
  </div>
</template>

<style>
.undo-controls { gap: 0; }
.redo-button .pi-replay { transform: scaleX(-1); }
.undo-menu { max-width: min(420px, 92vw); }
.undo-menu .p-menu-item-link { gap: 6px; }
</style>
