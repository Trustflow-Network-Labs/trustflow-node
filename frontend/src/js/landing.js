import {IsHostRunning, StartNode, SetUserConfirmation} from '../../wailsjs/go/main/App'
import ConfirmDialog from 'primevue/confirmdialog'
import { useConfirm } from "primevue/useconfirm"
import ToggleButton from 'primevue/togglebutton'
import { useMainStore } from '../stores/main.js'

let Confirm, MainStore
const setup = function() {
    Confirm = useConfirm()
    MainStore = useMainStore()
}

const created = async function () {
    this.hostRunning = await this.isHostRunning()
}

const computed = {
    landingClass() {
		return this.theme + '-landing-' + this.themeVariety
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
    appCanStart() {
		return MainStore.getAppCanStart
    },
    appConfirm() {
		return MainStore.getAppConfirm
    },
    appLogs() {
		return MainStore.getAppLogs
    },
    exitLogs() {
		return MainStore.getExitLogs
    },
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
    },
    appConfirm: {
        handler() {
            if (this.appConfirm == "") {
                return
            }
            Confirm.require({
                message: this.appConfirm,
                header: this.$t("message.landing.logic.remove-confirmation"),
                icon: 'pi pi-exclamation-triangle',
                accept: async () => {
                    await SetUserConfirmation(true)
                },
                reject: async () => {
                    await SetUserConfirmation(false)
                }
            })
        },
        deep: true,
        immediate: true,
    },
    public() {
        if (!this.public)
            this.relay = false
    },
    relay() {
        if (this.relay)
            this.public = true
    },
}

const mounted = async function() {}

const methods = {
    async isHostRunning() {
        return await IsHostRunning()
    },
    async startNode() {
        // Start node and don't wait
        // for complete initialization
        StartNode(this.port, this.public, this.relay)

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
    plugins: [],
    props: [
    ],
	mixins: [],
	components: {
        ConfirmDialog,
        ToggleButton,
    },
	directives: {},
	name: 'Landing',
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
            hostRunning: false,
            port: 30609,
            public: false,
            relay: false,
            confirm: null,
        }
    }
}
