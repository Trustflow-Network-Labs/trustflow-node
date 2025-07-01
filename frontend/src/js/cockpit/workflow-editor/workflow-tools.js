import { FindServices } from '../../../../wailsjs/go/main/App'

import { useMainStore } from '../../../stores/main.js'

import SearchResult from '../../../components/cockpit/workflow-editor/SearchResult.vue'

import InputGroup from 'primevue/inputgroup';
import InputText from 'primevue/inputtext';
import InputGroupAddon from 'primevue/inputgroupaddon';
import Menu from 'primevue/menu';
import Button from 'primevue/button';
import FloatLabel from 'primevue/floatlabel';
import Textarea from 'primevue/textarea';

let MainStore, That
const setup = function() {
    MainStore = useMainStore()
}

const created = async function () {
    That = this
}

const computed = {
    cockpitWorkflowEditorWorkflowToolsClass() {
		return this.theme + '-cockpit-workflow-editor-workflow-tools-' + this.themeVariety
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
            case 'workflow-details':
                this.workflowDetailsWindowMinimized = !this.workflowDetailsWindowMinimized
                break
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
        MainStore.setPickedService(service)
	},
	dragEndFunc(event) {
        event.preventDefault()
        // ignore
        MainStore.setPickedService(null)

	},
	dragOverFunc(event) {
        event.preventDefault()
        // ignore
	},
	dropFunc(event) {
        event.preventDefault()
        // ignore
        MainStore.setPickedService(null)
	},
}

const destroyed = function() {
}

export default {
    props: [
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
        FloatLabel,
        Textarea,
    },
	directives: {},
	name: 'WorkflowEditorWorkflowTools',
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
            workflowDetailsWindowMinimized: false,
            workflowName: "",
            workflowDescription: "",
            searchServicesWindowMinimized: false,
            serviceOffers: [],
        }
    }
}
