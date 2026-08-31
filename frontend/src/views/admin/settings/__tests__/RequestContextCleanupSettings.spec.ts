import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

import RequestContextCleanupSettings from '../RequestContextCleanupSettings.vue'

const {
  getSettings,
  updateSettings,
  getRequestContextCleanupTask,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  getRequestContextCleanupTask: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    settings: {
      getSettings: (...args: unknown[]) => getSettings(...args),
      updateSettings: (...args: unknown[]) => updateSettings(...args),
    },
    usage: {
      getRequestContextCleanupTask: (...args: unknown[]) => getRequestContextCleanupTask(...args),
      createRequestContextCleanupTask: vi.fn(),
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
  }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    locale: ref('zh-CN'),
  }),
}))

describe('RequestContextCleanupSettings', () => {
  beforeEach(() => {
    getSettings.mockReset()
    updateSettings.mockReset()
    getRequestContextCleanupTask.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    getRequestContextCleanupTask.mockResolvedValue(null)
    getSettings.mockResolvedValue({
      usage_log_request_context_enabled: true,
      usage_log_request_context_skip_api_key_ids: [7],
    })
    updateSettings.mockImplementation(async (payload: Record<string, unknown>) => ({
      usage_log_request_context_enabled: true,
      usage_log_request_context_skip_api_key_ids:
        payload.usage_log_request_context_skip_api_key_ids ?? [7],
    }))
  })

  it('loads skip whitelist IDs and adds a new API key ID', async () => {
    const wrapper = mount(RequestContextCleanupSettings, {
      global: { stubs: { Icon: true, ConfirmDialog: true } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('#7')

    await wrapper.get('input[type="text"]').setValue('42, 7')
    await wrapper.get('button.btn-secondary').trigger('click')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledWith({
      usage_log_request_context_skip_api_key_ids: [7, 42],
    })
    expect(wrapper.text()).toContain('#42')
    expect(showSuccess).toHaveBeenCalled()
  })

  it('rejects invalid API key IDs', async () => {
    const wrapper = mount(RequestContextCleanupSettings, {
      global: { stubs: { Icon: true, ConfirmDialog: true } },
    })
    await flushPromises()

    await wrapper.get('input[type="text"]').setValue('abc')
    await wrapper.get('button.btn-secondary').trigger('click')
    await flushPromises()

    expect(updateSettings).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalled()
  })
})
