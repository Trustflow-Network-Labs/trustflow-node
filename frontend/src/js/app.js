import Landing from '../components/Landing.vue'
import Dashboard from '../components/Dashboard.vue'

import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime'
import { NotifyFrontendReady } from '../../wailsjs/go/main/App'

const created = async function () {}

const computed = {}

const watch = {}

const mounted = async function() {
    EventsOn('syslog-event', (msg) => {
        this.appLogs.push(msg)
    })
    EventsOn('exitlog-event', (msg) => {
        this.exitLogs.push(msg)
    })
    EventsOn('sysconfirm-event', (question) => {
        this.sysConfirm = question
    })
    EventsOn('dependenciesready-event', (ready) => {
        this.appCanStart = ready
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
}

const destroyed = function() {
}

export default {
    props: [],
	mixins: [],
	components: {
		Landing,
        Dashboard
	},
	directives: {},
	name: 'App',
    created: created,
    computed: computed,
    watch: watch,
    mounted: mounted,
    methods: methods,
    unmounted: unmounted,
    destroyed: destroyed,
    data() {
        return {
            hostRunning: false,
            appLogs: [],
            exitLogs: [],
            sysConfirm: "",
            appCanStart: false,
        }
    }
}
