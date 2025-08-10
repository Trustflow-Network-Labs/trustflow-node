<template>
  <main id="cockpit" :class="[cockpitClass, 'window-container']">
    <div class="menu-container">
      <Menu />
    </div>
    <div class="resizer"></div>
    <div class="main-container">
      <Detail
        :panes-resized="panesResized" />
    </div>
  </main>
</template>

<script src="../js/cockpit.js" scoped />
<style src="../scss/cockpit.scss" lang="scss" scoped />
