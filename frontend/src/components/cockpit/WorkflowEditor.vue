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
        <div class="service-offer" v-for="(serviceOffer, serviceOfferIndex) in serviceOffers" :key="serviceOfferIndex">
          <Card style="width: 25rem; overflow: hidden">
              <template #header>
                {{ serviceOffer.node_id + "-" + serviceOffer.id }}
              </template>
              <template #title>{{ serviceOffer.name }}</template>
              <template #subtitle>{{ serviceOffer.type }}</template>
              <template #content>
                  <p class="m-0">
                      Description: {{ serviceOffer.description }}
                  </p>
                  <p class="m-0">
                      Service price model: {{ serviceOffer.service_price_model }}
                  </p>
                  <p class="m-0" v-for="(intfce, intfceIndex) in serviceOffer.interfaces" :key="intfceIndex">
                      Interface type: {{ intfce.interface_type }}
                      Interface paths: {{ intfce.path }}
                      Interface description: {{ intfce.description }}
                  </p>
              </template>
              <template #footer>
                  <div class="flex gap-4 mt-1">
                      <Button label="Cancel" severity="secondary" outlined class="w-full" />
                      <Button label="Save" class="w-full" />
                  </div>
              </template>
          </Card>
        </div>
      </div>
    </div>
  </main>
</template>

<script src="../../js/cockpit/workflow-editor.js" scoped />
<style src="../../scss/cockpit/workflow-editor.scss" lang="scss" scoped />
