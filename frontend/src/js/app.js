import Landing from '../components/Landing.vue'
import Dashboard from '../components/Dashboard.vue'

const created = async function () {}

const computed = {}

const watch = {}

const mounted = async function() {
}

const methods = {}

const destroyed = function() {
}

export default {
	mixins: [],
	components: {
		Landing,
        Dashboard
	},
	directives: {},
	name: 'App',
    created: created,
    computed: computed,
    watch: watch,
    mounted: mounted,
    methods: methods,
    destroyed: destroyed,
    data() {
        return {
            hostRunning: false
        }
    }
}
