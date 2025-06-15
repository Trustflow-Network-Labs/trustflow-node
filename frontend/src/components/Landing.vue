<template>
  <main :class="landingClass">
    <div class="columns-container">
      <div class="sections">
        <div class="section">
          <div class="trustflow-node-text">{{ $t("message.landing.trustflow-node-text") }}</div>
          <div class="trustflow-node-text-helper">{{ $t("message.landing.trustflow-node-text-helper") }}</div>
        </div>
        <div class="section">
          <img id="logo" alt="Trustflow logo" src="../assets/images/logo.png"/>
          <div class="start-node-text">{{ $t("message.landing.start-node-text") }}</div>
          <div id="input" class="input-box">
            <input id="port" v-model="port" autocomplete="off" class="input" type="number" />
            <button :class="['btn', {'disabled': !appCanStart}]" @click="startNode" :disabled="!appCanStart">{{ $t("message.landing.start-node-button-text") }}</button>
          </div>
          <div class="start-node-text-helper">{{ $t("message.landing.start-node-text-helper") }}</div>
        </div>
      </div>
    </div>
    <div class="app-logs-frame">
      <div class="app-logs-frame-header">
        {{ $t("message.landing.console-log") }}
      </div>
      <div class="app-logs">
        <div ref="log" class="app-log" v-for="(log, index) in appLogs" :key="index">
          ~ {{ log }}
        </div>
      </div>
    </div>
    <ConfirmDialog />
  </main>
</template>

<script src="../js/landing.js" scoped />
<style src="../scss/landing.scss" lang="scss" scoped />
