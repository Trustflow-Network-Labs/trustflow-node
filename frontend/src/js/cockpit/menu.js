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
                    label: 'Workflows & Jobs',
                    items: [
                        { label: 'Find services', command: function() {That.emitSelection('find-services')} },
                        { label: 'Request services', command: function() {That.emitSelection('request-services')} },
                        { label: 'List workflows', command: function() {That.emitSelection('list-workflows')} },
                        { label: 'Run workflow', command: function() {That.emitSelection('run-workflow')} }
                    ]
                },
                {
                    label: 'Configure node',
                    items: [
                    {
                        label: 'Blacklist',
                        items: [
                            { label: 'List nodes', command: function() {That.emitSelection('list-blacklist-nodes')} },
                            { label: 'Add node', command: function() {That.emitSelection('add-blacklist-node')} },
                            { label: 'Remove node', command: function() {That.emitSelection('remove-blacklist-node')} }
                        ]
                    },
                    {
                        label: 'Currencies',
                        items: [
                            { label: 'List currencies', command: function() {That.emitSelection('list-currencies')} },
                            { label: 'Add currency', command: function() {That.emitSelection('add-currency')} },
                            { label: 'Remove currency', command: function() {That.emitSelection('remove-currency')} }
                        ]
                    },
                    {
                        label: 'Resources',
                        items: [
                            { label: 'List resources', command: function() {That.emitSelection('list-resources')} },
                            { label: 'Add resource', command: function() {That.emitSelection('add-resource')} },
                            { label: 'Remove resource', command: function() {That.emitSelection('remove-resource')} },
                            { label: 'Set resource active', command: function() {That.emitSelection('set-resource-active')} },
                            { label: 'Set resource inactive', command: function() {That.emitSelection('set-resource-inactive')} }
                        ]
                    },
                    {
                        label: 'Services',
                        items: [
                            { label: 'List services', command: function() {That.emitSelection('list-services')} },
                            { label: 'Add service', command: function() {That.emitSelection('add-service')} },
                            { label: 'Remove service', command: function() {That.emitSelection('remove-service')} },
                            { label: 'Set service active', command: function() {That.emitSelection('set-service-active')} },
                            { label: 'Set service inactive', command: function() {That.emitSelection('set-service-inactive')} },
                            { label: 'Show service details', command: function() {That.emitSelection('show-service-details')} }
                        ]
                    },
                    {
                        label: 'Settings',
                        items: [
                            { label: 'List settings', command: function() {That.emitSelection('list-settings')} },
                            { label: 'Add setting', command: function() {That.emitSelection('add-setting')} },
                            { label: 'Remove setting', command: function() {That.emitSelection('remove-setting')} }
                        ]
                    }
                    ]
                }
            ],
            errorText: "",
        }
    }
}
