import {IsHostRunning, StopNode} from '../../wailsjs/go/main/App'

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
    async stopNode() {
        await StopNode()
        this.hostRunning = await this.isHostRunning()
    }
}

const destroyed = function() {
}

export default {
	mixins: [],
	components: {},
	directives: {},
	name: 'Dashboard',
    created: created,
    computed: computed,
    watch: watch,
    mounted: mounted,
    methods: methods,
    destroyed: destroyed,
    data() {
        return {
            hostRunning: false,
        }
    }
}
