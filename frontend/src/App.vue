<template>
  <Landing v-if="!hostRunning"
    @host-running="(running) => {hostRunning = running}"/>
  <Dashboard v-if="hostRunning"
    @host-running="(running) => {hostRunning = running}"/>
</template>

<script src="./js/app.js" scoped />
<style src="./scss/app.scss" lang="scss" scoped />
