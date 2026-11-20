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
import { setPasswordChangeHandler, setUnauthorizedHandler } from './lib/api'
import { useAuthStore } from './stores/auth'
import { vField, vStack } from './directives'

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
  pt: {
    // a password field is a textbox, not a popup button (aria-expanded is not allowed on it)
    password: { pcInputText: { root: { 'aria-expanded': undefined, 'aria-haspopup': undefined, 'aria-controls': undefined } } },
    // tree items are named by their content (as in the WAI-ARIA tree pattern); the expand
    // button is mouse-only (tabindex -1), keyboard users use the arrow keys on the item
    tree: { node: { 'aria-label': undefined }, nodeToggleButton: { 'aria-hidden': 'true' } },
    // the selected state of a row is on the row (aria-selected); a checkbox may not carry it
    datatable: { column: { pcRowCheckbox: { root: { 'aria-selected': undefined } } } },
  },
})
app.use(ToastService)
app.use(ConfirmationService)
app.directive('tooltip', Tooltip)
app.directive('field', vField)
app.directive('stack', vStack)

setUnauthorizedHandler(() => {
  const auth = useAuthStore(pinia)
  auth.clear()
  const cur = router.currentRoute.value
  if (cur.name !== 'login') router.push({ name: 'login', query: { redirect: cur.fullPath } })
})

// an administrator reset the password: everything but /auth/* answers 403 password_change_required
setPasswordChangeHandler(() => {
  const auth = useAuthStore(pinia)
  if (auth.me?.user) auth.me.user.must_change_password = true
  if (router.currentRoute.value.name !== 'changePassword') router.push({ name: 'changePassword' })
})

app.mount('#app')
