<template>
  <main :class="cockpitWorkflowEditorSearchServicesClass">
    <div class="search-services">
      <div class="search-services-header">
        <div class="search-services-header-title">
          <i class="pi pi-eye"></i> {{ $t("message.cockpit.detail.workflow-editor.search-services.search-services") }}
        </div>
        <div class="search-services-header-window-controls">
          <i :class="['pi', {'pi-window-minimize': searchServicesWindowMinimized, 'pi-window-maximize': !searchServicesWindowMinimized}]"
            @click="toggleWindow('search-services')"></i>
        </div>
      </div>
      <div v-show="!searchServicesWindowMinimized" class="search-services-body">
        <div>
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
  </main>
</template>

<script src="../../../js/cockpit/workflow-editor/search-services.js" scoped />
<style src="../../../scss/cockpit/workflow-editor/search-services.scss" lang="scss" scoped />
