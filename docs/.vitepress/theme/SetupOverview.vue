<script setup lang="ts">
// What the setup guide builds: Git -> GitOps tool -> the cluster, with Prometheus beside Valetkey.
const folders = [
  ['monitoring/', 'Prometheus + Grafana (optional)'],
  ['install/', 'Valetkey itself'],
  ['config/providers/', 'one file per provider'],
  ['config/tenants/', 'one file per tenant'],
]
</script>

<template>
  <figure class="vk-flow vk-setup" aria-label="Flux or Argo CD applies your Git repository to the cluster, where Valetkey runs next to Prometheus and Grafana">
    <div class="vk-node">
      <div class="vk-title">Your Git repository</div>
      <ul>
        <li v-for="[f, d] in folders" :key="f"><span><code>{{ f }}</code> {{ d }}</span></li>
      </ul>
    </div>
    <div class="vk-arrow">
      <span>every commit</span>
      <svg viewBox="0 0 44 12" aria-hidden="true"><path d="M1 6h40M35 1l6 5-6 5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" /></svg>
    </div>
    <div class="vk-node vk-main">
      <div class="vk-title">Flux or Argo CD</div>
      <div class="vk-sub">Applies the repository to the cluster and keeps it in sync</div>
    </div>
    <div class="vk-arrow">
      <span>applies</span>
      <svg viewBox="0 0 44 12" aria-hidden="true"><path d="M1 6h40M35 1l6 5-6 5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" /></svg>
    </div>
    <div class="vk-node">
      <div class="vk-title">Your cluster</div>
      <div class="vk-inner">
        <div class="vk-title">Valetkey</div>
        <div class="vk-sub">2 pods; your apps call it with their own keys, it calls the providers with the real ones</div>
      </div>
      <div class="vk-inner">
        <div class="vk-title">Prometheus + Grafana</div>
        <div class="vk-sub">Prometheus scrapes Valetkey; Valetkey reads 7 and 30 days of history back</div>
      </div>
    </div>
    <figcaption>Generated keys and status never go into Git: Valetkey writes them into the cluster.</figcaption>
  </figure>
</template>
