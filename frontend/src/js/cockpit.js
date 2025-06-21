import {IsHostRunning, StopNode} from '../../wailsjs/go/main/App.js'

import initResizer from '../mixins/window-resizer.js'
import { useMainStore } from '../stores/main.js'

let MainStore
const setup = function() {
    MainStore = useMainStore()
}

const created = async function () {
    this.hostRunning = await this.isHostRunning()
}

const computed = {
    cockpitClass() {
		return this.theme + '-cockpit-' + this.themeVariety
	},
	locale() {
		return MainStore.getLocale
	},
	theme() {
		return MainStore.getTheme
	},
	themeVariety() {
		return MainStore.getThemeVariety
	},
}

const watch = {
    hostRunning() {
        this.$emit('host-running', this.hostRunning)
    }
}

const mounted = async function() {
    this.initResizer('.window-container', '.menu-container', '.main-container', '.resizer')
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
	mixins: [
        initResizer,
    ],
	components: {},
	directives: {},
	name: 'Cockpit',
    setup: setup,
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
