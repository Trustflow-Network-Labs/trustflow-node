<template class="app">
  <Landing v-if="!hostRunning"
    @host-running="(running) => {hostRunning = running}"/>
  <Cockpit v-if="hostRunning"
    @host-running="(running) => {hostRunning = running}"/>
</template>

<script src="./js/app.js" scoped />
<style src="./scss/app.scss" lang="scss" scoped />
