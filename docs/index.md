---
layout: home

hero:
  name: LLMP
  text: Your LLM keys, shared safely
  tagline: A Kubernetes-native LLM proxy. Give each tenant its own API key for OpenAI, Anthropic or any OpenAI-compatible model, with limits, a dashboard and an audit trail. Your real provider keys never leave the cluster.
  image:
    src: /logo.svg
    alt: LLMP
  actions:
    - theme: brand
      text: Quick start
      link: /guide/quick-start
    - theme: alt
      text: What is LLMP?
      link: /guide/what-is-llmp
    - theme: alt
      text: GitHub
      link: https://github.com/anas1412/llmp

features:
  - title: Virtual keys per tenant
    details: Each app gets its own sk-proxy-… key in its namespace. Disable, rotate or delete it without touching the provider.
  - title: Any provider
    details: OpenAI, Anthropic, and anything OpenAI-compatible (vLLM, Ollama, OpenRouter, Groq, DeepSeek, Azure). OpenAI clients can even use Anthropic models.
  - title: Limits that stick
    details: Per-key model lists, IP ranges, requests and tokens per minute, scaled per tenant. Automatic failover between providers.
  - title: Built-in dashboard
    details: Add providers by pasting a key and ticking models, create keys with copy-paste snippets, watch usage and activity live.
  - title: Audited admin access
    details: Sign-in lockout, revocable sessions, strict browser headers, and every admin change in the activity log and pod log.
  - title: Tiny and boring to run
    details: One 32 MB distroless image, ~15 MB of memory, no database, no Redis. State lives in CRDs and Secrets.
---

<Walkthrough caption="A 78-second tour: sign in, add a provider and pick its models, create a key, copy it, see it in the activity log." />
