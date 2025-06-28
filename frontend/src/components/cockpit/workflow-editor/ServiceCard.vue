<template>
  <main :class="cockpitWorkflowEditorServiceCardClass">
    <div class="card-top">
      <div class="service-name">{{ service.name }}</div>
      <div class="service-type">{{ service.type }}</div>
    </div>
    <div class="card-rip"></div>
    <div class="card-bottom">
    </div>
  </main>
</template>

<script src="../../../js/cockpit/workflow-editor/service-card.js" scoped />
<style src="../../../scss/cockpit/workflow-editor/service-card.scss" lang="scss" scoped />
