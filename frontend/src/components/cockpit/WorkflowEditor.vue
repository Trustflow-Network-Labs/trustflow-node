<template>
  <main ref="workflowEditor" :class="cockpitWorkflowEditorClass">
    <div class="search-services">
      <div class="search-services-header">
        <div class="search-services-header-title">
          <i class="pi pi-eye"></i> Search services
        </div>
        <div class="search-services-header-window-controls">
          <i :class="['pi', {'pi-window-minimize': searchServicesWindowOpen, 'pi-window-maximize': !searchServicesWindowOpen}]"
            @click="toggleWindow('search-services')"></i>
        </div>
      </div>
      <div v-show="searchServicesWindowOpen" class="search-services-body">
        <div>
          <InputGroup>
              <InputText v-model="searchServicesPhrases" placeholder="Search phrases (comma separated)" />
              <InputGroupAddon>
                  <Button icon="pi pi-search" severity="secondary" variant="text"
                    @click="toggleSearchServiceTypes" />
              </InputGroupAddon>
          </InputGroup>
          <Menu appendTo="#cockpit" ref="menu" :model="searchServicesTypes" popup class="!min-w-fit" />
        </div>
        <div class="separator">Services found:</div>
        <div class="service-offers">
          <div class="service" v-for="(serviceOffer, serviceOfferIndex) in serviceOffers" :key="serviceOfferIndex">
            <div class="service-header">
              {{ serviceOffer.node_id + "-" + serviceOffer.id }}
            </div>
            <div class="service-title">{{ serviceOffer.name }}</div>
            <div class="service-subtitle">{{ serviceOffer.type }}</div>
            <div class="service-content">
                <p class="service-content-section">
                    Description: {{ serviceOffer.description }}
                </p>
                <p class="service-content-section">
                    Service price model: {{ serviceOffer.service_price_model }}
                </p>
                <p class="service-content-section" v-for="(intfce, intfceIndex) in serviceOffer.interfaces" :key="intfceIndex">
                    Interface type: {{ intfce.interface_type }}
                    Interface paths: {{ intfce.path }}
                    Interface description: {{ intfce.description }}
                </p>
            </div>
            <div class="service-footer">
                    <Button label="Cancel" severity="secondary" outlined class="w-full" />
                    <Button label="Save" class="w-full" />
            </div>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>

<script src="../../js/cockpit/workflow-editor.js" scoped />
<style src="../../scss/cockpit/workflow-editor.scss" lang="scss" scoped />
