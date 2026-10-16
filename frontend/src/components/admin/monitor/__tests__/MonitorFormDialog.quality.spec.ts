import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MonitorFormDialog from '../MonitorFormDialog.vue'
import MonitorRunResultDialog from '../MonitorRunResultDialog.vue'
import type { ChannelMonitor } from '@/api/admin/channelMonitor'

const { create, update, showError } = vi.hoisted(() => ({
  create: vi.fn(), update: vi.fn(), showError: vi.fn(),
}))
vi.mock('@/api/admin', () => ({ adminAPI: {
  channelMonitor: { create, update },
  channelMonitorTemplate: { list: vi.fn().mockResolvedValue({ items: [] }) },
  accounts: { list: vi.fn().mockResolvedValue({ items: [] }) },
} }))
vi.mock('@/api/keys', () => ({ keysAPI: { list: vi.fn() } }))
vi.mock('@/api/groups', () => ({ userGroupsAPI: { getUserGroupRates: vi.fn() } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ cachedPublicSettings: null, showError, showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))

const BaseDialog = defineComponent({ template: '<div><slot /><slot name="footer" /></div>' })
function mountForm(monitor: ChannelMonitor | null = null) {
  return mount(MonitorFormDialog, {
    props: { show: true, monitor },
    global: { stubs: {
      BaseDialog, Toggle: true, Select: true, ModelTagInput: true,
      MonitorKeyPickerDialog: true, MonitorAdvancedRequestConfig: true,
    } },
  })
}
const savedMonitor = {
  id: 1, name: 'Quality probe', provider: 'openai', api_mode: 'responses',
  check_mode: 'probe', endpoint: 'https://example.com', primary_model: 'test-model',
  extra_models: [], enabled: true, interval_seconds: 60,
  reasoning_effort: 'high', prompt: '只回答最终结果', expected_answer: '42', notify_feishu: true,
} as ChannelMonitor

describe('channel monitor quality configuration', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('saves prompt, expected answer, effort and notification preference on create', async () => {
    const wrapper = mountForm()
    await flushPromises()
    await wrapper.get('input[type="text"]').setValue('Quality probe')
    await wrapper.get('[data-testid="monitor-primary-model"]').setValue('test-model')
    await wrapper.get('#monitor-effort').setValue('high')
    await wrapper.get('#monitor-prompt').setValue('只回答最终结果')
    await wrapper.get('#monitor-expected-answer').setValue('42')
    await wrapper.get('#monitor-notify-feishu').setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(create).toHaveBeenCalledWith(expect.objectContaining({
      reasoning_effort: 'high', prompt: '只回答最终结果', expected_answer: '42', notify_feishu: true,
    }))
    wrapper.unmount()
  })

  it('rehydrates existing settings and explicitly clears them on update', async () => {
    const wrapper = mountForm(savedMonitor)
    await flushPromises()
    expect((wrapper.get('#monitor-prompt').element as HTMLTextAreaElement).value).toBe(savedMonitor.prompt)
    expect((wrapper.get('#monitor-effort').element as HTMLSelectElement).value).toBe('high')
    expect((wrapper.get('#monitor-notify-feishu').element as HTMLInputElement).checked).toBe(true)
    await wrapper.get('#monitor-prompt').setValue('')
    await wrapper.get('#monitor-expected-answer').setValue('')
    await wrapper.get('#monitor-effort').setValue('')
    await wrapper.get('#monitor-notify-feishu').setValue(false)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(1, expect.objectContaining({
      reasoning_effort: '', prompt: '', expected_answer: '', notify_feishu: false,
    }))
    wrapper.unmount()
  })

  it('blocks missing answers and replace-body conflicts with inline errors', async () => {
    const wrapper = mountForm(savedMonitor)
    await wrapper.get('#monitor-expected-answer').setValue(' ')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.get('[role="alert"]').text()).toContain('pairRequired')
    expect(update).not.toHaveBeenCalled()
    await wrapper.setProps({ monitor: { ...savedMonitor, body_override_mode: 'replace' } })
    await wrapper.get('form').trigger('submit')
    expect(wrapper.get('[role="alert"]').text()).toContain('replaceConflict')
    expect(update).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('clears quality configuration when switching to quota-only', async () => {
    const wrapper = mountForm(savedMonitor)
    await flushPromises()
    await wrapper.get('[data-testid="monitor-check-mode-quota"]').trigger('click')
    expect(wrapper.find('#monitor-prompt').exists()).toBe(false)
    // Selecting a linked account is required independently of quality settings.
    wrapper.findComponent({ name: 'Select' }).vm.$emit('update:modelValue', '5')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(1, expect.objectContaining({
      check_mode: 'quota', reasoning_effort: '', prompt: '', expected_answer: '', notify_feishu: false,
    }))
    wrapper.unmount()
  })

  it('renders quality mismatch separately from notification delivery failure', () => {
    const wrapper = mount(MonitorRunResultDialog, {
      props: { show: true, results: [{ model: 'test-model', status: 'quality_failed', latency_ms: 10, ping_latency_ms: 1,
        checked_at: '2026-10-16T00:00:00Z', message: 'expected 42; got 41', notification_error: 'not sent' }] },
      global: { stubs: { BaseDialog, MonitorQuotaView: true } },
    })
    expect(wrapper.text()).toContain('monitorCommon.status.quality_failed')
    expect(wrapper.get('[role="alert"]').text()).toContain('notificationFailed')
    expect(wrapper.text()).toContain('expected 42; got 41')
    wrapper.unmount()
  })
})
