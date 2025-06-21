<template>
  <main :class="[cockpitClass, 'window-container']">
    <div class="menu-container">
      <div id="input" class="input-box">
        <button class="btn" @click="stopNode">{{ $t("message.cockpit.stop-node-button-text") }}</button>
      </div>
    </div>
    <div class="resizer"></div>
    <div class="main-container">
      <div class="error-text">{{ errorText }}</div>
    </div>
  </main>
</template>

<script src="../js/cockpit.js" scoped />
<style src="../scss/cockpit.scss" lang="scss" scoped />
