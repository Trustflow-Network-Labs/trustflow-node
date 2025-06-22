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
            <InputText placeholder="Search phrases (comma separated)" />
            <InputGroupAddon>
                <Button icon="pi pi-search" severity="secondary" variant="text" @click="toggleSearchServiceTypes" />
            </InputGroupAddon>
        </InputGroup>
        <Menu appendTo="#cockpit" ref="menu" :model="searchServiceTypes" popup class="!min-w-fit" />
        </div>
        <div class="separator">Services found:</div>
        <div>
        </div>
      </div>
    </div>
  </main>
</template>

<script src="../../js/cockpit/workflow-editor.js" scoped />
<style src="../../scss/cockpit/workflow-editor.scss" lang="scss" scoped />
