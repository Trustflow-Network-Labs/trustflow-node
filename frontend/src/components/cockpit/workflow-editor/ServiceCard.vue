<template>
  <main :class="cockpitWorkflowEditorServiceCardClass">
    <div class="card-top">
      <div class="card-controls">
        <i class="pi pi-times-circle"
          @click="closeServiceCard"></i>
      </div>
      <div class="service-name">{{ service.name }}</div>
      <div :class="['service-type', {'data': service.type == 'DATA', 'function': service.type == 'DOCKER EXECUTION ENVIRONMENT' || service.type == 'STANDALONE EXECUTABLE'}]">
        <i class="pi pi-file"
          v-if="service.type == 'DATA'" />
        <i class="pi pi-server"
          v-if="service.type == 'DOCKER EXECUTION ENVIRONMENT' || service.type == 'STANDALONE EXECUTABLE'" />
        {{ service.type }}
      </div>
    </div>
    <div class="card-rip"></div>
    <div class="card-bottom">
      <i :class="['pi', {'pi-exclamation-triangle red': !ready, 'pi-check-square': ready}]"></i>
      <i :class="['pi', {'pi-money-bill': paid, 'pi-wallet  red': !paid}]"></i>
      <i class="pi pi-ellipsis-v"
        @click=""></i>
    </div>
  </main>
</template>

<script src="../../../js/cockpit/workflow-editor/service-card.js" scoped />
<style src="../../../scss/cockpit/workflow-editor/service-card.scss" lang="scss" scoped />
