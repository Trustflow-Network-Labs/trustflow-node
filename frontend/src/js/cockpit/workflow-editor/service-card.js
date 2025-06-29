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
    cockpitWorkflowEditorServiceCardClass() {
		return this.theme + '-cockpit-workflow-editor-service-card-' + this.themeVariety
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
    closeServiceCard() {
        this.$emit('close-service-card', this.serviceCardId)
    }
}

const destroyed = function() {
}

export default {
    props: [
        'serviceCardId',
        'service',
    ],
	mixins: [
        shorten,
        copyToClipboard,
    ],
	components: {
    },
	directives: {},
	name: 'WorkflowEditorServiceCard',
    setup: setup,
    created: created,
    computed: computed,
    watch: watch,
    mounted: mounted,
    methods: methods,
    destroyed: destroyed,
    data() {
        return {
            ready: false,
            free: false,
        }
    }
}
