import Menu from '../components/cockpit/Menu.vue'
import Detail from '../components/cockpit/Detail.vue'

import initResizer from '../mixins/window-resizer.js'
import { useMainStore } from '../stores/main.js'

let MainStore, That
const setup = function() {
    MainStore = useMainStore()
}

const created = function () {
    That = this
}

const computed = {
    cockpitClass() {
		return this.theme + '-cockpit-' + this.themeVariety
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
}

const watch = {
}

const mounted = function() {
    this.initResizer('.window-container', '.menu-container', '.main-container', '.resizer',
        function() {
            That.panesResized = false
        },
        null
        , function() {
            That.panesResized = true
        })
}

const methods = {
}

const destroyed = function() {
}

export default {
    props: [
    ],
	mixins: [
        initResizer,
    ],
	components: {
        Menu,
        Detail,
    },
	directives: {},
	name: 'Cockpit',
    setup: setup,
    created: created,
    computed: computed,
    watch: watch,
    mounted: mounted,
    methods: methods,
    destroyed: destroyed,
    data() {
        return {
            panesResized: false,
        }
    }
}
