---
layout: home

hero:
  name: Valetkey
  text: Minimal LLM Proxy
  tagline: Give each tenant its own API key for OpenAI, Anthropic or any OpenAI-compatible model, configured as CRDs. One small image, no database, and your real provider keys never leave the cluster.
  image:
    src: /logo.svg
    alt: Valetkey
  actions:
    - theme: brand
      text: Quick start
      link: /guide/quick-start
    - theme: alt
      text: What is Valetkey?
      link: /guide/what-is-valetkey
    - theme: alt
      text: GitHub
      link: https://github.com/anas1412/valetkey

features:
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2.586 17.414A2 2 0 0 0 2 18.828V21a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h1a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h.172a2 2 0 0 0 1.414-.586l.814-.814a6.5 6.5 0 1 0-4-4z"/><circle cx="16.5" cy="7.5" r=".5" fill="currentColor"/></svg>'
    title: Virtual keys per tenant
    details: Each app gets its own sk-proxy-… key in its namespace. Disable, rotate or delete it without touching the provider.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m10.586 5.414-5.172 5.172"/><path d="m18.586 13.414-5.172 5.172"/><path d="M6 12h12"/><circle cx="12" cy="20" r="2"/><circle cx="12" cy="4" r="2"/><circle cx="20" cy="12" r="2"/><circle cx="4" cy="12" r="2"/></svg>'
    title: Any provider
    details: OpenAI, Anthropic, and anything OpenAI-compatible (vLLM, Ollama, OpenRouter, Groq, DeepSeek, Azure). OpenAI clients can even use Anthropic models.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m12 14 4-4"/><path d="M3.34 19a10 10 0 1 1 17.32 0"/></svg>'
    title: Limits that stick
    details: Per-key model lists, IP ranges, requests and tokens per minute, scaled per tenant. Automatic failover between providers.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="7" height="9" x="3" y="3" rx="1"/><rect width="7" height="5" x="14" y="3" rx="1"/><rect width="7" height="9" x="14" y="12" rx="1"/><rect width="7" height="5" x="3" y="16" rx="1"/></svg>'
    title: Built-in dashboard
    details: Add providers by pasting a key and ticking models, create keys with copy-paste snippets, watch usage and activity live.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/></svg>'
    title: Access through Kubernetes RBAC
    details: No passwords to manage. Kubernetes RBAC decides who can change what, and every change lands in the Kubernetes audit log.
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.086 18.412A2 2 0 0112.67 19H5v-7.672a2 2 0 01.586-1.414L11.75 3.75a6 6 0 118.49 8.49z"/><path d="M16 8 2 22"/><path d="M17.488 15H9"/></svg>'
    title: Minimal by design
    details: One small static binary, no database, no Redis. Prompts are never stored, and history comes from the Prometheus you already run.
---

<Walkthrough caption="The optional dashboard, through kubectl port-forward: add a provider and pick its models, create a key, copy it, see it in the activity log." />
