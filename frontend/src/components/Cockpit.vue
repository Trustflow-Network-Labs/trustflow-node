<template>
  <main :class="[cockpitClass, 'window-container']">
    <div class="menu-container">
      <Menu
        @host-running="(running) => {hostRunning = running}" />
    </div>
    <div class="resizer"></div>
    <div class="main-container">
      <Detail />
    </div>
  </main>
</template>

<script src="../js/cockpit.js" scoped />
<style src="../scss/cockpit.scss" lang="scss" scoped />
