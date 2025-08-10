<template class="app">
  <Landing v-if="!hostRunning" />
  <Cockpit v-if="hostRunning" />
</template>

<script src="./js/app.js" scoped />
<style src="./scss/app.scss" lang="scss" scoped />
