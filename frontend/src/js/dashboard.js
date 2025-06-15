import {IsHostRunning, StopNode} from '../../wailsjs/go/main/App'

const created = async function () {
    this.hostRunning = await this.isHostRunning()
}

const computed = {
    dashboardClass() {
		return this.theme + '-dashboard-' + this.themeVariety
	},
	locale() {
		return this.$store.getters['main/getLocale']
	},
	theme() {
		return this.$store.getters['main/getTheme']
	},
	themeVariety() {
		return this.$store.getters['main/getThemeVariety']
	},
}

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
        'appCanStart',
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
