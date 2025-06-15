import {createApp} from 'vue'
import { createStore  } from 'vuex'

import App from './App.vue'
import './style.css';

import MainStore from './stores/main.js'

import PrimeVue from 'primevue/config'
import Aura from '@primeuix/themes/aura'
import ConfirmationService from 'primevue/confirmationservice'
import 'primeicons/primeicons.css'

const store = createStore({
	modules: {
		main: MainStore
	}
})

const app = createApp(App)

app.use(PrimeVue, {
    theme: {
        preset: Aura
    }
})
app.use(store)
app.use(ConfirmationService)

app.mount('#app')
