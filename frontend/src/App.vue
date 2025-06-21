<template class="app">
  <Landing v-if="!hostRunning"
    :app-logs="appLogs"
    :exit-logs="exitLogs"
    :app-confirm="sysConfirm"
    :app-can-start="appCanStart"
    @host-running="(running) => {hostRunning = running}"/>
  <Cockpit v-if="hostRunning"
    :app-logs="appLogs"
    :exit-logs="exitLogs"
    :app-confirm="sysConfirm"
    :app-can-start="appCanStart"
    @host-running="(running) => {hostRunning = running}"/>
</template>

<script src="./js/app.js" scoped />
<style src="./scss/app.scss" lang="scss" scoped />
