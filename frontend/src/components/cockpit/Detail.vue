<template>
  <main :class="cockpitDetailClass">
    <component
      :is="currentComponent"
      :panes-resized="panesResized" />
  </main>
</template>

<script src="../../js/cockpit/detail.js" scoped />
<style src="../../scss/cockpit/detail.scss" lang="scss" scoped />
