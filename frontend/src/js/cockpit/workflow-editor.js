import SearchServices from '../../components/cockpit/workflow-editor/SearchServices.vue'
import { useMainStore } from '../../stores/main.js'

import PlainDraggable from "plain-draggable"
import LeaderLine from "leader-line-new"

let MainStore, That
const setup = function() {
    MainStore = useMainStore()
}

const created = async function () {
    That = this
}

const computed = {
    cockpitWorkflowEditorClass() {
		return this.theme + '-cockpit-workflow-editor-' + this.themeVariety
	},
	locale() {
		return MainStore.getLocale
	},
	theme() {
		return MainStore.getTheme
	},
	themeVariety() {
		return MainStore.getThemeVariety
	}
}

const watch = {
    panesResized() {
        if (this.panesResized == true) {
        }
    },
}

const mounted = function() {
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
        SearchServices,
    },
	directives: {},
	name: 'WorkflowEditor',
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
