<script setup>
import Landing from './components/Landing.vue'
</script>

<template>
  <img id="logo" alt="Wails logo" src="./assets/images/logo.png"/>
  <Landing/>
</template>

<style>
#logo {
  display: block;
  width: 150px;
  height: auto;
  margin: auto;
  padding: 10% 0 0;
  background-position: center;
  background-repeat: no-repeat;
  background-size: 100% 100%;
  background-origin: content-box;
}
</style>
