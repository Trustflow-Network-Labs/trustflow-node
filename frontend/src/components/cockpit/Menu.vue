<template>
  <main :class="cockpitMenuClass">
    <img class="logo" alt="Trustflow logo" src="../../assets/images/logo.png"/>
    <PanelMenu :model="menuItems" />
    <div id="input" class="input-box gap-top-2">
      <button class="btn" @click="stopNode">{{ $t("message.cockpit.stop-node-button-text") }}</button>
    </div>
    <div class="error-text">{{ errorText }}</div>
  </main>
</template>

<script src="../../js/cockpit/menu.js" scoped />
<style src="../../scss/cockpit/menu.scss" lang="scss" scoped />
