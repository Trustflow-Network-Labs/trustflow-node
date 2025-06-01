import {IsHostRunning, StartNode} from '../../wailsjs/go/main/App'

const created = async function () {
    this.hostRunning = await this.isHostRunning()
}

const computed = {
    appCanStart() {
        return this.exitLogs.length == 0
    }
}

const watch = {
    hostRunning() {
        this.$emit('host-running', this.hostRunning)
    },
    appLogs: {
        handler() {
            this.$nextTick(() => {
                let lastIndex = this.appLogs.length - 1;
                if (lastIndex >= 0 && this.$refs.log) {
                    const el = this.$refs.log[lastIndex];
                    if (el && el.scrollIntoView) {
                        el.scrollIntoView();
                    }
                }
            })
        },
        deep: true,
        immediate: true,
    }
}

const mounted = async function() {}

const methods = {
    async isHostRunning() {
        return await IsHostRunning()
    },
    async startNode() {
        // Start node and don't wait
        // for complete initialization
        StartNode(this.port)

        // Use interval and max retries
        // to check is node started
        let maxRetries = 10
        let retries = 0
        let nodeStarted = window.setInterval(async () => {
            this.hostRunning = await this.isHostRunning()
            if (this.hostRunning || retries >= maxRetries-1) {
                clearInterval(nodeStarted)
            }
            retries++
        }, 500)
    }
}

const unmounted = function() {}

const destroyed = function() {
}

export default {
    props: [
        'appLogs',
        'exitLogs',
    ],
	mixins: [],
	components: {},
	directives: {},
	name: 'Landing',
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
            port: 30609,
            startNodeText: "Enter the node port number below 👇",
            startNodeTextHelper: "The Trustflow Node will use this port and the next five ports for various communication protocols (TCP, QUIC, WS, etc.).",
            trustflowNodeText: "Powering the decentralized backend of tomorrow  ",
            trustflowNodeTextHelper: "Trustflow Node is an open-source, decentralized framework for trust-based data exchange and secure computation across distributed networks. It orchestrates decentralized Docker and WASM runtimes, enabling robust, verifiable, and scalable infrastructure for the next generation of decentralized applications.",
        }
    }
}
