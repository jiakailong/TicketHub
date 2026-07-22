import { expect, test } from '@playwright/test'

const credentials = {
  mobile: process.env.TICKETHUB_TEST_ADMIN_MOBILE || '',
  password: process.env.TICKETHUB_TEST_ADMIN_PASSWORD || ''
}

async function assertPageIntegrity(page) {
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1)).toBe(true)
  const brokenImages = await page.locator('img:visible').evaluateAll((images) => images.filter((image) => !image.complete || image.naturalWidth === 0).map((image) => image.src))
  expect(brokenImages).toEqual([])
}

async function authenticate(page) {
  if (!credentials.mobile || !credentials.password) {
    throw new Error('TICKETHUB_TEST_ADMIN_MOBILE and TICKETHUB_TEST_ADMIN_PASSWORD are required')
  }
  const response = await page.request.post('http://127.0.0.1:9080/api/users/login', { data: credentials })
  expect(response.ok()).toBeTruthy()
  const body = await response.json()
  await page.goto('/')
  await page.evaluate((data) => {
    localStorage.setItem('tickethub_token', data.access_token)
    localStorage.setItem('tickethub_user', JSON.stringify(data.user))
  }, body.data)
}

test.describe('TicketHub storefront', () => {
  test('registration requests and completes a captcha challenge', async ({ page }) => {
    let registerAttempts = 0
    await page.route('**/api/users/register', async (route) => {
      registerAttempts += 1
      if (registerAttempts === 1) {
        await route.fulfill({
          status: 428,
          contentType: 'application/json',
          body: JSON.stringify({ code: 'CAPTCHA_REQUIRED', message: '请完成验证码验证' }),
        })
        return
      }
      const body = route.request().postDataJSON()
      expect(body.captcha_id).toBe('captcha-1')
      expect(body.captcha_answer).toBe('24680')
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 'OK', data: { id: '1' } }),
      })
    })
    await page.route('**/api/users/register/captcha', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ code: 'OK', data: { captcha_id: 'captcha-1', image: 'data:image/png;base64,iVBORw0KGgo=', expires_in_seconds: 120 } }),
      })
    })

    await page.goto('/register')
    await page.getByPlaceholder('请输入手机号').fill('13800000000')
    await page.getByPlaceholder('至少 6 位').fill('secret')
    await page.getByPlaceholder('再次输入密码').fill('secret')
    await page.locator('.auth-form .submit-button').click()

    await expect(page.getByPlaceholder('请输入验证码')).toBeVisible()
    await page.getByPlaceholder('请输入验证码').fill('24680')
    await page.locator('.auth-form .submit-button').click()
    await expect(page.getByRole('heading', { name: '登录 TicketHub' })).toBeVisible()
    expect(registerAttempts).toBe(2)
  })

  test('home, detail and login pages render cleanly', async ({ page }, testInfo) => {
    const consoleErrors = []
    page.on('console', (message) => { if (message.type() === 'error') consoleErrors.push(message.text()) })

    await page.goto('/')
    await expect(page.getByRole('heading', { name: '正在热售' })).toBeVisible()
    await expect(page.locator('.program-card')).toHaveCount(3)
    await assertPageIntegrity(page)
    await page.screenshot({ path: `.run/screenshots/home-${testInfo.project.name}.png`, fullPage: true })

    await page.goto('/programs/10001')
    await expect(page.getByRole('heading', { name: 'TicketHub Live 2027' })).toBeVisible()
    await expect(page.getByRole('button', { name: /选择座位/ })).toBeVisible()
    await assertPageIntegrity(page)
    await page.screenshot({ path: `.run/screenshots/program-${testInfo.project.name}.png`, fullPage: true })

    await page.goto('/login')
    await expect(page.getByRole('heading', { name: '登录 TicketHub' })).toBeVisible()
    await assertPageIntegrity(page)
    expect(consoleErrors).toEqual([])
  })

  test('authenticated seat and order views use real API data', async ({ page }, testInfo) => {
    await authenticate(page)

    await page.goto('/programs/10001/seats?category=1')
    await expect(page.getByRole('heading', { name: '选择你的座位' })).toBeVisible()
    await expect(page.locator('.seat')).toHaveCount(4)
    await assertPageIntegrity(page)
    await page.screenshot({ path: `.run/screenshots/seats-${testInfo.project.name}.png`, fullPage: true })

    await page.goto('/orders')
    await expect(page.getByRole('heading', { name: '我的订单' })).toBeVisible()
    await expect(page.locator('.order-row').first()).toBeVisible()
    await assertPageIntegrity(page)
    await page.screenshot({ path: `.run/screenshots/orders-${testInfo.project.name}.png`, fullPage: true })

    await page.locator('.order-row').first().click()
    await expect(page.locator('.order-heading')).toBeVisible()
    await assertPageIntegrity(page)
  })

  test('user can create and cancel an asynchronous order', async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== 'desktop', 'mutation flow runs once')
    await authenticate(page)
    await page.goto('/programs/10001/seats?category=1')
    await page.locator('.seat:not(:disabled)').first().click()
    await page.getByRole('button', { name: '确认并填写购票人' }).click()

    await expect(page.getByRole('heading', { name: '确认订单' })).toBeVisible()
    await page.locator('.person-row').first().click()
    await page.getByRole('button', { name: '提交订单' }).click()

    await expect(page.locator('.order-heading')).toBeVisible({ timeout: 15000 })
    await expect(page.getByRole('heading', { name: '等待支付' })).toBeVisible()
    await page.getByRole('button', { name: '取消订单' }).click()
    await page.getByRole('button', { name: '确认取消' }).click()
    await expect(page.getByRole('heading', { name: '订单已关闭' })).toBeVisible()
  })
})
