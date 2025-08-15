import {IsHostRunning, StopNode} from '../../../wailsjs/go/main/App.js'

import PanelMenu from 'primevue/panelmenu'

import { useMainStore } from '../../stores/main.js'

let MainStore, That
const setup = function() {
    MainStore = useMainStore()
}

const created = async function () {
    That = this
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
    selectedMenuKey() {
        return MainStore.getSelectedMenuKey
    },
    menuItems() {
        return [
            {
                icon: 'pi pi-gauge',
                label: 'Dashboard',
                key: 'dashboard',
                command: function() {That.emitSelection('dashboard')},
                class: ((this.selectedMenuKey == 'dashboard') ? 'active-menu-item' : ''),
            },
            {
                icon: 'pi pi-wave-pulse',
                label: 'Workflows',
                key: 'workflows',
                command: function() {That.emitSelection('workflows')},
                class: ((this.selectedMenuKey == 'workflows') ? 'active-menu-item' : ''),
                items: [
                    {
                        icon: 'pi pi-list',
                        label: 'List workflows',
                        key: 'list-workflows',
                        command: function() {That.emitSelection('list-workflows')},
                        class: ((this.selectedMenuKey == 'list-workflows') ? 'active-menu-item' : ''),
                    },
                    {
                        icon: 'pi pi-receipt',
                        label: 'Create Workflow',
                        key: 'workflow-editor',
                        command: function() {That.emitSelection('workflow-editor')},
                        class: ((this.selectedMenuKey == 'workflow-editor') ? 'active-menu-item' : ''),
                    },
                ]
            },
            {
                icon: 'pi pi-cog',
                label: 'Configure node',
                key: 'configure-node',
                command: function() {That.emitSelection('configure-node')},
                class: ((this.selectedMenuKey == 'configure-node') ? 'active-menu-item' : ''),
                items: [
                    {
                        icon: 'pi pi-lock',
                        label: 'Blacklist',
                        key: 'blacklist',
                        command: function() {That.emitSelection('blacklist')},
                        class: ((this.selectedMenuKey == 'blacklist') ? 'active-menu-item' : ''),
                    },
                    {
                        icon: 'pi pi-dollar',
                        label: 'Currencies',
                        key: 'currencies',
                        command: function() {That.emitSelection('currencies')},
                        class: ((this.selectedMenuKey == 'currencies') ? 'active-menu-item' : ''),
                    },
                    {
                        icon: 'pi pi-database',
                        label: 'Resources',
                        key: 'resources',
                        command: function() {That.emitSelection('resources')},
                        class: ((this.selectedMenuKey == 'resources') ? 'active-menu-item' : ''),
                    },
                    {
                        icon: 'pi pi-shopping-cart',
                        label: 'Services',
                        key: 'services',
                        command: function() {That.emitSelection('services')},
                        class: ((this.selectedMenuKey == 'services') ? 'active-menu-item' : ''),
                    },
                    {
                        icon: 'pi pi-sliders-h',
                        label: 'Settings',
                        key: 'settings',
                        command: function() {That.emitSelection('settings')},
                        class: ((this.selectedMenuKey == 'settings') ? 'active-menu-item' : ''),
                    }
                ]
            }
        ]
    }
}

const watch = {
}

const mounted = async function() {
}

const methods = {
    emitSelection(menuKey) {
        MainStore.setSelectedMenuKey(menuKey)
        this.toggleExpandedMenuItems()
    },
    toggleExpandedMenuItems() {
        if (this.expandedMenuKeys[this.selectedMenuKey] == undefined)
            this.expandedMenuKeys[this.selectedMenuKey] = true
        else
            this.expandedMenuKeys[this.selectedMenuKey] = !this.expandedMenuKeys[this.selectedMenuKey]
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
        const running = await this.isHostRunning()
        MainStore.setHostRunning(running)
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
            errorText: "",
            expandedMenuKeys: {},
        }
    }
}
