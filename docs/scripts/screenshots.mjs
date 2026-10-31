// Captures dashboard screenshots for the docs with a local Chromium.
//
//   ADMIN_KEY=... CHROME=/usr/bin/chromium node docs/scripts/screenshots.mjs [http://localhost:8081]
//
// Writes docs/public/screenshots/<page>-<theme>.png at 1440×900.
import { mkdirSync, writeFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { launch, adminCookie, sleep } from './cdp.mjs'

const base = process.argv[2] ?? 'http://localhost:8081'
if (!process.env.ADMIN_KEY) throw new Error('set ADMIN_KEY')
const out = join(dirname(fileURLToPath(import.meta.url)), '..', 'public', 'screenshots')
mkdirSync(out, { recursive: true })

// [file name, hash route, optional action run in the page before the shot]
const shots = [
  ['overview', '#/overview'],
  ['providers', '#/providers'],
  ['add-provider', '#/providers', `[...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Add provider')?.click()`],
  ['tenants', '#/tenants'],
  ['keys', '#/keys'],
  ['key-details', '#/keys', `document.querySelector('tbody tr button')?.click()`],
  ['create-key', '#/keys', `[...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Create key')?.click()`],
  ['activity', '#/activity'],
]

const page = await launch()
try {
  await page.send('Network.setCookie', { ...(await adminCookie(base, process.env.ADMIN_KEY)), url: base })
  for (const theme of ['dark', 'light']) {
    await page.send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: theme }] })
    for (const [file, route, action] of shots) {
      await page.send('Page.navigate', { url: `${base}/${route}` })
      await sleep(1500)
      await page.eval('location.reload()')
      await sleep(2500)
      if (action) {
        await page.eval(action)
        await sleep(1500)
      }
      const { data } = await page.send('Page.captureScreenshot', { format: 'png' })
      writeFileSync(join(out, `${file}-${theme}.png`), Buffer.from(data, 'base64'))
      console.log(`${file}-${theme}.png`)
    }
  }
} finally {
  await page.close()
}
