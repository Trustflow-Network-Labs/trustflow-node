<template>
  <main :class="cockpitWorkflowEditorWorkflowToolsClass">
    <div ref="workflowToolsContainer" class="workflow-tools-container">
    <div class="workflow-details">
      <div class="workflow-details-header">
        <div class="workflow-details-header-title">
          <i class="pi pi-receipt"></i> {{ $t("message.cockpit.detail.workflow-editor.workflow-details.workflow-details") }}
        </div>
        <div class="workflow-details-header-window-controls">
          <i :class="['pi', {'pi-window-minimize': workflowDetailsWindowMinimized, 'pi-window-maximize': !workflowDetailsWindowMinimized}]"
            @click="toggleWindow('workflow-details')"></i>
        </div>
      </div>
      <div v-show="!workflowDetailsWindowMinimized" class="workflow-details-body">
        <div class="workflow-details-body-section">
          <FloatLabel variant="on">
            <InputText id="workflowName" v-model="workflowName" />
            <label for="workflowName">{{ $t("message.cockpit.detail.workflow-editor.workflow-details.workflow-name") }}</label>
          </FloatLabel>
        </div>
        <div class="workflow-details-body-section">
          <FloatLabel variant="on">
              <Textarea id="workflowDescription" v-model="workflowDescription" rows="5" cols="30" style="resize: none" />
              <label for="workflowDescription">{{ $t("message.cockpit.detail.workflow-editor.workflow-details.workflow-description") }}</label>
          </FloatLabel>
        </div>
        <div class="workflow-details-body-section buttons">
          <div id="input" class="input-box">
            <button :class="['btn', 'light']"
              @click="">{{ $t("message.cockpit.detail.workflow-editor.workflow-details.delete") }}</button>
          </div>
          <div id="input" class="input-box">
            <button :class="['btn']"
              @click="">{{ $t("message.cockpit.detail.workflow-editor.workflow-details.save") }}</button>
          </div>
        </div>
      </div>
    </div>
    <div class="search-services">
      <div class="search-services-header">
        <div class="search-services-header-title">
          <i class="pi pi-search"></i> {{ $t("message.cockpit.detail.workflow-editor.search-services.search-services") }}
        </div>
        <div class="search-services-header-window-controls">
          <i :class="['pi', {'pi-window-minimize': searchServicesWindowMinimized, 'pi-window-maximize': !searchServicesWindowMinimized}]"
            @click="toggleWindow('search-services')"></i>
        </div>
      </div>
      <div v-show="!searchServicesWindowMinimized" class="search-services-body">
        <div class="search-services-body-section">
          <InputGroup>
              <InputText v-model="searchServicesPhrases" :placeholder="$t('message.cockpit.detail.workflow-editor.search-services.search-phrases')"></InputText>
              <InputGroupAddon>
                  <Button icon="pi pi-search" severity="secondary" variant="text"
                    @click="toggleSearchServiceTypes"></Button>
              </InputGroupAddon>
          </InputGroup>
          <Menu appendTo="#cockpit" ref="menu" :model="searchServicesTypes" popup class="!min-w-fit"></Menu>
        </div>
        <div class="separator">{{ $t("message.cockpit.detail.workflow-editor.search-services.services-found") }}:</div>
        <div class="service-offers">
          <SearchResult draggable="true"
            v-for="(serviceOffer, serviceOfferIndex) in serviceOffers" :key="serviceOfferIndex"
            :service="serviceOffer"
            @dragstart="dragStartFunc($event, serviceOffer)"
            @dragend="dragEndFunc($event)"
            @dragover="dragOverFunc($event)"
            @drop="dropFunc($event)" />
        </div>
      </div>
    </div>
    </div>
  </main>
</template>

<script src="../../../js/cockpit/workflow-editor/workflow-tools.js" scoped />
<style src="../../../scss/cockpit/workflow-editor/workflow-tools.scss" lang="scss" scoped />
