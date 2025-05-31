<template>
  <main class="container">
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
          <button class="btn" @click="startNode">Start</button>
        </div>
        <div class="start-node-text-helper">{{ startNodeTextHelper }}</div>
      </div>
    </div>
  </main>
</template>

<script src="../js/landing.js" scoped />
<style src="../scss/landing.scss" lang="scss" scoped />
