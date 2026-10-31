// Minimal Chrome DevTools Protocol client over Node's built-in WebSocket.
// Launches a headless Chromium with a throwaway profile.
import { spawn } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

export async function launch({ chrome = process.env.CHROME ?? 'chromium', width = 1440, height = 900 } = {}) {
  const profile = mkdtempSync(join(tmpdir(), 'valetkey-cdp-'))
  const port = 9300 + Math.floor(Math.random() * 600)
  const browser = spawn(chrome, ['--headless=new', `--remote-debugging-port=${port}`, `--user-data-dir=${profile}`,
    '--hide-scrollbars', '--no-first-run', `--window-size=${width},${height}`, 'about:blank'], { stdio: 'ignore' })

  let target
  for (let i = 0; i < 50 && !target; i++) {
    await sleep(200)
    target = await fetch(`http://127.0.0.1:${port}/json`).then((r) => r.json())
      .then((l) => l.find((t) => t.type === 'page')).catch(() => undefined)
  }
  if (!target) throw new Error('Chromium did not start')

  const ws = new WebSocket(target.webSocketDebuggerUrl)
  await new Promise((r) => ws.addEventListener('open', r, { once: true }))
  let id = 0
  const pending = new Map()
  const listeners = new Map()
  ws.addEventListener('message', (e) => {
    const msg = JSON.parse(e.data)
    if (msg.id) {
      pending.get(msg.id)?.(msg.result ?? {})
      pending.delete(msg.id)
    } else {
      listeners.get(msg.method)?.(msg.params)
    }
  })
  const send = (method, params = {}) => new Promise((r) => {
    pending.set(++id, r)
    ws.send(JSON.stringify({ id, method, params }))
  })
  await send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile: false })

  return {
    send,
    on: (method, fn) => listeners.set(method, fn),
    /** Evaluates an expression in the page and returns its value (awaits promises). */
    eval: async (expression) => (await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })).result?.value,
    async close() {
      ws.close()
      const exited = new Promise((r) => browser.once('exit', r))
      browser.kill()
      await exited
      rmSync(profile, { recursive: true, force: true, maxRetries: 5 })
    },
  }
}
