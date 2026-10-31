import { defineConfig } from 'vitepress'

// Served from GitHub Pages at /llmp/; override with DOCS_BASE for other hosts.
const base = process.env.DOCS_BASE ?? '/llmp/'

export default defineConfig({
  base,
  title: 'LLMP',
  description: 'Kubernetes-native LLM proxy: per-tenant API keys for OpenAI, Anthropic and any OpenAI-compatible model.',
  cleanUrls: true,
  // Links to the port-forwarded dashboard (http://localhost:8081) are intentional.
  ignoreDeadLinks: 'localhostLinks',
  lastUpdated: true,
  head: [['link', { rel: 'icon', type: 'image/svg+xml', href: `${base}logo.svg` }]],
  themeConfig: {
    logo: '/logo.svg',
    nav: [
      { text: 'Guide', link: '/guide/what-is-llmp' },
      { text: 'Reference', link: '/reference/configuration' },
      { text: 'Releases', link: 'https://github.com/anas1412/llmp/releases' },
    ],
    sidebar: [
      {
        text: 'Introduction',
        items: [
          { text: 'What is LLMP?', link: '/guide/what-is-llmp' },
          { text: 'Quick start', link: '/guide/quick-start' },
          { text: 'Dashboard tour', link: '/guide/dashboard' },
        ],
      },
      {
        text: 'Guides',
        items: [
          { text: 'Providers', link: '/guide/providers' },
          { text: 'Tenants', link: '/guide/tenants' },
          { text: 'API keys', link: '/guide/api-keys' },
          { text: 'Using a key in your app', link: '/guide/using-keys' },
          { text: 'Limits & access rules', link: '/guide/limits' },
          { text: 'Security & audit trail', link: '/guide/security' },
          { text: 'Monitoring', link: '/guide/monitoring' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Configuration', link: '/reference/configuration' },
          { text: 'Custom resources', link: '/reference/crds' },
          { text: 'Admin API', link: '/reference/admin-api' },
          { text: 'Development', link: '/reference/development' },
        ],
      },
    ],
    socialLinks: [{ icon: 'github', link: 'https://github.com/anas1412/llmp' }],
    editLink: { pattern: 'https://github.com/anas1412/llmp/edit/main/docs/:path', text: 'Edit this page on GitHub' },
    search: { provider: 'local' },
    footer: { message: 'Released under the ISC License.' },
  },
})
