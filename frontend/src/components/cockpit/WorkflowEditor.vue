<template>
  <main ref="workflowEditor" :class="cockpitWorkflowEditorClass"
    @dragend="dragEndFunc($event)"
    @dragover="dragOverFunc($event)"
    @drop="dropFunc($event)">
    <SearchServices
      v-if="workflowEditorEl"
      :workflow-editor-el="$refs['workflowEditor']" />
    <component v-for="(serviceCard, serviceCardIndex) in serviceCards" :key="serviceCardIndex"
      :is="serviceCard.type" v-bind="serviceCard.props"
      :ref="`serviceCard${serviceCardIndex}`" />
  </main>
</template>

<script src="../../js/cockpit/workflow-editor.js" scoped />
<style src="../../scss/cockpit/workflow-editor.scss" lang="scss" scoped />
