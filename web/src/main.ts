import { createApp } from 'vue'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import Tooltip from 'primevue/tooltip'
import { bootTheme } from './themes/apply'
import 'primeicons/primeicons.css'
import './styles.css'

import App from './App.vue'
import { router } from './router'
import { i18n } from './i18n'
import { setUnauthorizedHandler } from './lib/api'
import { useAuthStore } from './stores/auth'

// paints this device's last theme and picks its preset (see src/themes)
const boot = bootTheme()

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)
app.use(i18n)
app.use(PrimeVue, {
  theme: {
    preset: boot.preset,
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
