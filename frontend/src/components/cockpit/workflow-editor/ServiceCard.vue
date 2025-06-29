<template>
  <main :class="cockpitWorkflowEditorServiceCardClass">
    <div class="card-top">
      <div class="card-controls">
        <i class="pi pi-times-circle"
          @click="closeServiceCard"></i>
      </div>
      <div class="service-name">{{ service.name }}</div>
      <div :class="['service-type', {'data': service.type == 'DATA', 'function': service.type == 'DOCKER EXECUTION ENVIRONMENT' || service.type == 'STANDALONE EXECUTABLE'}]">
        {{ service.type }}
      </div>
    </div>
    <div class="card-rip"></div>
    <div class="card-bottom">
      <i :class="['pi', {'pi-exclamation-triangle': !ready, 'pi-check-square': ready}]"></i>
      <i :class="['pi', {'pi-money-bill': !free, 'pi-wallet': free}]"></i>
      <i class="pi pi-ellipsis-v"
        @click=""></i>
    </div>
  </main>
</template>

<script src="../../../js/cockpit/workflow-editor/service-card.js" scoped />
<style src="../../../scss/cockpit/workflow-editor/service-card.scss" lang="scss" scoped />
