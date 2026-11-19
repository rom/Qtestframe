import { createApp } from 'vue'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import Tooltip from 'primevue/tooltip'
import Aura from '@primevue/themes/aura'
import { definePreset } from '@primevue/themes'
import 'primeicons/primeicons.css'
import './styles.css'

import App from './App.vue'
import { router } from './router'
import { setUnauthorizedHandler } from './lib/api'
import { useAuthStore } from './stores/auth'

const Preset = definePreset(Aura, {
  semantic: {
    primary: {
      50: '{indigo.50}',
      100: '{indigo.100}',
      200: '{indigo.200}',
      300: '{indigo.300}',
      400: '{indigo.400}',
      500: '{indigo.500}',
      600: '{indigo.600}',
      700: '{indigo.700}',
      800: '{indigo.800}',
      900: '{indigo.900}',
      950: '{indigo.950}',
    },
  },
})

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)
app.use(PrimeVue, {
  theme: {
    preset: Preset,
    options: { darkModeSelector: '.app-dark', cssLayer: false },
  },
  ripple: false,
})
app.use(ToastService)
app.use(ConfirmationService)
app.directive('tooltip', Tooltip)

setUnauthorizedHandler(() => {
  const auth = useAuthStore(pinia)
  auth.clear()
  const cur = router.currentRoute.value
  if (cur.name !== 'login') router.push({ name: 'login', query: { redirect: cur.fullPath } })
})

app.mount('#app')
