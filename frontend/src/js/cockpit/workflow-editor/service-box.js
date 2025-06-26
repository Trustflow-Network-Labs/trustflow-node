import { useMainStore } from '../../../stores/main.js'
import shorten from '../../../mixins/text.js'
import copyToClipboard from '../../../mixins/clipboard.js'

let MainStore, That
const setup = function() {
    MainStore = useMainStore()
}

const created = async function () {
    That = this
}

const computed = {
    cockpitWorkflowEditorServiceBoxClass() {
		return this.theme + '-cockpit-workflow-editor-service-box-' + this.themeVariety
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
}

const mounted = function() {
}

const methods = {
}

const destroyed = function() {
}

export default {
    props: [
        'service',
    ],
	mixins: [
        shorten,
        copyToClipboard,
    ],
	components: {
    },
	directives: {},
	name: 'WorkflowEditorServiceBox',
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
