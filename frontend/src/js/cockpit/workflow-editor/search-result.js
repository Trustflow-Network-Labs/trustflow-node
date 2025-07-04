import { useMainStore } from '../../../stores/main.js'
import { textUtils } from '../../../mixins/text.js'
import copyToClipboard from '../../../mixins/clipboard.js'

let MainStore, That
const setup = function() {
    MainStore = useMainStore()
}

const created = async function () {
    That = this
}

const computed = {
    cockpitWorkflowEditorSearchResultClass() {
		return this.theme + '-cockpit-workflow-editor-search-result-' + this.themeVariety
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
        textUtils,
        copyToClipboard,
    ],
	components: {
    },
	directives: {},
	name: 'WorkflowEditorSearchResult',
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
