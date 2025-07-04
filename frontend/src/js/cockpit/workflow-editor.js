import { AddWorkflow, AddWorkflowJob, RemoveWorkflowJob } from '../../../wailsjs/go/main/App'

import { useMainStore } from '../../stores/main.js'

import WorkflowTools from '../../components/cockpit/workflow-editor/WorkflowTools.vue'
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
    serviceCards: {
        handler(current, before) {
            for (let i = 0; i < this.serviceCards.length; i++) {
                const serviceCard = this.serviceCards[i]
                const id = serviceCard.id
                if (this.draggableServiceCards[id] != null) {
                    this.draggableServiceCards[id].remove()
                    delete this.draggableServiceCards[id]
                }
                this.$nextTick(() => {
                    That.initDraggableServiceCard(serviceCard)
                })
            }
        },
        deep: true,
		immediate: false,
    }
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

        this.addServiceCard(event)

        MainStore.setPickedService(null)
	},
    async addServiceCard(event) {
        if(event.target.className.indexOf('grid') == -1)
            return

        let service = MainStore.getPickedService
        if (service == null)
            return

        let x = event.x
        let y = event.y
        if (this.snapToGrid) {
            x = Math.round(x/this.gridBoxXSize)*this.gridBoxXSize - this.gridBoxXSize
            y = Math.round(y/this.gridBoxYSize)*this.gridBoxYSize - this.gridBoxYSize
        }

        let id = this.serviceCards.length
        let serviceCard = {
            id: id,
            workflowId: this.workflowId,
            workflowJobId: null,
            type: 'ServiceCard',
            props: {
                serviceCardId: id,
                service: service,
            },
            coords: {
                x: x,
                y, y,
            }
        }

        let name = this.$refs['workflowTools'].workflowName
        let description = this.$refs['workflowTools'].workflowDescription

        if (this.workflowId == null) {
            // Add workflow
            let response = await AddWorkflow(name, description, service.node_id, service.id, 0, "")
            if (response.error != null && response.error != "") {
                // TODO, print error
                console.log(response.error)
                return
            }
            console.log(response, typeof response)
            this.workflowId = response.workflow_id
            this.workflowJobsIds = response.workflow_jobs_ids
        }
        else {
            // Add workflow job
            let response = await AddWorkflowJob(this.workflowId, service.node_id, service.id, 0, "")
            if (response.error != null && response.error != "") {
                // TODO, print error
                console.log(response.error)
                return
            }
            this.workflowJobsIds.push(...response.workflow_jobs_ids)
        }

        serviceCard.workflowId = this.workflowId
        serviceCard.workflowJobId = this.workflowJobsIds[this.workflowJobsIds.length-1]
        this.serviceCards.push(serviceCard)
    },
    initDraggableServiceCard(serviceCard) {
        const serviceCardRef = `serviceCard${serviceCard.id}`
        const serviceCardEl = this.$refs[serviceCardRef][0].$el

        this.draggableServiceCards[serviceCard.id] = new PlainDraggable(serviceCardEl, {
            autoScroll: true,
        })
        this.draggableServiceCards[serviceCard.id].left = serviceCard.coords.x
        this.draggableServiceCards[serviceCard.id].top = serviceCard.coords.y

        if (this.snapToGrid) {
            this.draggableServiceCards[serviceCard.id].snap = {
                targets: this.gridSnapTargets, gravity: this.gridSnapGravity
            }
        }

        let serviceCardIndex = this.findServiceCardIndex(serviceCard.id)
        this.draggableServiceCards[serviceCard.id].onDragEnd = (pos) => {
            if (serviceCardIndex >= 0) {
                That.serviceCards[serviceCardIndex].coords = {
                    x: pos.left,
                    y: pos.top,
                }
            }
        }
    },
    findServiceCardIndex(id) {
        let serviceCardIndex = -1
        for (let i = 0; i < this.serviceCards.length; i++) {
            if (this.serviceCards[i].id == id) {
                serviceCardIndex = i
                break
            } 
        }
        return serviceCardIndex
    },
    async removeServiceCard(id) {
        // Find service card
        let removeIndex = -1
        for (let i = 0; i < this.serviceCards.length; i++) {
            if (this.serviceCards[i].id == id) {
                removeIndex = i
                break
            } 
            
        }
        if (removeIndex >= 0) {
            // Remove workflow job
            let err = await RemoveWorkflowJob(this.serviceCards[removeIndex].workflowJobId)
            if (err != null && err != "") {
                // TODO, print error
                console.log(err)
                return
            }

            // Remove draggable object
            if (this.draggableServiceCards[id] != null) {
                this.draggableServiceCards[id].remove()
                delete this.draggableServiceCards[id]
            }

            // Remove service card
            this.serviceCards.splice(removeIndex, 1)
        }
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
        WorkflowTools,
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
            draggableServiceCards: {},
            workflowId: null,
            workflowJobsIds: [],
       }
    }
}
