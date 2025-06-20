import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n/dist/vue-i18n.cjs'

import App from './App.vue'
import './style.css';

import PrimeVue from 'primevue/config'
import Aura from '@primeuix/themes/aura'
import ConfirmationService from 'primevue/confirmationservice'
import 'primeicons/primeicons.css'

import Locale_en_GB from './locales/en_GB.js'

const messages = {
	'en_GB': Locale_en_GB
}

const i18n = createI18n({
	locale: 'en_GB',
	fallbackLocale: 'en_GB',
	messages
})

const app = createApp(App)
const pinia = createPinia()

app.use(PrimeVue, {
    theme: {
        preset: Aura
    }
})
app.use(pinia)
app.use(i18n)
app.use(ConfirmationService)

app.mount('#app')
