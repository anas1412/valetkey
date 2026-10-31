<script setup lang="ts">
// How one request goes through Valetkey: the app's own key in, the real key out.
const checks = ['key enabled and not expired', 'model allowed', 'client IP allowed', 'within its rate limits', 'within its budget']
</script>

<template>
  <figure class="vk-flow" aria-label="An app calls Valetkey with its own key; Valetkey checks the key's rules and forwards the request to the provider with the real key">
    <div class="vk-node">
      <div class="vk-title">Your app</div>
      <div class="vk-sub">Any OpenAI or Anthropic SDK, with its own key</div>
      <div class="vk-sub"><code>sk-proxy-…</code></div>
    </div>
    <div class="vk-arrow">
      <span>its key</span>
      <svg viewBox="0 0 44 12" aria-hidden="true"><path d="M1 6h40M35 1l6 5-6 5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" /></svg>
    </div>
    <div class="vk-node vk-main">
      <div class="vk-title">Valetkey</div>
      <ul>
        <li v-for="c in checks" :key="c">
          <svg viewBox="0 0 12 12" aria-hidden="true"><path d="M2 6.5l2.5 2.5L10 3" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" /></svg>
          {{ c }}
        </li>
      </ul>
      <div class="vk-sub">then swaps in the real key</div>
    </div>
    <div class="vk-arrow">
      <span>real key</span>
      <svg viewBox="0 0 44 12" aria-hidden="true"><path d="M1 6h40M35 1l6 5-6 5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" /></svg>
    </div>
    <div class="vk-node">
      <div class="vk-title">Provider</div>
      <div class="vk-sub">OpenAI, Anthropic, DeepSeek, or any OpenAI-compatible API such as vLLM or Ollama</div>
    </div>
    <figcaption>The real provider key never leaves your cluster. Each tenant only ever holds its own key.</figcaption>
  </figure>
</template>
