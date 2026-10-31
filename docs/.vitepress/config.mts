import { defineConfig } from 'vitepress'

// Served from GitHub Pages (anas.cyberia.tn/valetkey/); override with DOCS_BASE for other hosts.
const base = process.env.DOCS_BASE ?? '/valetkey/'

export default defineConfig({
  base,
  title: 'Valetkey',
  description: 'Valetkey, the minimal LLM proxy for Kubernetes: per-tenant API keys for OpenAI, Anthropic and any OpenAI-compatible model, configured as CRDs.',
  cleanUrls: true,
  // Links to the port-forwarded dashboard (http://localhost:8081) are intentional.
  ignoreDeadLinks: 'localhostLinks',
  lastUpdated: true,
  head: [['link', { rel: 'icon', type: 'image/svg+xml', href: `${base}logo.svg` }]],
  themeConfig: {
    logo: '/logo.svg',
    nav: [
      { text: 'Guide', link: '/guide/quick-start' },
      { text: 'Reference', link: '/reference/helm' },
      { text: 'Releases', link: 'https://github.com/anas1412/valetkey/releases' },
    ],
    sidebar: [
      {
        text: 'Introduction',
        items: [
          { text: 'What is Valetkey?', link: '/guide/what-is-valetkey' },
          { text: 'Quick start', link: '/guide/quick-start' },
        ],
      },
      {
        text: 'Working with Valetkey',
        items: [
          { text: 'kubectl & GitOps', link: '/guide/kubectl' },
          { text: 'valetkeyctl CLI', link: '/guide/cli' },
          { text: 'Providers', link: '/guide/providers' },
          { text: 'Tenants', link: '/guide/tenants' },
          { text: 'API keys', link: '/guide/api-keys' },
          { text: 'Using a key in your app', link: '/guide/using-keys' },
          { text: 'Limits & access rules', link: '/guide/limits' },
          { text: 'Budgets & costs', link: '/guide/budgets' },
        ],
      },
      {
        text: 'Operations',
        items: [
          { text: 'Security & access', link: '/guide/security' },
          { text: 'Monitoring', link: '/guide/monitoring' },
          { text: 'Usage history', link: '/guide/usage-history' },
          { text: 'Dashboard (optional)', link: '/guide/dashboard' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Helm values', link: '/reference/helm' },
          { text: 'Configuration', link: '/reference/configuration' },
          { text: 'Custom resources', link: '/reference/crds' },
          { text: 'Admin API', link: '/reference/admin-api' },
          { text: 'Development', link: '/reference/development' },
        ],
      },
    ],
    socialLinks: [{ icon: 'github', link: 'https://github.com/anas1412/valetkey' }],
    editLink: { pattern: 'https://github.com/anas1412/valetkey/edit/main/docs/:path', text: 'Edit this page on GitHub' },
    search: { provider: 'local' },
    footer: { message: 'Released under the ISC License.' },
  },
})
