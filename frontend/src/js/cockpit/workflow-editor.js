import { FindServices } from '../../../wailsjs/go/main/App'

import { useMainStore } from '../../stores/main.js'

import PlainDraggable from "plain-draggable"
import LeaderLine from "leader-line-new"


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
	},
    serviceOffer() {
        return MainStore.getServiceOffer
    }
}

const watch = {
    panesResized() {
        if (this.panesResized == true) {
            this.repositionSearchServicesBox()
        }
    },
    serviceOffer() {
        console.log(this.serviceOffer)
        this.serviceOffers.push(this.serviceOffer)
    },
}

const mounted = function() {
    this.initSearchServicesBox()
}

const methods = {
    initSearchServicesBox() {
        let searchServicesEl = document.querySelector('.search-services')
        let searchServicesHeaderEl = searchServicesEl.querySelector('.search-services-header')
        this.draggableSearchServices = new PlainDraggable(searchServicesEl,
            {
                handle: searchServicesHeaderEl,
                containment: this.$refs['workflowEditor'],
            })
    },
    destroySearchServicesBox() {
        this.destroyDraggable(this.draggableSearchServices)
    },
    repositionSearchServicesBox() {
        this.draggableSearchServices.position()
    },
    destroyDraggable(el) {
        el.remove()
    },
    toggleSearchServiceTypes(event) {
        this.$refs["menu"].toggle(event)
    },
    toggleWindow(win) {
        switch (win) {
            case 'search-services':
                this.searchServicesWindowOpen = !this.searchServicesWindowOpen
                break
            default:
        }
        this.repositionSearchServicesBox()
    },
    async findServices({item}){
        this.serviceOffers.length = 0
        await FindServices(this.searchServicesPhrases, item.id)
    },
}

const destroyed = function() {
    this.destroySearchServicesBox()
}

export default {
    props: [
        'panesResized',
    ],
	mixins: [
    ],
	components: {
        InputGroup,
        InputText,
        InputGroupAddon,
        Menu,
        Button,
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
            draggableSearchServices: null,
            searchServicesWindowOpen: true,
            serviceOffers: [],
        }
    }
}
