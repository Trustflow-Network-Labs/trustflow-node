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
    EventsOn('topicpeerconnectedlog-event', (peerId) => {
        let topicPeers = MainStore.getTopicPeers
        topicPeers.push(peerId)
        MainStore.setTopicPeers(topicPeers)
    })
    EventsOn('routingpeerconnectedlog-event', (peerId) => {
        let routingPeers = MainStore.getRoutingPeers
        routingPeers.push(peerId)
        MainStore.setRoutingPeers(routingPeers)
    })
    EventsOn('topicpeerdisconnectedlog-event', (peerId) => {
        let topicPeers = MainStore.getTopicPeers
        let topicPeersIndex = topicPeers.indexOf(peerId)
        if (topicPeersIndex > -1) {
            topicPeers.splice(topicPeersIndex, 1)
            MainStore.setTopicPeers(topicPeers)
        }
    })
    EventsOn('routingpeerdisconnectedlog-event', (peerId) => {
        let routingPeers = MainStore.getRoutingPeers
        let routingPeersIndex = routingPeers.indexOf(peerId)
        if (routingPeersIndex > -1) {
            routingPeers.splice(routingPeersIndex, 1)
            MainStore.setRoutingPeers(routingPeers)
        }
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
    EventsOff('topicpeerconnectedlog-event')
    EventsOff('routingpeerconnectedlog-event')
    EventsOff('topicpeerdisconnectedlog-event')
    EventsOff('routingpeerdisconnectedlog-event')
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
