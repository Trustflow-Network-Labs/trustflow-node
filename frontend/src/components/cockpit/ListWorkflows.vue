<template>
  <main ref="listWorkflows" :class="cockpitListWorkflowsClass">
    <div class="list-workflows-controls">
      <div class="list-workflows-controls-search">
        <InputGroup>
            <InputText :placeholder="$t('message.cockpit.detail.list-workflows.search-workflows')" />
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
      <div class="list-workflows-filters-title">
        <i class="pi pi-filter"></i> {{ $t("message.cockpit.detail.list-workflows.filter-workflows-by-status") }}
      </div>
      <div class="list-workflows-filters-list">
        <div class="list-workflows-filters-box">
          <div class="list-workflows-filters-box-content">
            <div class="list-workflows-filters-box-content-icon">
              <OverlayBadge :value="inDesign" severity="contrast" size="small">
                  <Avatar icon="pi pi-objects-column" size="large"
                     style="background-color: var(--tf-in-design-icon-background-color); color: var(--tf-in-design-icon-color)" />
              </OverlayBadge>
            </div>
            <div class="list-workflows-filters-box-content-details">
              {{ $t("message.cockpit.detail.list-workflows.in-design") }}
            </div>
          </div>
        </div>
        <div class="list-workflows-filters-box">
          <div class="list-workflows-filters-box-content">
            <div class="list-workflows-filters-box-content-icon">
              <OverlayBadge :value="running" severity="contrast" size="small">
                  <Avatar icon="pi pi-play" size="large"
                     style="background-color: var(--tf-running-icon-background-color); color: var(--tf-running-icon-color)" />
              </OverlayBadge>
            </div>
            <div class="list-workflows-filters-box-content-details">
              {{ $t("message.cockpit.detail.list-workflows.running") }}
            </div>
          </div>
        </div>
        <div class="list-workflows-filters-box">
          <div class="list-workflows-filters-box-content">
            <div class="list-workflows-filters-box-content-icon">
              <OverlayBadge :value="completed" severity="contrast" size="small">
                  <Avatar icon="pi pi-verified" size="large"
                     style="background-color: var(--tf-completed-icon-background-color); color: var(--tf-completed-icon--color)" />
              </OverlayBadge>
            </div>
            <div class="list-workflows-filters-box-content-details">
              {{ $t("message.cockpit.detail.list-workflows.completed") }}
            </div>
          </div>
        </div>
        <div class="list-workflows-filters-box">
          <div class="list-workflows-filters-box-content">
            <div class="list-workflows-filters-box-content-icon">
              <OverlayBadge :value="errored" severity="contrast" size="small">
                  <Avatar icon="pi pi-exclamation-circle" size="large"
                     style="background-color: var(--tf-errored-icon-background-color); color: var(--tf-errored-icon-color)" />
              </OverlayBadge>
            </div>
            <div class="list-workflows-filters-box-content-details">
              {{ $t("message.cockpit.detail.list-workflows.errored") }}
            </div>
          </div>
        </div>
      </div>
    </div>
    <div class="list-workflows">
      <div class="list-workflows-title">
        <i class="pi pi-list"></i> {{ $t("message.cockpit.detail.list-workflows.workflows-list") }}
      </div>
    </div>
    <Toast position="top-center" />
    <ConfirmDialog />
  </main>
</template>

<script src="../../js/cockpit/list-workflows.js" scoped />
<style src="../../scss/cockpit/list-workflows.scss" lang="scss" scoped />
