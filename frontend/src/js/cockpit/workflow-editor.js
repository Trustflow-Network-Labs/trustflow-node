import {
    AddWorkflow,
    UpdateWorkflow,
    RemoveWorkflow,
    GetWorkflowJob,
    AddWorkflowJob,
    RemoveWorkflowJob,
    SetServiceCardGUIProps,
    SetWorkflowGUIProps,
} from '../../../wailsjs/go/main/App'

import { useMainStore } from '../../stores/main.js'

import { textUtils } from '../../mixins/text.js'

import WorkflowTools from '../../components/cockpit/workflow-editor/WorkflowTools.vue'
import ServiceCard from '../../components/cockpit/workflow-editor/ServiceCard.vue'

import PlainDraggable from "plain-draggable"
import LeaderLine from "leader-line-new"

import Toast from 'primevue/toast'
import { useToast } from 'primevue/usetoast'
import ConfirmDialog from 'primevue/confirmdialog'
import { useConfirm } from "primevue/useconfirm"

let MainStore, UseToast, UseConfirm, That
const setup = function() {
    MainStore = useMainStore()
    UseToast = useToast()
    UseConfirm = useConfirm()
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
    async workflowId() {
        await this.saveSnapToGridProp()
    },
    async snapToGrid() {
        await this.saveSnapToGridProp()
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

        this.initGridSnapPoints(x, y, w, h)
    },
    initGridSnapPoints(x, y, w, h) {
    	for (let i = 0; i < x; i++) {
            for (let j = 0; j < y; j++) {
                this.gridSnapTargets.push({x: i * w, y: j * h})
            }
        }
    },
    async saveSnapToGridProp() {
        if (this.workflowId == null)
            return
        const snapToGrid = (this.snapToGrid) ? 1 : 0
        let err = await SetWorkflowGUIProps(this.workflowId, snapToGrid)
        if (err != null && err != "") {
            // Print error
            UseToast.add({
                severity: "error",
                summary: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-props-not-saved"),
                detail: err,
                closable: 3000,
                life: null,
            })
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
                workflowJob: null,
            },
            coords: {
                x: x,
                y, y,
            }
        }

        let name = this.$refs['workflowTools'].workflowName
        let description = this.$refs['workflowTools'].workflowDescription

        if (name == "") {
            name = this.generateRandomName()
            this.$refs['workflowTools'].workflowName = name
        }

        if (service.entrypoint == null)
            service.entrypoint = []
        if (service.commands == null)
            service.commands = []
        if (service.interfaces == null)
            service.interfaces = []
        if (service.service_price_model == null)
            service.service_price_model = []

        if (this.workflowId == null) {
            // Add workflow
            let response = await AddWorkflow(
                name, description, service.node_id, service.id, service.name, service.description,
                service.type, service.entrypoint, service.commands, service.interfaces,
                service.service_price_model, service.last_seen, 0, "")
            if (response.error != null && response.error != "") {
                // Print error
                UseToast.add({
                    severity: "error",
                    summary: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-not-added"),
                    detail: response.error,
                    closable: true,
                    life: null,
                })
                return
            }
            this.workflowId = response.workflow_id
            this.workflowJobsIds = response.workflow_jobs_ids
            // Print confirmation
            UseToast.add({
                severity: "info",
                summary: this.$t("message.cockpit.detail.workflow-editor.logic.success"),
                detail: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-added"),
                closable: true,
                life: 3000,
            })
        }
        else {
            // Add workflow job
            let response = await AddWorkflowJob(this.workflowId, service.node_id, service.id,
                service.name, service.description, service.type, service.entrypoint, service.commands,
                service.interfaces, service.service_price_model, service.last_seen, 0, "")
            if (response.error != null && response.error != "") {
                // Print error
                UseToast.add({
                    severity: "error",
                    summary: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-job-not-added"),
                    detail: response.error,
                    closable: true,
                    life: null,
                })
                return
            }
            this.workflowJobsIds.push(...response.workflow_jobs_ids)
        }

        serviceCard.workflowId = this.workflowId
        serviceCard.workflowJobId = this.workflowJobsIds[this.workflowJobsIds.length-1]

        // Get newly added workflow job with ids
        let response = await GetWorkflowJob(serviceCard.workflowJobId)
        if (response.error != null && response.error != "") {
            // Print error
            UseToast.add({
                severity: "error",
                summary: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-job-not-loaded"),
                detail: response.error,
                closable: true,
                life: null,
            })
        }
        serviceCard.props.workflowJob = response.workflow_job

        this.serviceCards.push(serviceCard)

        // Update service card props in DB
        let err = await SetServiceCardGUIProps(serviceCard.workflowJobId, x, y)
        if (err != null && err != "") {
            // Print error
            UseToast.add({
                severity: "error",
                summary: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-service-card-coords-not-saved"),
                detail: err,
                closable: true,
                life: null,
            })
        }
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
        this.draggableServiceCards[serviceCard.id].onDragEnd = async (pos) => {
            if (serviceCardIndex >= 0) {
                let x = pos.left
                let y = pos.top

                // Update Service Card coordinates
                That.serviceCards[serviceCardIndex].coords = {
                    x: x,
                    y: y,
                }

                // Update service card props in DB
                let err = await SetServiceCardGUIProps(serviceCard.workflowJobId, x, y)
                if (err != null && err != "") {
                    // Print error
                    UseToast.add({
                        severity: "error",
                        summary: That.$t("message.cockpit.detail.workflow-editor.logic.workflow-service-card-coords-not-saved"),
                        detail: err,
                        closable: true,
                        life: null,
                    })
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
                // Print error
                UseToast.add({
                    severity: "error",
                    summary: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-job-not-removed"),
                    detail: err,
                    closable: true,
                    life: null,
                })
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
    async updateWorkflow() {
        let name = this.$refs['workflowTools'].workflowName
        let description = this.$refs['workflowTools'].workflowDescription

        if (name == "") {
            name = this.generateRandomName()
            this.$refs['workflowTools'].workflowName = name
        }

       if (!this.workflowId) {
            // Add workflow
            let response = await AddWorkflow(name, description, "", 0, "", "", "", [], [], [], [], "", 0, "")
            if (response.error != null && response.error != "") {
                // Print error
                UseToast.add({
                    severity: "error",
                    summary: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-not-added"),
                    detail: response.error,
                    closable: true,
                    life: null,
                })
                return
            }
            this.workflowId = response.workflow_id
            // Print confirmation
            UseToast.add({
                severity: "info",
                summary: this.$t("message.cockpit.detail.workflow-editor.logic.success"),
                detail: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-added"),
                closable: true,
                life: 3000,
            })
        }
        else {
            // Update workflow
            let err = await UpdateWorkflow(this.workflowId, name, description)
            if (err != null && err != "") {
                // Print error
                UseToast.add({
                    severity: "error",
                    summary: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-not-updated"),
                    detail: err,
                    closable: true,
                    life: null,
                })
                return
            }
            // Print confirmation
            UseToast.add({
                severity: "info",
                summary: this.$t("message.cockpit.detail.workflow-editor.logic.success"),
                detail: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-updated"),
                closable: true,
                life: 3000,
            })
        }
    },
    async removeWorkflow() {
        if (!this.workflowId) {
            UseToast.add({
                severity: "info",
                summary: this.$t("message.cockpit.detail.workflow-editor.logic.info"),
                detail: this.$t("message.cockpit.detail.workflow-editor.logic.no-active-workflow-to-remove"),
                closable: true,
                life: 3000,
            })
            return
        }

        UseConfirm.require({
            message: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-remove-confirmation-message"),
            header: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-remove-confirmation"),
            icon: 'pi pi-exclamation-triangle',
            accept: async () => {
                // Remove workflow
                let err = await RemoveWorkflow(this.workflowId)
                if (err != null && err != "") {
                    // Print error
                    UseToast.add({
                        severity: "error",
                        summary: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-not-removed"),
                        detail: err,
                        closable: true,
                        life: null,
                    })
                    return
                }
                // Print confirmation
                UseToast.add({
                    severity: "info",
                    summary: this.$t("message.cockpit.detail.workflow-editor.logic.success"),
                    detail: this.$t("message.cockpit.detail.workflow-editor.logic.workflow-removed"),
                    closable: true,
                    life: 3000,
                })
                // Reset vars
                this.workflowId = null
                this.workflowJobsIds.length = 0
                this.$refs['workflowTools'].workflowName = ""
                this.$refs['workflowTools'].workflowDescription = ""
                this.$refs['workflowTools'].searchServicesPhrases = ""
                this.$refs['workflowTools'].serviceOffers.length = 0
                this.serviceCards.length = 0
                for (const id in this.draggableServiceCards) {
                    if (Object.prototype.hasOwnProperty.call(this.draggableServiceCards, id)) {
                        this.draggableServiceCards[id].remove()
                        delete this.draggableServiceCards[id]
                    }
                }
            },
            reject: async () => {
            }
        })
    },
}

const destroyed = function() {
}

export default {
    props: [
    ],
	mixins: [
        textUtils,
    ],
	components: {
        WorkflowTools,
        ServiceCard,
        Toast,
        ConfirmDialog,
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
