import { describe, expect, it, vi, beforeEach, beforeAll } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

let UsageTable: typeof import('../UsageTable.vue')['default']

const installStorageStub = () => {
  const store = new Map<string, string>()
  const storage = {
    getItem: vi.fn((key: string) => store.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => {
      store.set(key, String(value))
    }),
    removeItem: vi.fn((key: string) => {
      store.delete(key)
    }),
    clear: vi.fn(() => {
      store.clear()
    }),
    key: vi.fn((index: number) => Array.from(store.keys())[index] ?? null),
    get length() {
      return store.size
    },
  }

  Object.defineProperty(globalThis, 'localStorage', {
    value: storage,
    configurable: true,
  })
  Object.defineProperty(window, 'localStorage', {
    value: storage,
    configurable: true,
  })
}

const messages: Record<string, string> = {
  'usage.costDetails': 'Cost Breakdown',
  'admin.usage.inputCost': 'Input Cost',
  'admin.usage.outputCost': 'Output Cost',
  'admin.usage.cacheCreationCost': 'Cache Creation Cost',
  'admin.usage.cacheReadCost': 'Cache Read Cost',
  'usage.inputTokenPrice': 'Input price',
  'usage.outputTokenPrice': 'Output price',
  'usage.perMillionTokens': '/ 1M tokens',
  'usage.serviceTier': 'Service tier',
  'usage.serviceTierPriority': 'Fast',
  'usage.serviceTierFlex': 'Flex',
  'usage.serviceTierStandard': 'Standard',
  'usage.rate': 'Rate',
  'usage.accountMultiplier': 'Account rate',
  'usage.original': 'Original',
  'usage.userBilled': 'User billed',
  'usage.accountBilled': 'Account billed',
  'usage.model': 'Model',
  'usage.type': 'Type',
  'admin.usage.requestContext': 'Request Context',
  'admin.usage.viewRequestContext': 'View request context',
  'admin.usage.requestContextTruncated': 'Truncated',
  'admin.usage.messageCount': 'Messages',
  'admin.usage.latestUserPrompt': 'Latest User Prompt',
  'admin.usage.fullRequestContext': 'Full Request Context',
  'admin.usage.requestContextCopied': 'Request context copied',
  'common.copy': 'Copy',
  'common.export': 'Export',
  'common.close': 'Close',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const copyToClipboard = vi.fn().mockResolvedValue(true)

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.request_id">
        <slot name="cell-model" :row="row" :value="row.model" />
        <slot name="cell-cost" :row="row" />
        <slot name="cell-context" :row="row" />
      </div>
    </div>
  `,
}

describe('admin UsageTable tooltip', () => {
  beforeAll(async () => {
    installStorageStub()
    UsageTable = (await import('../UsageTable.vue')).default
  })

  beforeEach(() => {
    localStorage.clear()
    copyToClipboard.mockClear()
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      top: 20,
      left: 20,
      right: 120,
      bottom: 40,
      width: 100,
      height: 20,
      toJSON: () => ({}),
    } as DOMRect)
  })

  it('shows service tier and billing breakdown in cost tooltip', async () => {
    const row = {
      request_id: 'req-admin-1',
      actual_cost: 0.092883,
      total_cost: 0.092883,
      account_rate_multiplier: 1,
      rate_multiplier: 1,
      service_tier: 'priority',
      input_cost: 0.020285,
      output_cost: 0.00303,
      cache_creation_cost: 0,
      cache_read_cost: 0.069568,
      input_tokens: 4057,
      output_tokens: 101,
    }

    const wrapper = mount(UsageTable, {
      props: {
        data: [row],
        loading: false,
        columns: [],
      },
      global: {
        stubs: {
          DataTable: DataTableStub,
          EmptyState: true,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await wrapper.find('.group.relative').trigger('mouseenter')
    await nextTick()

    const text = wrapper.text()
    expect(text).toContain('Service tier')
    expect(text).toContain('Fast')
    expect(text).toContain('Rate')
    expect(text).toContain('1.00x')
    expect(text).toContain('Account rate')
    expect(text).toContain('User billed')
    expect(text).toContain('Account billed')
    expect(text).toContain('$0.092883')
    expect(text).toContain('$5.0000 / 1M tokens')
    expect(text).toContain('$30.0000 / 1M tokens')
    expect(text).toContain('$0.069568')
  })

  it('shows requested and upstream models separately for admin rows', () => {
    const row = {
      request_id: 'req-admin-model-1',
      model: 'claude-sonnet-4',
      upstream_model: 'claude-sonnet-4-20250514',
      actual_cost: 0,
      total_cost: 0,
      account_rate_multiplier: 1,
      rate_multiplier: 1,
      input_cost: 0,
      output_cost: 0,
      cache_creation_cost: 0,
      cache_read_cost: 0,
      input_tokens: 0,
      output_tokens: 0,
    }

    const wrapper = mount(UsageTable, {
      props: {
        data: [row],
        loading: false,
        columns: [],
      },
      global: {
        stubs: {
          DataTable: DataTableStub,
          EmptyState: true,
          Icon: true,
          Teleport: true,
        },
      },
    })

    const text = wrapper.text()
    expect(text).toContain('claude-sonnet-4')
    expect(text).toContain('claude-sonnet-4-20250514')
  })

  it('opens sanitized request context details for admin rows', async () => {
    const row = {
      request_id: 'req-admin-context-1',
      model: 'gpt-5.1',
      actual_cost: 0,
      total_cost: 0,
      account_rate_multiplier: 1,
      rate_multiplier: 1,
      input_cost: 0,
      output_cost: 0,
      cache_creation_cost: 0,
      cache_read_cost: 0,
      input_tokens: 0,
      output_tokens: 0,
      request_context_json: {
        model: 'gpt-5.1',
        messages: [
          { role: 'system', content: 'system prompt' },
          { role: 'user', content: 'old user prompt' },
          { role: 'assistant', content: 'assistant reply' },
          { role: 'user', content: 'hello from context' },
        ],
        api_key: '[REDACTED]',
      },
      request_context_truncated: true,
      request_context_bytes: 2048,
    }

    const wrapper = mount(UsageTable, {
      props: {
        data: [row],
        loading: false,
        columns: [],
      },
      global: {
        stubs: {
          DataTable: DataTableStub,
          EmptyState: true,
          Icon: true,
          Teleport: true,
        },
      },
    })

    const contextButton = wrapper.find('button[title="View request context"]')
    expect(contextButton.exists()).toBe(true)

    await contextButton.trigger('click')
    await nextTick()

    const text = wrapper.text()
    expect(text).toContain('Request Context')
    expect(text).toContain('req-admin-context-1')
    expect(text).toContain('2.0 KB')
    expect(text).toContain('Truncated')
    expect(text).toContain('Messages')
    expect(text).toContain('4')
    expect(text).toContain('Latest User Prompt')
    expect(text).toContain('messages:last user')
    expect(text).toContain('hello from context')
    expect(text).toContain('Full Request Context')
    expect(text).toContain('Copy')
    expect(text).toContain('Export')
    expect(text).toContain('old user prompt')
    expect(text).toContain('[REDACTED]')
  })

  it('extracts the latest user prompt from responses input context', async () => {
    const row = {
      request_id: 'req-admin-context-responses',
      model: 'gpt-5.1',
      actual_cost: 0,
      total_cost: 0,
      account_rate_multiplier: 1,
      rate_multiplier: 1,
      input_cost: 0,
      output_cost: 0,
      cache_creation_cost: 0,
      cache_read_cost: 0,
      input_tokens: 0,
      output_tokens: 0,
      request_context_json: {
        model: 'gpt-5.1',
        input: [
          { type: 'message', role: 'developer', content: [{ type: 'input_text', text: 'developer instructions' }] },
          { type: 'message', role: 'user', content: [{ type: 'input_text', text: 'first prompt' }] },
          { type: 'message', role: 'user', content: [{ type: 'input_text', text: 'latest prompt' }] },
        ],
      },
      request_context_truncated: false,
      request_context_bytes: 512,
    }

    const wrapper = mount(UsageTable, {
      props: {
        data: [row],
        loading: false,
        columns: [],
      },
      global: {
        stubs: {
          DataTable: DataTableStub,
          EmptyState: true,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await wrapper.find('button[title="View request context"]').trigger('click')
    await nextTick()

    const promptBlock = wrapper.find('section pre')
    expect(promptBlock.text()).toContain('latest prompt')
    expect(promptBlock.text()).not.toContain('developer instructions')
    expect(promptBlock.text()).not.toContain('first prompt')
    expect(wrapper.text()).toContain('input:last user')
  })

  it('copies and exports the full request context JSON', async () => {
    const originalCreateObjectURL = URL.createObjectURL
    const originalRevokeObjectURL = URL.revokeObjectURL
    URL.createObjectURL = vi.fn(() => 'blob:request-context') as typeof URL.createObjectURL
    URL.revokeObjectURL = vi.fn() as typeof URL.revokeObjectURL
    const clickMock = vi.fn()
    const originalCreateElement = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tagName: string, options?: ElementCreationOptions) => {
      const element = originalCreateElement(tagName, options)
      if (tagName.toLowerCase() === 'a') {
        Object.defineProperty(element, 'click', { value: clickMock })
      }
      return element
    })

    const row = {
      request_id: 'req/export 1',
      model: 'gpt-5.1',
      actual_cost: 0,
      total_cost: 0,
      account_rate_multiplier: 1,
      rate_multiplier: 1,
      input_cost: 0,
      output_cost: 0,
      cache_creation_cost: 0,
      cache_read_cost: 0,
      input_tokens: 0,
      output_tokens: 0,
      request_context_json: {
        model: 'gpt-5.1',
        input: 'export me',
      },
      request_context_truncated: false,
      request_context_bytes: 128,
    }

    const wrapper = mount(UsageTable, {
      props: {
        data: [row],
        loading: false,
        columns: [],
      },
      global: {
        stubs: {
          DataTable: DataTableStub,
          EmptyState: true,
          Icon: true,
          Teleport: true,
        },
      },
    })

    await wrapper.find('button[title="View request context"]').trigger('click')
    await nextTick()

    const actionButtons = wrapper.findAll('section button')
    await actionButtons.find((button) => button.text().includes('Copy'))!.trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith(expect.stringContaining('"input": "export me"'), 'Request context copied')

    await actionButtons.find((button) => button.text().includes('Export'))!.trigger('click')
    expect(URL.createObjectURL).toHaveBeenCalled()
    expect(clickMock).toHaveBeenCalled()
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:request-context')

    URL.createObjectURL = originalCreateObjectURL
    URL.revokeObjectURL = originalRevokeObjectURL
  })
})
