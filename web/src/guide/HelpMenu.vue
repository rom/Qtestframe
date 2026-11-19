<script setup lang="ts">
// Top-bar Help button: user guide, help for the current page, restart the
// intro tour. Also starts the tour on a user's first sign-in.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Menu from 'primevue/menu'
import { useAuthStore } from '@/stores/auth'
import IntroTour from './IntroTour.vue'
import { guidePageForRoute } from './context'
import { helpPath } from './links'
import { shouldStartTour } from './tour'

const { t } = useI18n()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const menu = ref<InstanceType<typeof Menu>>()
const tour = ref<InstanceType<typeof IntroTour>>()

const SHOWN_KEY = 'qtf.introShown'
function shownThisSession(): boolean {
  try {
    return sessionStorage.getItem(SHOWN_KEY) === '1'
  } catch {
    return false
  }
}
function markShown() {
  try {
    sessionStorage.setItem(SHOWN_KEY, '1')
  } catch {
    /* ignore */
  }
}

function startTour() {
  markShown()
  void tour.value?.start()
}

const contextPage = computed(() => (route.name === 'help' ? null : guidePageForRoute(route.name)))
const items = computed(() => [
  { label: t('guide.userGuide'), icon: 'pi pi-book', command: () => router.push(helpPath('index')) },
  ...(contextPage.value ? [{ label: t('guide.thisPage'), icon: 'pi pi-question-circle', command: () => router.push(helpPath(contextPage.value!)) }] : []),
  { separator: true },
  { label: t('guide.restartTour'), icon: 'pi pi-replay', command: () => setTimeout(startTour, 50) },
])

// first sign-in (or the tour was never finished): start it once the layout has settled
let timer: ReturnType<typeof setTimeout> | undefined
function maybeStart() {
  clearTimeout(timer)
  if (!shouldStartTour(auth.me) || shownThisSession()) return
  timer = setTimeout(() => {
    if (shouldStartTour(auth.me) && !shownThisSession() && route.name !== 'login') startTour()
  }, 900)
}
onMounted(maybeStart)
watch(() => auth.me?.user?.id, maybeStart)
</script>

<template>
  <Button
    class="qtf-help-button"
    icon="pi pi-question-circle"
    text
    rounded
    severity="secondary"
    :aria-label="t('guide.help')"
    aria-haspopup="menu"
    v-tooltip.bottom="t('guide.help')"
    @click="menu?.toggle($event)"
  />
  <Menu ref="menu" :model="items" popup :aria-label="t('guide.helpMenu')" />
  <IntroTour ref="tour" />
</template>
