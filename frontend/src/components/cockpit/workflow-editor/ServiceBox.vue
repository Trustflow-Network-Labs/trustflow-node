<template>
  <main :class="cockpitWorkflowEditorServiceBoxClass">
    <div :class="['hover-ring', {'data': service.type == 'DATA', 'function': service.type == 'DOCKER EXECUTION ENVIRONMENT' || service.type == 'STANDALONE EXECUTABLE'}]"></div>
    <div :class="['service', {'data': service.type == 'DATA', 'function': service.type == 'DOCKER EXECUTION ENVIRONMENT' || service.type == 'STANDALONE EXECUTABLE'}]">
      <div :class="['service-header', {'data': service.type == 'DATA', 'function': service.type == 'DOCKER EXECUTION ENVIRONMENT' || service.type == 'STANDALONE EXECUTABLE'}]">
        {{ shorten(service.type, 28, 0) }}
      </div>
      <div class="service-title">{{ service.name }}</div>
      <div class="service-subtitle">
        <div>{{ shorten(service.node_id + "-" + service.id, 12, 12) }}</div>
        <div class="clickable"><i class="pi pi-copy" @click.stop="copyToClipboard" :data-ref="service.node_id + '-' + service.id"></i></div>
      </div>
      <div class="service-content">
          <div class="service-content-section"
            v-if="service.description !=''">
            <div class="service-content-section-header">{{ $t("message.cockpit.detail.workflow-editor.service-box.description") }}:</div>
            {{ shorten(service.description, 100, 0) }}
          </div>
          <div class="service-content-section">
            <div class="service-content-section-header">{{ $t("message.cockpit.detail.workflow-editor.service-box.service-price-model") }}:</div>
            <div class="service-price upper" v-if="service.service_price_model == null">{{ $t("message.cockpit.detail.workflow-editor.service-box.free") }}</div>
            <div v-else>
              <div class="service-price-model" v-for="(priceModel, priceModelIndex) in service.service_price_model" :key="priceModelIndex">
                <div>{{ (priceModelIndex+1) + ". " + priceModel.resource_group + " / " + priceModel.resource_name }}</div>
                <div class="service-price">{{ priceModel.currency_symbol + "" + priceModel.price + " per " +  priceModel.resource_unit}}</div>
              </div>
            </div>
          </div>
          <div class="service-content-section-group-header">{{ $t("message.cockpit.detail.workflow-editor.service-box.service-interfaces") }}:</div>
          <div class="service-content-section ind-1" v-for="(intfce, intfceIndex) in service.interfaces" :key="intfceIndex">
            <div class="service-content-section-data ind-m-1"
              v-if="intfce.interface_type == 'STDIN'">{{ "(" + (intfceIndex+1) + ")" }} {{ $t("message.cockpit.detail.workflow-editor.service-box.input-data") }}</div>
            <div class="service-content-section-data ind-m-1"
              v-else-if="intfce.interface_type == 'STDOUT'">{{ "(" + (intfceIndex+1) + ")" }} {{ $t("message.cockpit.detail.workflow-editor.service-box.output-data") }}</div>
            <div class="service-content-section-data ind-m-1"
              v-else-if="intfce.interface_type == 'MOUNT'">{{ "(" + (intfceIndex+1) + ")" }} {{ $t("message.cockpit.detail.workflow-editor.service-box.file-system-mount-point") }}</div>
            <div class="service-content-section-data ind-m-1"
              v-else>{{ "(" + (intfceIndex+1) + ") " + intfce.interface_type }}</div>
            <div class="service-content-section-header"
              v-if="intfce.description !=''">{{ $t("message.cockpit.detail.workflow-editor.service-box.interface-description") }}:</div>
            <div v-if="intfce.description !=''">{{ intfce.description }}</div>
            <div class="service-content-section-header">{{ $t("message.cockpit.detail.workflow-editor.service-box.interface-paths") }}:</div>
            <div class="service-content-section-data" v-for="(path, pathIndex) in intfce.path.split(',')" :key="pathIndex">
              <div>{{ shorten(path, 12, 12) }}</div>
              <div class="clickable"><i class="pi pi-copy" @click.stop="copyToClipboard" :data-ref="path"></i></div>
            </div>
          </div>
      </div>
      <div class="service-footer">
        <div id="input" class="input-box">
          <button class="btn" @click="">{{ $t("message.cockpit.detail.workflow-editor.service-box.add-to-workflow") }}</button>
        </div>
      </div>
    </div>
  </main>
</template>

<script src="../../../js/cockpit/workflow-editor/service-box.js" scoped />
<style src="../../../scss/cockpit/workflow-editor/service-box.scss" lang="scss" scoped />
