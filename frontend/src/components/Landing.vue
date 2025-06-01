<template>
  <main class="rows-container">
    <div class="columns-container">
      <div class="sections">
        <div class="section">
          <div class="trustflow-node-text">{{ trustflowNodeText }}</div>
          <div class="trustflow-node-text-helper">{{ trustflowNodeTextHelper }}</div>
        </div>
        <div class="section">
          <img id="logo" alt="Trustflow logo" src="../assets/images/logo.png"/>
          <div class="start-node-text">{{ startNodeText }}</div>
          <div id="input" class="input-box">
            <input id="port" v-model="port" autocomplete="off" class="input" type="number" />
            <button :class="['btn', {'disabled': !appCanStart}]" @click="startNode" :disabled="!appCanStart">Start</button>
          </div>
          <div class="start-node-text-helper">{{ startNodeTextHelper }}</div>
        </div>
      </div>
    </div>
    <div class="app-logs-frame">
      <div class="app-logs-frame-header">
        Console log
      </div>
      <div class="app-logs">
        <div ref="log" class="app-log" v-for="(log, index) in appLogs" :key="index">
          ~ {{ log }}
        </div>
      </div>
    </div>
  </main>
</template>

<script src="../js/landing.js" scoped />
<style src="../scss/landing.scss" lang="scss" scoped />
