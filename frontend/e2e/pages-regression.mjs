// E2E page-render regression for the FIHOS web UI.
//
// Logs in as admin, sets a stale tenant (worst case: empty data, which used to
// crash the dashboard with "Cannot read properties of null (reading 'map')"),
// then visits every route and asserts no uncaught page errors and no error
// banner ("Insufficient permissions" = the earlier /sessions 403 bug).
//
// Usage:
//   node e2e/pages-regression.mjs
// Env overrides: BASE_URL, ADMIN_EMAIL, ADMIN_PASS, STALE_TENANT, CHROME_PATH, PAGES
let puppeteer
try {
  puppeteer = await import('puppeteer-core')
} catch {
  console.error('puppeteer-core is not installed. Install it with:')
  console.error('  npm install --no-save puppeteer-core@23  (from frontend/ or project root)')
  process.exit(2)
}

const CHROME =
  process.env.CHROME_PATH ||
  (process.platform === 'win32'
    ? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
    : '/usr/bin/google-chrome')
const BASE = process.env.BASE_URL || 'http://localhost:8081'
const ADMIN_EMAIL = process.env.ADMIN_EMAIL || 'admin@fihos.dev'
const ADMIN_PASS = process.env.ADMIN_PASS || 'admin12345'
const STALE_TENANT = process.env.STALE_TENANT || '6'
const PAGES = (
  process.env.PAGES ||
  '/dashboard /routers /vouchers /sessions /customers /billing /alerts /rate-windows /team'
).split(' ')

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: 'new',
  args: ['--no-sandbox', '--disable-gpu'],
})
const context = await browser.createBrowserContext()
const page = await context.newPage()
const errors = []
page.on('pageerror', (e) => errors.push(`pageerror: ${e.message}`))

await page.goto(BASE + '/#/dashboard', { waitUntil: 'domcontentloaded' })
await page.waitForSelector('form.login-card', { timeout: 10000 })
await page.type('input[type=email]', ADMIN_EMAIL)
await page.type('input[type=password]', ADMIN_PASS)
await Promise.all([
  page.click('button[type=submit]'),
  page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}),
])
await page.waitForSelector('.sidebar', { timeout: 15000 })

await page.evaluate(
  (tenant) => localStorage.setItem('fihos.activeTenant', String(tenant)),
  STALE_TENANT,
)
await page.reload({ waitUntil: 'domcontentloaded' })
await page.waitForSelector('.sidebar', { timeout: 15000 })

let failed = 0
for (const p of PAGES) {
  errors.length = 0
  await page.goto(BASE + '/#' + p, { waitUntil: 'domcontentloaded' })
  await new Promise((r) => setTimeout(r, 1800))
  const state = await page.evaluate(() => ({
    banner: document.querySelector('.error-banner')?.textContent ?? null,
    content: document.querySelector('.content')?.innerText.replace(/\s+/g, ' ').trim().slice(0, 80) ?? null,
    anyRows: !!document.querySelector('.table tbody tr'),
  }))
  const banned = errors.filter((e) => e.includes('Cannot read properties of null'))
  const forbidden = state.banner?.includes('Insufficient permissions') ?? false
  const ok = errors.length === 0 && !forbidden
  if (!ok) failed++
  console.log(
    `${p.padEnd(14)} ${ok ? 'OK ' : 'FAIL'} errs=${errors.length}${banned.length ? ` nullCrash=${banned.length}` : ''}${forbidden ? ' 403Banner' : ''} rows=${state.anyRows} "${(state.content ?? '')}"`,
  )
}

await context.close()
await browser.close()
if (failed > 0) {
  console.error(`\n${failed}/${PAGES.length} page(s) FAILED`)
  process.exit(1)
}
console.log('\nAll pages OK')