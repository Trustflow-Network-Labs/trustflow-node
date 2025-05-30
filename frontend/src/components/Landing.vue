<template>
  <main>
    <div id="result" class="result">{{ resultText }}</div>
    <div id="input" class="input-box">
      <input id="port" v-model="port" autocomplete="off" class="input" type="number" v-if="!hostRunning"/>
      <button class="btn" @click="startNode" v-if="!hostRunning">Start</button>
      <button class="btn" @click="stopNode" v-if="hostRunning">Stop</button>
    </div>
  </main>
</template>

<script src="../js/landing.js" scoped />
<style src="../scss/landing.scss" lang="scss" scoped />
