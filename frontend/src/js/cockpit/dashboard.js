import { useMainStore } from '../../stores/main.js'

let MainStore
const setup = function() {
    MainStore = useMainStore()
}

const created = async function () {
}

const computed = {
    cockpitDashboardClass() {
		return this.theme + '-cockpit-dashboard-' + this.themeVariety
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
}

const mounted = async function() {
}

const methods = {
}

const destroyed = function() {
}

export default {
    props: [
    ],
	mixins: [
    ],
	components: {
    },
	directives: {},
	name: 'Dashboard',
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
