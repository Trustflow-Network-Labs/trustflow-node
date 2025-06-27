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
    this.workflowEditorEl = this.$refs.workflowEditor
    this.initGrid(this.snapXPoints, this.snapYPoints, this.gridSizeX, this.gridSizeY, this.gridSnapGravity)
}

const methods = {
    initGrid(x, y, w, h, g) {
        let gridEl = this.$refs['workflowEditor']
        gridEl.style.setProperty(`--grid-size-x`, this.gridSizeX)
        gridEl.style.setProperty(`--grid-size-y`, this.gridSizeY)
        gridEl.style.setProperty(`--dash-length`, this.dashLength)
        gridEl.style.setProperty(`--grid-width`, this.dashWidth)
        gridEl.style.setProperty(`--dash-color`, this.dashColor)
        gridEl.style.setProperty(`--background`, this.backgroundColor)

        this.initGridSnap(x, y, w, h)
    },
    initGridSnap(x, y, w, h) {
    	for (let i = 0; i < x; i++) {
            for (let j = 0; j < y; j++) {
                this.gridSnapTargets.push({x: i * w, y: j * h})
            }
        }
    },
	dragEndFunc(event) {
        event.preventDefault()
	},
	dragOverFunc(event) {
        event.preventDefault()
	},
	dropFunc(event) {
        event.preventDefault()
        MainStore.setSelectedService(null)
	},
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
            workflowEditorEl: null,
            snapXPoints: 160,
            snapYPoints: 100,
            gridSizeX: '160px',
            gridSizeY: '100px',
            dashLength: '5px',
            dashWidth: '1px',
            dashColor: 'rgba(205, 81, 36, .5)',
            backgroundColor: 'rgb(27, 38, 54)',
			gridSnapGravity: 60,
			gridSnapTargets: [],
       }
    }
}
