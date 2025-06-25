<template>
  <main :class="cockpitWorkflowEditorServiceBoxClass">
    <div :class="['hover-ring', {'data': service.type == 'DATA', 'function': service.type == 'DOCKER EXECUTION ENVIRONMENT' || service.type == 'STANDALONE EXECUTABLE'}]"></div>
    <div :class="['service', {'data': service.type == 'DATA', 'function': service.type == 'DOCKER EXECUTION ENVIRONMENT' || service.type == 'STANDALONE EXECUTABLE'}]">
      <div :class="['service-header', {'data': service.type == 'DATA', 'function': service.type == 'DOCKER EXECUTION ENVIRONMENT' || service.type == 'STANDALONE EXECUTABLE'}]">
        {{ shorten(service.type, 28, 0) }}
      </div>
      <div class="service-title">{{ service.name }}</div>
      <div class="service-subtitle">{{ shorten(service.node_id + "-" + service.id, 12, 12) }}</div>
      <div class="service-content">
          <div class="service-content-section"
            v-if="service.description !=''">
            <div class="service-content-section-header">Description:</div>
            {{ shorten(service.description, 100, 0) }}
          </div>
          <div class="service-content-section">
            <div class="service-content-section-header">Service price model:</div>
              {{ (service.service_price_model == null) ? "FREE" : service.service_price_model }}
          </div>
          <div class="service-content-section-group-header">Service interfaces:</div>
          <div class="service-content-section ind-1" v-for="(intfce, intfceIndex) in service.interfaces" :key="intfceIndex">
            <div class="service-content-section-header ind-m-1">{{ "(" + (intfceIndex+1) + ")" }} Interface type:</div>
            {{ intfce.interface_type }}
            <div class="service-content-section-header"
              v-if="intfce.description !=''">Interface description:</div>
              <div v-if="intfce.description !=''">{{ intfce.description }}</div>
            <div class="service-content-section-header">Interface paths:</div>
            <div v-for="(path, pathIndex) in intfce.path.split(',')" :key="pathIndex">
              {{ shorten(path, 12, 12) }}
            </div>
          </div>
      </div>
      <div class="service-footer">
        <div id="input" class="input-box">
          <button class="btn light" @click="">{{ $t("message.cockpit.detail.workflow-editor.service-box.service-details") }}</button>
          <button class="btn" @click="">{{ $t("message.cockpit.detail.workflow-editor.service-box.request-service") }}</button>
        </div>
      </div>
    </div>
  </main>
</template>

<script src="../../../js/cockpit/workflow-editor/service-box.js" scoped />
<style src="../../../scss/cockpit/workflow-editor/service-box.scss" lang="scss" scoped />
