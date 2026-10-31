// Records a walkthrough video of the dashboard: overview → add a provider and pick
// its models → create an API key → copy it → activity. Needs ffmpeg and Chromium.
//
//   kubectl port-forward -n valetkey-system deploy/valetkey 8082:8082 &
//   CHROME=/usr/bin/chromium node docs/scripts/walkthrough.mjs [http://localhost:8082]
//
// Writes docs/public/walkthrough.mp4 (H.264, 1440×900) and a poster image.
// Expects a tenant "research" and a provider serving an OpenAI-compatible /v1/models at PROVIDER_URL.
import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { launch, sleep } from './cdp.mjs'

const base = process.argv[2] ?? 'http://localhost:8082'
const providerURL = process.env.PROVIDER_URL ?? 'http://127.0.0.1:9101'
const out = join(dirname(fileURLToPath(import.meta.url)), '..', 'public')
mkdirSync(out, { recursive: true })

// Page helpers: a visible cursor, finding elements by text, React-friendly typing.
const helpers = `
window.__demo = (() => {
  const cursor = document.createElement('div');
  cursor.style.cssText = 'position:fixed;z-index:99999;left:720px;top:450px;width:22px;height:22px;margin:-11px 0 0 -11px;border-radius:50%;' +
    'background:rgba(99,102,241,.35);border:2px solid #6366f1;pointer-events:none;transition:left .6s cubic-bezier(.2,.8,.2,1),top .6s cubic-bezier(.2,.8,.2,1),transform .15s';
  const ready = () => document.body && !cursor.isConnected && document.body.appendChild(cursor);
  const visible = (el) => el && el.getClientRects().length > 0;
  const byText = (sel, text) => [...document.querySelectorAll(sel)].filter(visible).find((e) => e.textContent.trim() === text)
    ?? [...document.querySelectorAll(sel)].filter(visible).find((e) => e.textContent.trim().includes(text));
  const moveTo = async (el) => {
    ready();
    el.scrollIntoView({ block: 'center', behavior: 'instant' });
    const r = el.getBoundingClientRect();
    cursor.style.left = (r.left + r.width / 2) + 'px';
    cursor.style.top = (r.top + r.height / 2) + 'px';
    await new Promise((res) => setTimeout(res, 700));
  };
  const click = async (el) => {
    if (!el) throw new Error('element not found');
    await moveTo(el);
    cursor.style.transform = 'scale(.7)';
    await new Promise((res) => setTimeout(res, 120));
    cursor.style.transform = '';
    el.click();
  };
  const type = async (el, text) => {
    if (!el) throw new Error('input not found');
    await click(el);
    el.focus();
    const proto = el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const set = Object.getOwnPropertyDescriptor(proto, 'value').set;
    for (const ch of text) {
      set.call(el, el.value + ch);
      el.dispatchEvent(new Event('input', { bubbles: true }));
      await new Promise((res) => setTimeout(res, 55));
    }
  };
  return { byText, click, type, moveTo, ready };
})();`

const steps = [
  // Overview
  [3000, `__demo.moveTo(__demo.byText('nav button, nav a, aside button', 'Overview'))`],
  // Add a provider and pick its models from the provider's own list
  [2500, `__demo.click(__demo.byText('nav button, nav a, aside button', 'Providers'))`],
  [1500, `__demo.click(__demo.byText('button', 'Add provider'))`],
  [1200, `__demo.click(__demo.byText('[role=dialog] button', 'OpenAI'))`],
  [1200, `__demo.click(__demo.byText('[role=option], [role=dialog] li, [role=dialog] button', 'Other (OpenAI-compatible)'))`],
  [600, `__demo.type(document.querySelector('input[placeholder="openai"]'), 'llama-server')`],
  [600, `__demo.type(document.querySelector('input[placeholder="https://my-endpoint.example.com"]'), ${JSON.stringify(providerURL)})`],
  [600, `__demo.type(document.querySelector('input[placeholder="sk-…"]'), 'local-server-token')`],
  [2500, `__demo.click(__demo.byText('button', 'Choose specific models instead'))`],
  [900, `__demo.click(__demo.byText('[role=dialog] label', 'llama-3.1-8b-instruct'))`],
  [1200, `__demo.click(__demo.byText('[role=dialog] label', 'qwen2.5-7b-instruct'))`],
  [3500, `__demo.click([...document.querySelectorAll('[role=dialog] button[type=submit]')].pop())`],
  // Create an API key for a tenant, limited to one model
  [2500, `__demo.click(__demo.byText('nav button, nav a, aside button', 'API keys'))`],
  [1500, `__demo.click(__demo.byText('button', 'Create key'))`],
  [1200, `__demo.click(document.querySelector('[role=dialog] [role=combobox]'))`],
  [1000, `__demo.click(__demo.byText('[role=option]', 'research'))`],
  [600, `__demo.type(document.querySelector('input[placeholder="billing-service"]'), 'chat-app')`],
  [2500, `__demo.click(document.querySelector('[role=dialog] [role=switch]'))`],
  [1200, `__demo.click(__demo.byText('[role=dialog] label', 'qwen2.5-7b-instruct'))`],
  [4000, `__demo.click([...document.querySelectorAll('[role=dialog] button[type=submit]')].pop())`],
  // Open the new key: usage snippets, copy
  [2500, `__demo.click([...document.querySelectorAll('tbody tr')].find((r) => r.textContent.includes('chat-app'))?.querySelector('button'))`],
  [1500, `__demo.moveTo(__demo.byText('button', 'Python'))`],
  [2500, `__demo.click(__demo.byText('button', 'Python'))`],
  [3000, `__demo.click([...document.querySelectorAll('tbody tr')].find((r) => r.textContent.includes('chat-app'))?.querySelector('button:not([aria-label])'))`],
  // Activity
  [4000, `__demo.click(__demo.byText('nav button, nav a, aside button', 'Activity'))`],
]

const page = await launch()
const frames = []
const dir = mkdtempSync(join(tmpdir(), 'valetkey-video-'))
try {
  await page.send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'dark' }] })
  await page.send('Page.enable')
  page.on('Page.screencastFrame', ({ data, metadata, sessionId }) => {
    frames.push({ data, t: metadata.timestamp })
    void page.send('Page.screencastFrameAck', { sessionId })
  })
  await page.send('Page.navigate', { url: `${base}/#/overview` })
  await sleep(2500)
  // The dashboard is a single page (no reloads), so the helpers stay loaded for the whole run.
  await page.eval(helpers)
  await page.eval('__demo.ready()')
  await page.send('Page.startScreencast', { format: 'jpeg', quality: 92, maxWidth: 1440, maxHeight: 900 })

  for (const [wait, action] of steps) {
    const err = await page.eval(`(async () => { try { await ${action}; return '' } catch (e) { return String(e) } })()`)
    if (err) console.warn(`step failed: ${action.slice(0, 80)} → ${err}`)
    await sleep(wait)
  }
  await page.send('Page.stopScreencast')
} finally {
  await page.close()
}

// Frames arrive only when the page changes: give each frame its real duration.
let list = ''
frames.forEach((f, i) => {
  const file = join(dir, `${String(i).padStart(5, '0')}.jpg`)
  writeFileSync(file, Buffer.from(f.data, 'base64'))
  const next = frames[i + 1]?.t ?? f.t + 2
  list += `file '${file}'\nduration ${Math.max(0.01, next - f.t).toFixed(3)}\n`
})
list += `file '${join(dir, `${String(frames.length - 1).padStart(5, '0')}.jpg`)}'\n`
writeFileSync(join(dir, 'frames.txt'), list)

execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-f', 'concat', '-safe', '0', '-i', join(dir, 'frames.txt'),
  '-vf', 'fps=30,format=yuv420p', '-c:v', 'libx264', '-preset', 'slow', '-crf', '24', '-movflags', '+faststart',
  join(out, 'walkthrough.mp4')])
execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-i', join(out, 'walkthrough.mp4'), '-ss', '6', '-frames:v', '1',
  join(out, 'walkthrough-poster.jpg')])
rmSync(dir, { recursive: true, force: true })
console.log(`walkthrough.mp4 — ${frames.length} frames`)
