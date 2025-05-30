import {IsHostRunning, StartNode, StopNode} from '../../wailsjs/go/main/App'

const created = async function () {
    this.hostRunning = await this.isHostRunning()
}

const computed = {}

const watch = {}

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
    },
    async stopNode() {
        await StopNode()
        this.hostRunning = await this.isHostRunning()
    }
}

export default {
    created: created,
    computed: computed,
    watch: watch,
    mounted: mounted,
    methods: methods,
    data() {
        return {
            hostRunning: false,
            port: 30609,
            resultText: "Please enter node port number below 👇",
        }
    }
}
