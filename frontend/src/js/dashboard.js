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
        let err
        this.errorText = ""
        await (err = StopNode())
        if (err != null && err != "") {
            this.errorText = err
        }
        this.hostRunning = await this.isHostRunning()
    }
}

const destroyed = function() {
}

export default {
    props: [
        'appLogs',
        'exitLogs',
        'appConfirm',
    ],
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
            errorText: "",
        }
    }
}
