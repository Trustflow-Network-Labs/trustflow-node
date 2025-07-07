<template>
  <main ref="listWorkflows" :class="cockpitListWorkflowsClass">
    <div class="list-workflows-controls">
      <div class="list-workflows-controls-search">
        <InputGroup>
            <InputText placeholder="Search workflows" />
            <InputGroupAddon>
                <Button icon="pi pi-search" severity="secondary" variant="text" @click="" />
            </InputGroupAddon>
        </InputGroup>
      </div>
      <div class="list-workflows-controls-buttons">
        <div id="input" class="input-box">
          <button :class="['btn', 'light']"
            @click=""><i class="pi pi-times-circle"></i> {{ $t("message.cockpit.detail.list-workflows.delete") }}</button>
        </div>
        <div id="input" class="input-box">
          <button :class="['btn']"
            @click="navigate('workflow-editor')"><i class="pi pi-plus-circle"></i> {{ $t("message.cockpit.detail.list-workflows.new") }}</button>
        </div>
      </div>
    </div>
    <div class="list-workflows-filters">
      <div class="list-workflows-filters-box">
      </div>
    </div>
    <Toast position="top-center" />
    <ConfirmDialog />
  </main>
</template>

<script src="../../js/cockpit/list-workflows.js" scoped />
<style src="../../scss/cockpit/list-workflows.scss" lang="scss" scoped />
