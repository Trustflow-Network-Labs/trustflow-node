<template>
  <main :class="[dashboardClass, 'window-container']">
    <div class="menu-container">
      <div id="input" class="input-box">
        <button class="btn" @click="stopNode">{{ $t("message.dashboard.stop-node-button-text") }}</button>
      </div>
    </div>
    <div class="resizer"></div>
    <div class="main-container">
      <div class="error-text">{{ errorText }}</div>
    </div>
  </main>
</template>

<script src="../js/dashboard.js" scoped />
<style src="../scss/dashboard.scss" lang="scss" scoped />
