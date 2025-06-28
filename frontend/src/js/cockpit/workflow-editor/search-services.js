import { FindServices } from '../../../../wailsjs/go/main/App'

import { useMainStore } from '../../../stores/main.js'

import SearchResult from '../../../components/cockpit/workflow-editor/SearchResult.vue'

import InputGroup from 'primevue/inputgroup';
import InputText from 'primevue/inputtext';
import InputGroupAddon from 'primevue/inputgroupaddon';
import Menu from 'primevue/menu';
import Button from 'primevue/button';

let MainStore, That
const setup = function() {
    MainStore = useMainStore()
}

const created = async function () {
    That = this
}

const computed = {
    cockpitWorkflowEditorSearchServicesClass() {
		return this.theme + '-cockpit-workflow-editor-search-services-' + this.themeVariety
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
    serviceOffer() {
        return MainStore.getServiceOffer
    }
}

const watch = {
    serviceOffer() {
        this.serviceOffers.push(this.serviceOffer)
    },
}

const mounted = function() {
}

const methods = {
    toggleSearchServiceTypes(event) {
        this.$refs["menu"].toggle(event)
    },
    toggleWindow(win) {
        switch (win) {
            case 'search-services':
                this.searchServicesWindowMinimized = !this.searchServicesWindowMinimized
                break
            default:
        }
    },
    async findServices({item}){
        this.serviceOffers.length = 0
        await FindServices(this.searchServicesPhrases, item.id)
    },
	dragStartFunc(event, service) {
        MainStore.setSelectedService(service)
/*
        let draggingEl = document.createElement('div')
		draggingEl.className = 'dragging-service-box'
		draggingEl.id = "drag-ghost"
		draggingEl.textNode = "Dragging"
		draggingEl.style.position = "absolute"
		draggingEl.style.top = "-1000px"
		this.workflowEditorEl.appendChild(draggingEl)
	  
		event.dataTransfer.setDragImage(draggingEl, 80, 50)
		event.dataTransfer.dropEffect = "copy"
*/
	},
	dragEndFunc(event) {
        event.preventDefault()
        // ignore
        MainStore.setSelectedService(null)

	},
	dragOverFunc(event) {
        event.preventDefault()
        // ignore
	},
	dropFunc(event) {
        event.preventDefault()
        // ignore
        MainStore.setSelectedService(null)
	},
}

const destroyed = function() {
}

export default {
    props: [
        'workflowEditorEl',
    ],
	mixins: [
    ],
	components: {
        SearchResult,
        InputGroup,
        InputText,
        InputGroupAddon,
        Menu,
        Button,
    },
	directives: {},
	name: 'WorkflowEditorSearchServices',
    setup: setup,
    created: created,
    computed: computed,
    watch: watch,
    mounted: mounted,
    methods: methods,
    destroyed: destroyed,
    data() {
        return {
            searchServicesPhrases: "",
            searchServicesTypes: [
                {
                    id: '',
                    label: 'Any',
                    icon: 'pi pi-asterisk',
                    command: async (event) => {
                        await That.findServices(event)
                    },
                },
                {
                    separator: true
                },
                {
                    id: 'DATA',
                    label: 'Data',
                    icon: 'pi pi-file',
                    command: async (event) => {
                        await That.findServices(event)
                    },
                },
                {
                    id: 'DOCKER EXECUTION ENVIRONMENT,STANDALONE EXECUTABLE',
                    label: 'Functions',
                    icon: 'pi pi-code',
                    command: async(event) => {
                        await That.findServices(event)
                    },
                },
            ],
            searchServicesWindowMinimized: false,
            serviceOffers: [],
        }
    }
}
