import { defineStore } from 'pinia'

export const useMainStore = defineStore('main', {
    state: () => ({
        theme: 'common',
        themeVariety: 'default',
        themeName: 'Main theme, variety default',
        locale: 'en_GB',
        selectedMenuKey: null,
    }),

    getters: {
        getTheme: (state) => state.theme,
        getThemeVariety: (state) => state.themeVariety,
        getThemeName: (state) => state.themeName,
        getLocale: (state) => state.locale,
        getSelectedMenuKey: (state) => state.selectedMenuKey,
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
        setSelectedMenuKey(menuKey) {
            this.selectedMenuKey = menuKey
        },
    }
})