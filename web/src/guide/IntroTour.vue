<script setup lang="ts">
// The intro tour overlay: a modal step card next to a spotlighted UI
// element. Logic (steps, transitions, placement) lives in tour.ts.
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import { useBreakpoint } from '@/lib/breakpoint'
import { useUiStore } from '@/stores/ui'
import { usePrefsStore } from '@/stores/prefs'
import {
  TOUR_STEPS,
  focusables,
  initialTour,
  isLastStep,
  placeCard,
  resolveTarget,
  spotlight,
  tourEnded,
  tourTransition,
  trapFocus,
  type Box,
  type TourEvent,
} from './tour'

const emit = defineEmits<{ ended: [status: 'finished' | 'skipped'] }>()
const { t } = useI18n()
const ui = useUiStore()
const prefs = usePrefsStore()
const overlayNav = useBreakpoint('tablet')
const phone = useBreakpoint('phone')

const state = ref(initialTour())
const active = computed(() => state.value.status === 'active')
const step = computed(() => TOUR_STEPS[state.value.index])
const hole = ref<Box | null>(null)
const pos = ref<{ top: number; left: number }>({ top: 0, left: 0 })
const placed = ref(false)
const card = ref<HTMLElement>()
const title = ref<HTMLElement>()
const announcement = ref('')
let returnFocus: HTMLElement | null = null
let openedDrawer = false

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

function setAppInert(on: boolean) {
  const app = document.getElementById('app')
  if (app) app.inert = on
}

/** Find the step's target (opening or closing the nav drawer as needed), then place spotlight and card. */
async function locate() {
  placed.value = false
  const s = step.value
  if (!s) return
  let found = resolveTarget(s, document, { drawerClosed: overlayNav.value && !ui.mobileNav })
  if (found && found.inDrawer && overlayNav.value && !ui.mobileNav) {
    ui.mobileNav = true
    openedDrawer = true
    await wait(320)
  } else if ((!found || !found.inDrawer) && overlayNav.value && ui.mobileNav && openedDrawer) {
    ui.mobileNav = false
    await wait(320)
  }
  if (!active.value || step.value !== s) return
  found = resolveTarget(s, document)
  if (found) (found.el as HTMLElement).scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
  measure(found?.el ?? null)
}

let currentEl: Element | null = null
function measure(el: Element | null = currentEl) {
  currentEl = el
  const vp = { width: window.innerWidth, height: window.innerHeight }
  const r = el?.getBoundingClientRect()
  hole.value = r && r.width > 0 && r.height > 0 ? spotlight({ top: r.top, left: r.left, width: r.width, height: r.height }, vp) : null
  const c = card.value
  const size = { width: c?.offsetWidth || Math.min(380, vp.width - 24), height: c?.offsetHeight || 220 }
  const p = placeCard(hole.value, size, vp, { narrow: 640 })
  pos.value = { top: p.top, left: p.left }
  placed.value = true
}

function onResize() {
  if (active.value) measure()
}

async function go(event: TourEvent) {
  const before = state.value
  state.value = tourTransition(state.value, event)
  if (tourEnded(state.value)) return end(state.value.status as 'finished' | 'skipped')
  if (state.value.index === before.index && event !== 'start') return
  const s = TOUR_STEPS[state.value.index]
  announcement.value = `${t('tour.stepOf', { n: state.value.index + 1, total: state.value.total })}. ${t(`tour.steps.${s.id}.title`)}. ${t(`tour.steps.${s.id}.body`)}`
  await nextTick()
  await locate()
  await nextTick()
  measure()
  if (event === 'start') title.value?.focus()
  else if (!card.value?.contains(document.activeElement)) card.value?.querySelector<HTMLElement>('.tour-next')?.focus()
}

/** Start (or restart) the tour. */
async function start() {
  if (active.value) return
  returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
  openedDrawer = false
  setAppInert(true)
  window.addEventListener('resize', onResize)
  window.addEventListener('scroll', onResize, true)
  await go('start')
}

async function end(status: 'finished' | 'skipped') {
  window.removeEventListener('resize', onResize)
  window.removeEventListener('scroll', onResize, true)
  setAppInert(false)
  if (openedDrawer) ui.mobileNav = false
  hole.value = null
  emit('ended', status)
  try {
    await prefs.save({ intro_done: true })
  } catch (e) {
    console.warn('saving intro_done failed', e)
  }
  await nextTick()
  const back = returnFocus && document.contains(returnFocus) ? returnFocus : document.querySelector<HTMLElement>('.qtf-help-button')
  back?.focus()
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    void go('skip')
  } else if (e.key === 'Tab' && card.value) {
    const next = trapFocus(focusables(card.value), document.activeElement, e.shiftKey)
    if (next) {
      e.preventDefault()
      next.focus()
    }
  }
}

onBeforeUnmount(() => {
  if (active.value) {
    setAppInert(false)
    window.removeEventListener('resize', onResize)
    window.removeEventListener('scroll', onResize, true)
  }
})

const cardStyle = computed(() => ({ top: `${pos.value.top}px`, left: `${pos.value.left}px`, visibility: placed.value ? 'visible' : 'hidden' }) as Record<string, string>)
const holeStyle = computed(() => (hole.value ? { top: `${hole.value.top}px`, left: `${hole.value.left}px`, width: `${hole.value.width}px`, height: `${hole.value.height}px` } : {}))

defineExpose({ start, active })
</script>

<template>
  <Teleport to="body">
    <div v-if="active" class="tour-layer" :class="{ phone }">
      <div v-if="hole" class="tour-hole" :style="holeStyle" aria-hidden="true" />
      <div v-else class="tour-backdrop" aria-hidden="true" />
      <div
        ref="card"
        class="tour-card"
        role="dialog"
        aria-modal="true"
        aria-labelledby="tour-title"
        aria-describedby="tour-body"
        :style="cardStyle"
        @keydown="onKeydown"
      >
        <div class="tour-progress small">{{ t('tour.stepOf', { n: state.index + 1, total: state.total }) }}</div>
        <h2 id="tour-title" ref="title" tabindex="-1">{{ t(`tour.steps.${step.id}.title`) }}</h2>
        <p id="tour-body">{{ t(`tour.steps.${step.id}.body`) }}</p>
        <div class="tour-dots" aria-hidden="true">
          <span v-for="(_, i) in state.total" :key="i" :class="{ on: i === state.index }" />
        </div>
        <div class="tour-actions">
          <Button :label="t('tour.skip')" text severity="secondary" class="tour-skip" @click="go('skip')" />
          <span class="grow" />
          <Button v-if="state.index > 0" :label="t('tour.back')" icon="pi pi-arrow-left" severity="secondary" outlined class="tour-back" @click="go('back')" />
          <Button
            :label="isLastStep(state) ? t('tour.finish') : t('tour.next')"
            :icon="isLastStep(state) ? 'pi pi-check' : 'pi pi-arrow-right'"
            icon-pos="right"
            class="tour-next"
            @click="go('next')"
          />
        </div>
        <div class="sr-only" aria-live="polite" aria-atomic="true">{{ announcement }}</div>
      </div>
    </div>
  </Teleport>
</template>

<style>
.tour-layer {
  position: fixed;
  inset: 0;
  z-index: 3000;
}
.tour-backdrop {
  position: fixed;
  inset: 0;
  background: rgb(0 0 0 / 0.55);
}
.tour-hole {
  position: fixed;
  border-radius: 8px;
  box-shadow:
    0 0 0 3px var(--p-primary-color),
    0 0 0 200vmax rgb(0 0 0 / 0.55);
  pointer-events: none;
  transition:
    top 0.2s ease,
    left 0.2s ease,
    width 0.2s ease,
    height 0.2s ease;
}
.tour-card {
  position: fixed;
  width: min(380px, calc(100vw - 24px));
  background: var(--qtf-card);
  color: var(--qtf-text);
  border: 1px solid var(--qtf-border);
  border-radius: 12px;
  padding: 18px 18px 14px;
  box-shadow: 0 12px 40px rgb(0 0 0 / 0.3);
}
.tour-card h2 {
  font-size: 18px;
  margin: 4px 0 8px;
}
.tour-card h2:focus {
  outline: none;
}
.tour-card h2:focus-visible {
  outline: 2px solid var(--qtf-focus, var(--p-primary-color));
  outline-offset: 2px;
}
.tour-card p {
  margin: 0 0 12px;
  line-height: 1.5;
}
.tour-progress {
  color: var(--qtf-muted);
}
.tour-dots {
  display: flex;
  gap: 5px;
  margin-bottom: 12px;
}
.tour-dots span {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--qtf-border);
}
.tour-dots span.on {
  background: var(--p-primary-color);
}
.tour-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}
.tour-actions .grow {
  flex: 1;
}
@media (prefers-reduced-motion: reduce) {
  .tour-hole {
    transition: none;
  }
}
</style>
