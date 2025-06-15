<template>
  <main :class="dashboardClass">
    <div id="input" class="input-box">
      <button class="btn" @click="stopNode">{{ $t("message.dashboard.stop-node-button-text") }}</button>
    </div>
    <div class="error-text">{{ errorText }}</div>
  </main>
</template>

<script src="../js/dashboard.js" scoped />
<style src="../scss/dashboard.scss" lang="scss" scoped />
