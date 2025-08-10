import { defineStore } from 'pinia'

export const useMainStore = defineStore('main', {
    state: () => ({
        theme: 'common',
        themeVariety: 'default',
        themeName: 'Main theme, variety default',
        locale: 'en_GB',
        appCanStart: false,
        appConfirm: "",
        appLogs: [],
        exitLogs: [],
        hostRunning: false,
        selectedMenuKey: null,
        serviceOffer: null,
        pickedService: null,
    }),

    getters: {
        getTheme: (state) => state.theme,
        getThemeVariety: (state) => state.themeVariety,
        getThemeName: (state) => state.themeName,
        getLocale: (state) => state.locale,
        getAppCanStart: (state) => state.appCanStart,
        getAppConfirm: (state) => state.appConfirm,
        getHostRunning: (state) => state.hostRunning,
        getAppLogs: (state) => state.appLogs,
        getExitLogs: (state) => state.exitLogs,
        getSelectedMenuKey: (state) => state.selectedMenuKey,
        getServiceOffer: (state) => state.serviceOffer,
        getPickedService: (state) => state.pickedService,
    },

    actions: {
        setTheme(theme) {
            this.theme = theme
        },
        setThemeVariety(themeVariety) {
            this.themeVariety = themeVariety
        },
        setThemeName(themeName) {
            this.themeName = themeName
        },
        setLocale(locale) {
            this.locale = locale
        },
        setAppCanStart(appCanStart) {
            this.appCanStart = appCanStart
        },
        setAppConfirm(appConfirm) {
            this.appConfirm = appConfirm
        },
        setHostRunning(hostRunning) {
            this.hostRunning = hostRunning
        },
        setAppLogs(appLogs) {
            this.appLogs = appLogs
        },
        setExitLogs(exitLogs) {
            this.exitLogs = exitLogs
        },
        setSelectedMenuKey(menuKey) {
            this.selectedMenuKey = menuKey
        },
        setServiceOffer(serviceOffer) {
            this.serviceOffer = serviceOffer
        },
        setPickedService(service) {
            this.pickedService = service
        },
    }
})