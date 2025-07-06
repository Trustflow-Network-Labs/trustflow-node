import {IsHostRunning, StopNode} from '../../../wailsjs/go/main/App.js'

import PanelMenu from 'primevue/panelmenu'

import { useMainStore } from '../../stores/main.js'

let MainStore, That
const setup = function() {
    MainStore = useMainStore()
}

const created = async function () {
    That = this
    this.hostRunning = await this.isHostRunning()
}

const computed = {
    cockpitMenuClass() {
		return this.theme + '-cockpit-menu-' + this.themeVariety
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
    },
}

const mounted = async function() {
}

const methods = {
    emitSelection(menuKey) {
        MainStore.setSelectedMenuKey(menuKey)
    },
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
    },
}

const destroyed = function() {
}

export default {
    props: [
    ],
	mixins: [
    ],
	components: {
        PanelMenu,
    },
	directives: {},
	name: 'Menu',
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
            menuItems: [
                {
                    label: 'Dashboard',
                    command: function() {That.emitSelection('dashboard')},
                },
                {
                    label: 'Workflows',
                    items: [
                        {
                            label: 'List workflows',
                            command: function() {That.emitSelection('list-workflows')},
                        },
                        {
                            label: 'Create Workflow',
                            command: function() {That.emitSelection('workflow-editor')},
                        },
                    ]
                },
                {
                    label: 'Configure node',
                    items: [
                        {
                            label: 'Blacklist',
                            command: function() {That.emitSelection('blacklist')},
                        },
                        {
                            label: 'Currencies',
                            command: function() {That.emitSelection('currencies')},
                        },
                        {
                            label: 'Resources',
                            command: function() {That.emitSelection('resources')},
                        },
                        {
                            label: 'Services',
                            command: function() {That.emitSelection('services')},
                        },
                        {
                            label: 'Settings',
                            command: function() {That.emitSelection('settings')},
                        }
                    ]
                }
            ],
            errorText: "",
        }
    }
}
