import { defineAsyncComponent } from 'vue'

import { useMainStore } from '../../stores/main.js'

let MainStore
const setup = function() {
    MainStore = useMainStore()
}

const created = async function () {
}

const computed = {
    cockpitDetailClass() {
		return this.theme + '-cockpit-detail-' + this.themeVariety
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
    currentComponent() {
        let selectedComponent = MainStore.getSelectedMenuKey
        switch (selectedComponent) {
            case 'dashboard':
                return defineAsyncComponent(() => import('../../components/cockpit/Dashboard.vue'))
            case 'list-workflows':
                return defineAsyncComponent(() => import('../../components/cockpit/ListWorkflows.vue'))
            case 'workflow-editor':
                return defineAsyncComponent(() => import('../../components/cockpit/WorkflowEditor.vue'))
            default:
                return defineAsyncComponent(() => import('../../components/cockpit/Dashboard.vue'))
        }
    }
}

const watch = {
    panesResized() {
        this.$emit('panes-resized', this.panesResized)
    }
}

const mounted = async function() {
}

const methods = {
}

const destroyed = function() {
}

export default {
    props: [
        'panesResized',
    ],
	mixins: [
    ],
	components: {
    },
	directives: {},
	name: 'Detail',
    setup: setup,
    created: created,
    computed: computed,
    watch: watch,
    mounted: mounted,
    methods: methods,
    destroyed: destroyed,
    data() {
        return {
        }
    }
}
