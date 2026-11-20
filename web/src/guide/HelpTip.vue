<script setup lang="ts">
// Context-sensitive help: a small "i" button that explains a field or a
// section. The explanation appears on mouse-over and on keyboard focus
// (WCAG 1.4.13: it can be hovered, stays until dismissed, Escape closes it);
// a click or tap pins it. "Read more" opens the matching section of the
// user guide. Screen readers get the text as the button's description.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { uid } from '@/lib/a11y'
import { placeTip, tipHelpPath, tipKey, type TipId } from './tips'

const props = defineProps<{ id: TipId; label?: string }>()
const { t, locale } = useI18n()
const router = useRouter()

const open = ref(false)
const pinned = ref(false)
const button = ref<HTMLButtonElement>()
const bubble = ref<HTMLDivElement>()
const link = ref<HTMLAnchorElement>()
const pos = ref({ top: 0, left: 0, above: false })
const ids = { desc: uid('tipd'), bubble: uid('tip') }

const text = computed(() => t(tipKey(props.id)))
const href = computed(() => tipHelpPath(props.id, String(locale.value)))
const ariaLabel = computed(() => (props.label ? t('guide.tipFor', { topic: props.label }) : t('guide.tip')))

let hideTimer: ReturnType<typeof setTimeout> | undefined
function cancelHide() {
  clearTimeout(hideTimer)
}
function show() {
  cancelHide()
  if (open.value) return
  open.value = true
  void nextTick(position)
}
function hide(force = false) {
  cancelHide()
  if (pinned.value && !force) return
  open.value = false
  pinned.value = false
}
/** Mouse left the button or the bubble: wait a moment so the pointer can move between them. */
function hideSoon() {
  if (pinned.value) return
  cancelHide()
  hideTimer = setTimeout(() => hide(), 180)
}
function toggle() {
  if (pinned.value) return hide(true)
  pinned.value = true
  show()
}

function position() {
  if (!button.value || !bubble.value) return
  const r = button.value.getBoundingClientRect()
  const b = bubble.value.getBoundingClientRect()
  pos.value = placeTip(r, { width: b.width, height: b.height }, window.innerWidth, window.innerHeight)
}

function onFocusOut(e: FocusEvent) {
  const next = e.relatedTarget as Node | null
  if (next && (button.value?.contains(next) || bubble.value?.contains(next))) return
  hide(true)
}
function onButtonKey(e: KeyboardEvent) {
  // the bubble lives at the end of the document: Tab moves into its link first
  if (e.key === 'Tab' && !e.shiftKey && open.value && link.value) {
    e.preventDefault()
    link.value.focus()
  }
}
function onLinkKey(e: KeyboardEvent) {
  if (e.key === 'Tab') {
    e.preventDefault()
    button.value?.focus()
    if (!e.shiftKey) hide(true)
  }
}
function onDocKey(e: KeyboardEvent) {
  if (e.key !== 'Escape' || !open.value) return
  // close the tip only, not the drawer or dialog around it
  e.stopPropagation()
  e.preventDefault()
  const inside = bubble.value?.contains(document.activeElement)
  hide(true)
  if (inside) button.value?.focus()
}
function onDocPointer(e: PointerEvent) {
  const target = e.target as Node
  if (button.value?.contains(target) || bubble.value?.contains(target)) return
  hide(true)
}
function onViewport() {
  if (open.value) position()
}
function readMore(e: MouseEvent) {
  if (!href.value || e.ctrlKey || e.metaKey || e.shiftKey || e.button !== 0) return
  e.preventDefault()
  hide(true)
  void router.push(href.value)
}

function listen(on: boolean) {
  const m = on ? 'addEventListener' : 'removeEventListener'
  document[m]('keydown', onDocKey as EventListener, true)
  document[m]('pointerdown', onDocPointer as EventListener, true)
  window[m]('resize', onViewport)
  window[m]('scroll', onViewport, true)
}
watch(open, (v) => listen(v))
onBeforeUnmount(() => {
  cancelHide()
  if (open.value) listen(false)
})
</script>

<template>
  <button
    ref="button"
    type="button"
    class="qtf-tip"
    :aria-label="ariaLabel"
    :aria-describedby="ids.desc"
    :aria-expanded="open"
    :aria-controls="ids.bubble"
    @mouseenter="show"
    @mouseleave="hideSoon"
    @focus="show"
    @focusout="onFocusOut"
    @keydown="onButtonKey"
    @click.stop.prevent="toggle"
  >
    <i class="pi pi-info-circle" aria-hidden="true" />
    <span :id="ids.desc" hidden>{{ text }}</span>
  </button>
  <Teleport to="body">
    <div
      v-if="open"
      :id="ids.bubble"
      ref="bubble"
      class="qtf-tip-bubble"
      :class="{ above: pos.above }"
      :style="{ top: pos.top + 'px', left: pos.left + 'px' }"
      @mouseenter="cancelHide"
      @mouseleave="hideSoon"
      @focusout="onFocusOut"
    >
      <p>{{ text }}</p>
      <a v-if="href" ref="link" :href="href" class="qtf-tip-more" @click="readMore" @keydown="onLinkKey">
        <i class="pi pi-book" aria-hidden="true" /> {{ t('guide.tipMore') }}
      </a>
    </div>
  </Teleport>
</template>

<style>
.qtf-tip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  margin: 0;
  border: 0;
  border-radius: 50%;
  background: none;
  color: var(--qtf-muted);
  cursor: help;
  flex: none;
  vertical-align: middle;
}
.qtf-tip:hover,
.qtf-tip[aria-expanded='true'] {
  color: var(--p-primary-color);
  background: var(--qtf-hover);
}
.qtf-tip:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 1px;
}
.qtf-tip .pi {
  font-size: 13px;
}
.qtf-tip-bubble {
  position: fixed;
  z-index: 3000;
  max-width: min(320px, calc(100vw - 16px));
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid var(--qtf-border);
  background: var(--qtf-card);
  color: var(--qtf-text);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.18);
  font-size: 13px;
  font-weight: normal;
  line-height: 1.45;
  text-align: left;
  white-space: normal;
}
.qtf-tip-bubble p {
  margin: 0;
}
.qtf-tip-more {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  margin-top: 6px;
  color: var(--p-primary-color);
  font-weight: 600;
  text-decoration: underline;
}
/* a field label with a tip: label and "i" on one line, the control below */
.field:has(> .qtf-tip) {
  display: grid;
  grid-template-columns: minmax(0, max-content) 1fr;
  align-items: center;
  column-gap: 4px;
}
.field:has(> .qtf-tip) > * {
  grid-column: 1 / -1;
}
.field:has(> .qtf-tip) > label,
.field:has(> .qtf-tip) > .field-label {
  grid-column: 1;
}
.field:has(> .qtf-tip) > .qtf-tip {
  grid-column: 2;
  justify-self: start;
}
</style>
