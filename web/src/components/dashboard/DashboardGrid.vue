<script setup lang="ts">
// The dashboard grid: widgets on 12 columns (one column on phones, in
// reading order). In edit mode every widget has a move and a resize handle:
// drag them with the pointer, or focus them and use the arrow keys (Home and
// End: first and last column, or narrowest and full width); Enter or Space
// picks the widget up and drops it, Escape puts it back. Every change is
// announced. Widgets report when they come near the screen ("visible"), so
// their data loads lazily.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import { GRID_COLS, applyKey, cellAt, gridRows, moveItem, readingOrder, resizeItem, type KeyMode } from '@/lib/dashboardGrid'
import type { DashboardWidget } from '@/lib/widgets'

const props = defineProps<{
  items: DashboardWidget[]
  editing: boolean
  titleOf: (w: DashboardWidget) => string
  uid: string
}>()
const emit = defineEmits<{
  'update:items': [items: DashboardWidget[]]
  visible: [id: string]
  configure: [w: DashboardWidget]
  remove: [w: DashboardWidget]
}>()
const { t } = useI18n()

const ROW_PX = 80
const GAP_PX = 16
const gridEl = ref<HTMLElement | null>(null)
const ordered = computed(() => readingOrder(props.items))
const rows = computed(() => gridRows(props.items))
const announcement = ref('')
function announce(msg: string) {
  // the same text twice is still read
  announcement.value = ''
  void nextTick(() => (announcement.value = msg))
}
const cellStyle = (w: DashboardWidget) => ({ gridColumn: `${w.x + 1} / span ${w.w}`, gridRow: `${w.y + 1} / span ${w.h}`, '--h': String(w.h) })

// ---------- keyboard ----------
const grab = ref<{ id: string; mode: KeyMode; before: DashboardWidget[] } | null>(null)
function itemOf(id: string, list = props.items) {
  return list.find((x) => x.id === id)
}
function describePlace(w: DashboardWidget, mode: KeyMode) {
  const title = props.titleOf(w)
  return mode === 'move' ? t('dashboards.announce.moved', { title, col: w.x + 1, row: w.y + 1 }) : t('dashboards.announce.resized', { title, w: w.w, h: w.h })
}
function refocus(id: string, mode: KeyMode) {
  void nextTick(() => {
    const el = gridEl.value?.querySelector<HTMLElement>(`[data-handle="${CSS.escape(`${id}:${mode}`)}"]`)
    if (el && document.activeElement !== el) el.focus()
  })
}
function onKey(w: DashboardWidget, mode0: KeyMode, e: KeyboardEvent) {
  const mode: KeyMode = e.shiftKey && mode0 === 'move' && e.key.startsWith('Arrow') ? 'resize' : mode0
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    if (grab.value?.id === w.id) {
      grab.value = null
      announce(t('dashboards.announce.dropped', { title: props.titleOf(w) }))
    } else {
      grab.value = { id: w.id, mode: mode0, before: props.items.map((x) => ({ ...x })) }
      announce(t(mode0 === 'move' ? 'dashboards.announce.grabbedMove' : 'dashboards.announce.grabbedResize', { title: props.titleOf(w) }))
    }
    return
  }
  if (e.key === 'Escape' && grab.value?.id === w.id) {
    e.preventDefault()
    e.stopPropagation()
    emit('update:items', grab.value.before)
    grab.value = null
    announce(t('dashboards.announce.cancelled', { title: props.titleOf(w) }))
    refocus(w.id, mode0)
    return
  }
  const next = applyKey(props.items, w.id, e.key, mode)
  if (!next) return
  e.preventDefault()
  emit('update:items', next)
  const moved = itemOf(w.id, next)
  if (moved) announce(describePlace(moved, mode))
  refocus(w.id, mode0)
}

// ---------- pointer ----------
const drag = ref<{ id: string; mode: KeyMode; start: DashboardWidget[]; grabX: number; grabY: number; pointer: number; changed: boolean } | null>(null)
function metrics() {
  const r = gridEl.value!.getBoundingClientRect()
  return { r, colW: (r.width + GAP_PX) / GRID_COLS, rowH: ROW_PX + GAP_PX }
}
function startDrag(w: DashboardWidget, mode: KeyMode, e: PointerEvent) {
  if (!gridEl.value || e.button !== 0) return
  const { r, colW, rowH } = metrics()
  const cell = cellAt(e.clientX - r.left, e.clientY - r.top, colW, rowH)
  drag.value = { id: w.id, mode, start: props.items.map((x) => ({ ...x })), grabX: cell.x - w.x, grabY: cell.y - w.y, pointer: e.pointerId, changed: false }
  ;(e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId)
  e.preventDefault()
}
function onPointerMove(e: PointerEvent) {
  const dr = drag.value
  if (!dr || e.pointerId !== dr.pointer || !gridEl.value) return
  const { r, colW, rowH } = metrics()
  const it = itemOf(dr.id, dr.start)
  if (!it) return
  let next: DashboardWidget[]
  if (dr.mode === 'move') {
    const c = cellAt(e.clientX - r.left, e.clientY - r.top, colW, rowH, dr.grabX, dr.grabY)
    next = moveItem(dr.start, dr.id, c.x, c.y)
  } else {
    const c = cellAt(e.clientX - r.left, e.clientY - r.top, colW, rowH)
    next = resizeItem(dr.start, dr.id, c.x - it.x + 1, c.y - it.y + 1)
  }
  const cur = itemOf(dr.id)
  const n = itemOf(dr.id, next)
  if (cur && n && (cur.x !== n.x || cur.y !== n.y || cur.w !== n.w || cur.h !== n.h)) {
    dr.changed = true
    emit('update:items', next)
  }
}
function onPointerUp(e: PointerEvent) {
  const dr = drag.value
  if (!dr || e.pointerId !== dr.pointer) return
  drag.value = null
  const it = itemOf(dr.id)
  if (dr.changed && it) announce(describePlace(it, dr.mode))
}
function cancelDrag(e: KeyboardEvent) {
  if (e.key === 'Escape' && drag.value) {
    emit('update:items', drag.value.start)
    drag.value = null
  }
}
onMounted(() => {
  window.addEventListener('pointermove', onPointerMove)
  window.addEventListener('pointerup', onPointerUp)
  window.addEventListener('pointercancel', onPointerUp)
  window.addEventListener('keydown', cancelDrag)
})
onBeforeUnmount(() => {
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('pointerup', onPointerUp)
  window.removeEventListener('pointercancel', onPointerUp)
  window.removeEventListener('keydown', cancelDrag)
  io?.disconnect()
})
watch(
  () => props.editing,
  (v) => {
    if (!v) {
      grab.value = null
      drag.value = null
    }
  },
)

// ---------- lazy loading ----------
const seen = new Set<string>()
let io: IntersectionObserver | null = null
function observe() {
  const els = gridEl.value?.querySelectorAll<HTMLElement>('[data-widget]') ?? []
  for (const el of els) {
    const id = el.dataset.widget!
    if (seen.has(id)) continue
    if (typeof IntersectionObserver === 'undefined') {
      seen.add(id)
      emit('visible', id)
      continue
    }
    if (!io) {
      io = new IntersectionObserver(
        (entries) => {
          for (const en of entries) {
            const wid = (en.target as HTMLElement).dataset.widget!
            if (en.isIntersecting && !seen.has(wid)) {
              seen.add(wid)
              io?.unobserve(en.target)
              emit('visible', wid)
            }
          }
        },
        { rootMargin: '300px 0px' },
      )
    }
    io.observe(el)
  }
}
onMounted(observe)
watch(
  () => props.items.map((x) => x.id).join(),
  () => void nextTick(observe),
)
/** Forget which widgets were seen (a new dashboard): they report again. */
function reset() {
  seen.clear()
  io?.disconnect()
  io = null
  void nextTick(observe)
}
defineExpose({ reset })
</script>

<template>
  <div ref="gridEl" class="dash-grid" :class="{ editing }" :style="{ '--rows': String(rows) }">
    <p :id="`${uid}-help`" class="sr-only">{{ t('dashboards.handleHelp') }}</p>
    <div class="sr-only" aria-live="assertive" aria-atomic="true">{{ announcement }}</div>
    <section
      v-for="w in ordered"
      :key="w.id"
      :data-widget="w.id"
      class="card widget"
      :class="[`widget-${w.type}`, { active: drag?.id === w.id || grab?.id === w.id }]"
      :style="cellStyle(w)"
      :aria-labelledby="`${uid}-${w.id}-t`"
    >
      <header class="widget-head">
        <h2 :id="`${uid}-${w.id}-t`" class="widget-title">{{ titleOf(w) }}</h2>
        <div v-if="editing" class="widget-tools">
          <button
            type="button"
            class="handle"
            :data-handle="`${w.id}:move`"
            :aria-label="t('dashboards.moveHandle', { title: titleOf(w) })"
            :aria-describedby="`${uid}-help`"
            :aria-pressed="grab?.id === w.id && grab.mode === 'move'"
            v-tooltip="t('dashboards.move')"
            @keydown="onKey(w, 'move', $event)"
            @pointerdown="startDrag(w, 'move', $event)"
          >
            <i class="pi pi-arrows-alt" aria-hidden="true" />
          </button>
          <Button icon="pi pi-cog" text rounded size="small" severity="secondary" :aria-label="t('dashboards.configureNamed', { title: titleOf(w) })" v-tooltip="t('dashboards.configure')" @click="emit('configure', w)" />
          <Button icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('dashboards.removeNamed', { title: titleOf(w) })" v-tooltip="t('dashboards.remove')" @click="emit('remove', w)" />
        </div>
      </header>
      <div class="widget-body" :inert="editing || undefined"><slot :item="w" /></div>
      <button
        v-if="editing"
        type="button"
        class="handle resize"
        :data-handle="`${w.id}:resize`"
        :aria-label="t('dashboards.resizeHandle', { title: titleOf(w), w: w.w, h: w.h })"
        :aria-describedby="`${uid}-help`"
        :aria-pressed="grab?.id === w.id && grab.mode === 'resize'"
        @keydown="onKey(w, 'resize', $event)"
        @pointerdown="startDrag(w, 'resize', $event)"
      >
        <i class="pi pi-arrow-down-right" aria-hidden="true" />
      </button>
    </section>
  </div>
</template>

<style scoped>
.dash-grid {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  grid-auto-rows: 80px;
  gap: 16px;
  position: relative;
  min-height: 80px;
}
.dash-grid.editing {
  background-image: linear-gradient(to right, color-mix(in srgb, var(--qtf-border) 60%, transparent) 1px, transparent 1px);
  background-size: calc((100% + 16px) / 12) 100%;
  border-radius: 8px;
  padding-bottom: 96px;
}
.widget {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  margin: 0;
  overflow: hidden;
}
.widget.active {
  outline: 2px dashed var(--p-primary-color, #2563eb);
  outline-offset: 2px;
  z-index: 2;
}
.widget-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  min-height: 28px;
}
.widget-title {
  flex: 1;
  min-width: 0;
  margin: 0;
  font-size: 15px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.widget-tools {
  display: flex;
  align-items: center;
  gap: 2px;
}
.widget-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.widget-key_figures .widget-body {
  overflow-y: auto;
}
.handle {
  display: inline-grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border: 1px solid var(--qtf-border);
  border-radius: 6px;
  background: var(--qtf-card);
  color: var(--qtf-text);
  cursor: grab;
  touch-action: none;
}
.handle[aria-pressed='true'] {
  background: var(--p-primary-color, #2563eb);
  color: var(--p-primary-contrast-color, #fff);
}
.handle:focus-visible {
  outline: 2px solid var(--p-primary-color, #2563eb);
  outline-offset: 2px;
}
.handle.resize {
  position: absolute;
  right: 4px;
  bottom: 4px;
  cursor: nwse-resize;
  z-index: 1;
}
@media (max-width: 640px) {
  .dash-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .dash-grid.editing {
    background-image: none;
  }
  .widget {
    grid-column: 1 / -1 !important;
    grid-row: auto / span var(--h) !important;
  }
}
@media (forced-colors: active) {
  .widget.active {
    outline-color: Highlight;
  }
}
</style>
