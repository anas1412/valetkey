<script setup lang="ts">
import { withBase } from 'vitepress'

// The dashboard walkthrough video (docs/public/walkthrough.mp4, recorded by scripts/walkthrough.mjs).
defineProps<{ caption?: string }>()
</script>

<template>
  <div class="walkthrough">
    <video controls muted playsinline preload="metadata" :poster="withBase('/walkthrough-poster.jpg')">
      <source :src="withBase('/walkthrough.mp4')" type="video/mp4" />
    </video>
    <p v-if="caption">{{ caption }}</p>
  </div>
</template>
