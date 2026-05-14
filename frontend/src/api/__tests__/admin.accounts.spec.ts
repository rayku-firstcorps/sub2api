import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    post,
  },
}))

import { importKiroAccounts } from '@/api/admin/accounts'

describe('admin accounts api', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('posts Kiro refresh token imports to the registered backend route', async () => {
    const payload = {
      refresh_tokens: ['kiro-rt'],
      auth_method: 'social' as const,
      region: 'us-east-1',
    }
    const response = { total: 1, success: 1, failed: 0, items: [] }
    post.mockResolvedValue({ data: response })

    const result = await importKiroAccounts(payload)

    expect(post).toHaveBeenCalledWith('/admin/accounts/import/kiro', payload)
    expect(result).toEqual(response)
  })
})
