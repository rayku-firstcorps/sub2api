import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import OpsFeishuNotificationCard from '../OpsFeishuNotificationCard.vue'

const { api } = vi.hoisted(() => ({ api: {
  getFeishuNotificationConfig: vi.fn(),
  updateFeishuNotificationConfig: vi.fn(),
  testFeishuNotification: vi.fn()
} }))
vi.mock('@/api/admin/ops', () => ({ opsAPI: api }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)

const config = (configured = false) => ({
  enabled: false, min_severity: '', rate_limit_per_hour: 60,
  include_resolved_alerts: false, webhook_configured: configured, secret_configured: configured
})

async function open(configured = false) {
  api.getFeishuNotificationConfig.mockResolvedValue(config(configured))
  const wrapper = mount(OpsFeishuNotificationCard)
  await flushPromises()
  return wrapper
}

describe('Feishu signed notifications', () => {
  beforeEach(() => vi.resetAllMocks())

  it('requires a signing secret when adding a webhook', async () => {
    const wrapper = await open()
    await wrapper.get('[data-testid="feishu-enabled"]').setValue(true)
    await wrapper.get('#ops-feishu-webhook').setValue('https://open.feishu.cn/open-apis/bot/v2/hook/11111111-2222-3333-4444-555555555555')
    await wrapper.get('form').trigger('submit')
    expect(api.updateFeishuNotificationConfig).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toContain('credentialsRequired')
    await wrapper.get('#ops-feishu-secret').setValue('signing-secret')
    api.updateFeishuNotificationConfig.mockResolvedValue({ ...config(true), enabled: true })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.updateFeishuNotificationConfig).toHaveBeenCalledWith(expect.objectContaining({ enabled: true, signing_secret: 'signing-secret' }))
    expect((wrapper.get('#ops-feishu-secret').element as HTMLInputElement).value).toBe('')
    expect((wrapper.get('#ops-feishu-webhook').element as HTMLInputElement).value).toBe('')
  })

  it('preserves saved credentials when changing unrelated settings', async () => {
    const wrapper = await open(true)
    await wrapper.get('#ops-feishu-limit').setValue('30')
    expect(wrapper.get('[data-testid="feishu-test"]').attributes('disabled')).toBeDefined()
    api.updateFeishuNotificationConfig.mockResolvedValue({ ...config(true), rate_limit_per_hour: 30 })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    const payload = api.updateFeishuNotificationConfig.mock.calls[0]?.[0]
    expect(payload).toEqual({ enabled: false, min_severity: '', rate_limit_per_hour: 30, include_resolved_alerts: false })
    expect(wrapper.get('[data-testid="feishu-test"]').attributes('disabled')).toBeUndefined()
  })

  it('requires explicit clearing and disables the channel atomically', async () => {
    const wrapper = await open(true)
    await wrapper.get('[data-testid="feishu-enabled"]').setValue(true)
    await wrapper.get('[data-testid="feishu-clear"]').setValue(true)
    api.updateFeishuNotificationConfig.mockResolvedValue(config())
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.updateFeishuNotificationConfig).toHaveBeenCalledWith(expect.objectContaining({ enabled: false, clear_credentials: true }))
    expect(wrapper.get('[data-testid="feishu-test"]').attributes('disabled')).toBeDefined()
  })

  it('disables repeat tests while sending and displays safe backend errors', async () => {
    const wrapper = await open(true)
    let reject!: (err: unknown) => void
    api.testFeishuNotification.mockReturnValue(new Promise((_, rej) => { reject = rej }))
    await wrapper.get('[data-testid="feishu-test"]').trigger('click')
    expect(wrapper.get('[data-testid="feishu-test"]').attributes('disabled')).toBeDefined()
    reject({ status: 502, message: 'Feishu rejected message (code 19021)' })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('19021')
    expect(api.testFeishuNotification).toHaveBeenCalledTimes(1)
  })

  it('does not expose a save action after failed loading and allows retry', async () => {
    api.getFeishuNotificationConfig.mockRejectedValue(new Error('offline'))
    const wrapper = mount(OpsFeishuNotificationCard)
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('loadFailed')
    api.getFeishuNotificationConfig.mockResolvedValue(config())
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('form').exists()).toBe(true)
  })

  it('rejects non-official URLs and does not issue a test before saving', async () => {
    const wrapper = await open(true)
    await wrapper.get('#ops-feishu-webhook').setValue('https://evil.example/webhook')
    await wrapper.get('#ops-feishu-secret').setValue('secret')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.get('[role="alert"]').text()).toContain('invalidWebhook')
    expect(api.updateFeishuNotificationConfig).not.toHaveBeenCalled()
    expect(api.testFeishuNotification).not.toHaveBeenCalled()
  })
})
