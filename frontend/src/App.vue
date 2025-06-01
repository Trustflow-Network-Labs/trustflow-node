<template>
  <Landing v-if="!hostRunning"
    :app-logs="appLogs"
    :exit-logs="exitLogs"
    :app-confirm="sysConfirm"
    @host-running="(running) => {hostRunning = running}"/>
  <Dashboard v-if="hostRunning"
    :app-logs="appLogs"
    :exit-logs="exitLogs"
    :app-confirm="sysConfirm"
    @host-running="(running) => {hostRunning = running}"/>
</template>

<script src="./js/app.js" scoped />
<style src="./scss/app.scss" lang="scss" scoped />
