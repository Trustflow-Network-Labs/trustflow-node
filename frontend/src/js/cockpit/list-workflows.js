import {
    RemoveWorkflow,
    ListWorkflows,
} from '../../../wailsjs/go/main/App'

import { useMainStore } from '../../stores/main.js'

import { textUtils } from '../../mixins/text.js'


import InputText from 'primevue/inputtext'
import InputGroup from 'primevue/inputgroup'
import InputGroupAddon from 'primevue/inputgroupaddon'
import Button from 'primevue/button'
import Badge from 'primevue/badge'
import OverlayBadge from 'primevue/overlaybadge'
import Avatar from 'primevue/avatar'

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
    cockpitListWorkflowsClass() {
		return this.theme + '-cockpit-list-workflows-' + this.themeVariety
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
}

const mounted = async function() {
    let response = await ListWorkflows(0, 10)
    console.log(response)
}

const methods = {
    navigate(menuKey) {
        MainStore.setSelectedMenuKey(menuKey)
    }
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
        Toast,
        ConfirmDialog,
        InputText,
        InputGroup,
        InputGroupAddon,
        Button,
        Badge,
        OverlayBadge,
        Avatar,
    },
	directives: {},
	name: 'ListWorkflows',
    setup: setup,
    created: created,
    computed: computed,
    watch: watch,
    mounted: mounted,
    methods: methods,
    destroyed: destroyed,
    data() {
        return {
            inDesign: 0,
            running: 0,
            completed: 0,
            errored: 0,
       }
    }
}
