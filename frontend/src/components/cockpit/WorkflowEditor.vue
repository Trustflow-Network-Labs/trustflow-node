<template>
  <main ref="workflowEditor" :class="cockpitWorkflowEditorClass">
    <div ref="grid" class="grid"
      @dragend="dragEndFunc($event)"
      @dragover="dragOverFunc($event)"
      @drop="dropFunc($event)">
      <component v-for="(serviceCard, serviceCardIndex) in serviceCards" :key="serviceCardIndex"
        :is="serviceCard.type" v-bind="serviceCard.props"
        :ref="`serviceCard${serviceCard.props.serviceCardId}`"
        @close-service-card="async (id) => await removeServiceCard(id)" />
    </div>
    <WorkflowTools ref="workflowTools"
      v-if="workflowEditorEl"
      @snap-to-grid="(snap) => snapToGrid = snap" />
  </main>
</template>

<script src="../../js/cockpit/workflow-editor.js" scoped />
<style src="../../scss/cockpit/workflow-editor.scss" lang="scss" scoped />
