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
                    icon: 'pi pi-gauge',
                    label: 'Dashboard',
                    command: function() {That.emitSelection('dashboard')},
                },
                {
                    icon: 'pi pi-wave-pulse',
                    label: 'Workflows',
                    items: [
                        {
                            icon: 'pi pi-list',
                            label: 'List workflows',
                            command: function() {That.emitSelection('list-workflows')},
                        },
                        {
                            icon: 'pi pi-receipt',
                            label: 'Create Workflow',
                            command: function() {That.emitSelection('workflow-editor')},
                        },
                    ]
                },
                {
                    icon: 'pi pi-cog',
                    label: 'Configure node',
                    items: [
                        {
                            icon: 'pi pi-shield',
                            label: 'Blacklist',
                            command: function() {That.emitSelection('blacklist')},
                        },
                        {
                            icon: 'pi pi-dollar',
                            label: 'Currencies',
                            command: function() {That.emitSelection('currencies')},
                        },
                        {
                            icon: 'pi pi-database',
                            label: 'Resources',
                            command: function() {That.emitSelection('resources')},
                        },
                        {
                            icon: 'pi pi-shopping-cart',
                            label: 'Services',
                            command: function() {That.emitSelection('services')},
                        },
                        {
                            icon: 'pi pi-sliders-h',
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
