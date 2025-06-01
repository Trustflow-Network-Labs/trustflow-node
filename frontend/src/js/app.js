import Landing from '../components/Landing.vue'
import Dashboard from '../components/Dashboard.vue'

import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime'

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
}

const methods = {}

const unmounted = function() {
    EventsOff('syslog-event')
    EventsOff('exitlog-event')
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
        }
    }
}
