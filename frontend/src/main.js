import {createApp} from 'vue'
import App from './App.vue'
import './style.css';


import PrimeVue from 'primevue/config'
import Aura from '@primeuix/themes/aura'
import ConfirmationService from 'primevue/confirmationservice'
import 'primeicons/primeicons.css'

const app = createApp(App)

app.use(PrimeVue, {
    theme: {
        preset: Aura
    }
})
app.use(ConfirmationService)

app.mount('#app')
