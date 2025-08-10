import Landing from '../components/Landing.vue'
import Cockpit from '../components/Cockpit.vue'

import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime'
import { NotifyFrontendReady } from '../../wailsjs/go/main/App'

import { useMainStore } from '../stores/main.js'

let MainStore
const setup = function() {
    MainStore = useMainStore()
}

const created = async function () {}

const computed = {
    hostRunning() {
		return MainStore.getHostRunning
    }
}

const watch = {}

const mounted = async function() {
    EventsOn('syslog-event', (msg) => {
        let appLogs = MainStore.getAppLogs
        appLogs.push(msg)
        MainStore.setAppLogs(appLogs)
    })
    EventsOn('exitlog-event', (msg) => {
        let exitLogs = MainStore.getExitLogs
        exitLogs.push(msg)
        MainStore.setExitLogs(exitLogs)
    })
    EventsOn('sysconfirm-event', (question) => {
        MainStore.setAppConfirm(question)
    })
    EventsOn('dependenciesready-event', (appCanStart) => {
        MainStore.setAppCanStart(appCanStart)
    })
    EventsOn('serviceofferlog-event', (serviceOffer) => {
        MainStore.setServiceOffer(serviceOffer)
    })
    EventsOn('hostrunninglog-event', (running) => {
        MainStore.setHostRunning(running)
    })

    // ✅ Tell backend we're ready
    await NotifyFrontendReady()
}

const methods = {}

const unmounted = function() {
    EventsOff('syslog-event')
    EventsOff('exitlog-event')
    EventsOff('sysconfirm-event')
    EventsOff('dependenciesready-event')
    EventsOff('serviceofferlog-event')
    EventsOff('hostrunninglog-event')
}

const destroyed = function() {
}

export default {
    props: [],
	mixins: [],
	components: {
		Landing,
        Cockpit
	},
	directives: {},
	name: 'App',
    setup: setup,
    created: created,
    computed: computed,
    watch: watch,
    mounted: mounted,
    methods: methods,
    unmounted: unmounted,
    destroyed: destroyed,
    data() {
        return {
        }
    }
}
