import {IsHostRunning, StartNode} from '../../wailsjs/go/main/App'

const created = async function () {
    this.hostRunning = await this.isHostRunning()
}

const computed = {}

const watch = {
    hostRunning() {
        this.$emit('host-running', this.hostRunning)
    }
}

const mounted = async function() {
}

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

const destroyed = function() {
}

export default {
	mixins: [],
	components: {},
	directives: {},
	name: 'Landing',
    created: created,
    computed: computed,
    watch: watch,
    mounted: mounted,
    methods: methods,
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
