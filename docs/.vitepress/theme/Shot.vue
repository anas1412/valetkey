<script setup lang="ts">
import { withBase } from 'vitepress'

// A dashboard screenshot that follows the site's light/dark theme.
// Files live in docs/public/screenshots/<name>-light.png and <name>-dark.png.
defineProps<{ name: string; alt: string }>()
</script>

<template>
  <figure class="shot">
    <img class="light-only" :src="withBase(`/screenshots/${name}-light.png`)" :alt="alt" loading="lazy" width="1440" height="900" />
    <img class="dark-only" :src="withBase(`/screenshots/${name}-dark.png`)" :alt="alt" loading="lazy" width="1440" height="900" />
  </figure>
</template>
