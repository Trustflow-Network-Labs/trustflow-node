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
        selectedMenuKey: null,
        serviceOffer: null,
        selectedService: null,
    }),

    getters: {
        getTheme: (state) => state.theme,
        getThemeVariety: (state) => state.themeVariety,
        getThemeName: (state) => state.themeName,
        getLocale: (state) => state.locale,
        getAppCanStart: (state) => state.appCanStart,
        getAppConfirm: (state) => state.appConfirm,
        getAppLogs: (state) => state.appLogs,
        getExitLogs: (state) => state.exitLogs,
        getSelectedMenuKey: (state) => state.selectedMenuKey,
        getServiceOffer: (state) => state.serviceOffer,
        getSelectedService: (state) => state.selectedService,
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
        setSelectedService(service) {
            this.selectedService = service
        },
    }
})