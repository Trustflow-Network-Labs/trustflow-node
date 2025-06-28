import { useMainStore } from '../../stores/main.js'

import SearchServices from '../../components/cockpit/workflow-editor/SearchServices.vue'
import ServiceCard from '../../components/cockpit/workflow-editor/ServiceCard.vue'

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
    initGrid(x, y, w, h) {
        let gridEl = this.$refs['workflowEditor']
        gridEl.style.setProperty(`--grid-size-x`, `${this.gridSizeX}px`)
        gridEl.style.setProperty(`--grid-size-y`, `${this.gridSizeY}px`)
        gridEl.style.setProperty(`--dash-length`, `${this.dashLength}px`)
        gridEl.style.setProperty(`--grid-width`, `${this.dashWidth}px`)
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

        let service = MainStore.getSelectedService
        if (service == null)
            return

        this.serviceCards.push({
            type: 'ServiceCard',
            props: {
                service: service,
            }
        })

        this.$nextTick(() => {
            let serviceCardRef = `serviceCard${That.serviceCards.length-1}`
            let serviceCardEl = That.$refs[serviceCardRef][0].$el

            let serviceCardDraggable = new PlainDraggable(serviceCardEl)
            serviceCardDraggable.left = event.x
            serviceCardDraggable.top = event.y

            if (That.snapToGrid) {
                let x = Math.round(event.x/this.gridSizeX)*this.gridSizeX - this.gridSizeX
    			let y = Math.round(event.y/this.gridSizeY)*this.gridSizeY - this.gridSizeY
                serviceCardDraggable.left = x
                serviceCardDraggable.top = y

                serviceCardDraggable.snap = {
                    targets: That.gridSnapTargets, gravity: That.gridSnapGravity
                }
            }


        })

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
        ServiceCard,
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
            snapXPoints: 100,
            snapYPoints: 100,
            gridSizeX: 160,
            gridSizeY: 100,
            dashLength: 5,
            dashWidth: 1,
            dashColor: 'rgba(205, 81, 36, .5)',
            backgroundColor: 'rgb(27, 38, 54)',
            snapToGrid: true,
			gridSnapGravity: 80,
			gridSnapTargets: [],
            serviceCards: [],
       }
    }
}
