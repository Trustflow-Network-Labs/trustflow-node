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
    this.initGrid(this.snapXPoints, this.snapYPoints, this.gridBoxXSize, this.gridBoxYSize, this.gridSnapGravity)
}

const methods = {
    initGrid(x, y, w, h) {
        let workFlowEl = this.$refs['workflowEditor']
        workFlowEl.style.setProperty(`--grid-x-size`, `${this.gridXRelSize * 100}%`)
        workFlowEl.style.setProperty(`--grid-y-size`, `${this.gridYRelSize * 100}%`)
        workFlowEl.style.setProperty(`--grid-box-x-size`, `${this.gridBoxXSize}px`)
        workFlowEl.style.setProperty(`--grid-box-y-size`, `${this.gridBoxYSize}px`)
        workFlowEl.style.setProperty(`--dash-length`, `${this.dashLength}px`)
        workFlowEl.style.setProperty(`--grid-width`, `${this.dashWidth}px`)
        workFlowEl.style.setProperty(`--dash-color`, this.dashColor)
        workFlowEl.style.setProperty(`--background`, this.backgroundColor)

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

        if(event.target.className.indexOf('grid') == -1)
            return

        let service = MainStore.getPickedService
        if (service == null)
            return

        this.serviceCards.push({
            index: this.serviceCards.length,
            type: 'ServiceCard',
            props: {
                serviceCardId: this.serviceCards.length,
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
                let x = Math.round(event.x/this.gridBoxXSize)*this.gridBoxXSize - this.gridBoxXSize
    			let y = Math.round(event.y/this.gridBoxYSize)*this.gridBoxYSize - this.gridBoxYSize
                serviceCardDraggable.left = x
                serviceCardDraggable.top = y

                serviceCardDraggable.snap = {
                    targets: That.gridSnapTargets, gravity: That.gridSnapGravity
                }
            }
        })

        MainStore.setPickedService(null)
	},
    removeServiceCard(index) {
        let removeIndex = -1
        for (let i = 0; i < this.serviceCards.length; i++) {
            if (this.serviceCards[i].index == index) {
                removeIndex = i
                break
            } 
            
        }
        if (removeIndex >= 0) 
            this.serviceCards.splice(removeIndex, 1)
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
            gridBoxXSize: 160,
            gridBoxYSize: 100,
            gridXRelSize: 10,
            gridYRelSize: 10,
            dashLength: 5,
            dashWidth: 1,
            dashColor: 'rgba(205, 81, 36, .5)',
            backgroundColor: 'rgb(27, 38, 54)',
            snapToGrid: false,
			gridSnapGravity: 80,
			gridSnapTargets: [],
            serviceCards: [],
       }
    }
}
