<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { opsAPI, type FeishuNotificationConfig, type FeishuNotificationConfigUpdate } from '@/api/admin/ops'

const { t } = useI18n()
const config = ref<FeishuNotificationConfig | null>(null)
const saved = ref('')
const webhook = ref('')
const secret = ref('')
const clearCredentials = ref(false)
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const error = ref('')
const success = ref('')
const dirty = computed(() => JSON.stringify(config.value) !== saved.value || !!webhook.value || !!secret.value || clearCredentials.value)
const busy = computed(() => loading.value || saving.value || testing.value)
const canTest = computed(() => !busy.value && !dirty.value && config.value?.webhook_configured && config.value?.secret_configured)

function errorMessage(err: unknown, fallback: string): string {
  // Never log the Axios error: its request config can contain the credentials.
  const failure = err as { message?: string; response?: { data?: { message?: string; detail?: string } } }
  const response = failure?.response?.data
  return response?.message || response?.detail || (!(err instanceof Error) && failure?.message) || t(fallback)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    config.value = await opsAPI.getFeishuNotificationConfig()
    saved.value = JSON.stringify(config.value)
    webhook.value = ''
    secret.value = ''
    clearCredentials.value = false
  } catch (err) {
    error.value = errorMessage(err, 'admin.ops.feishu.loadFailed')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!config.value || busy.value) return
  error.value = ''
  success.value = ''
  const url = webhook.value.trim()
  const key = secret.value.trim()
  if (!clearCredentials.value) {
    if (url && !/^https:\/\/open\.feishu\.cn\/open-apis\/bot\/v2\/hook\/[A-Za-z0-9_-]{16,128}$/.test(url)) {
      error.value = t('admin.ops.feishu.invalidWebhook')
      return
    }
    if ((url && !key) || (config.value.enabled && (!config.value.webhook_configured && !url || !config.value.secret_configured && !key)) || (key && !url && !config.value.webhook_configured)) {
      error.value = t('admin.ops.feishu.credentialsRequired')
      return
    }
  }
  const limit = config.value.rate_limit_per_hour
  if (!Number.isInteger(limit) || limit < 1 || limit > 3600) {
    error.value = t('admin.ops.feishu.invalidLimit')
    return
  }
  const payload: FeishuNotificationConfigUpdate = {
    enabled: clearCredentials.value ? false : config.value.enabled,
    min_severity: config.value.min_severity,
    rate_limit_per_hour: limit,
    include_resolved_alerts: config.value.include_resolved_alerts
  }
  if (clearCredentials.value) {
    payload.clear_credentials = true
  } else {
    if (url) payload.webhook_url = url
    if (key) payload.signing_secret = key
  }
  saving.value = true
  try {
    config.value = await opsAPI.updateFeishuNotificationConfig(payload)
    saved.value = JSON.stringify(config.value)
    webhook.value = ''
    secret.value = ''
    clearCredentials.value = false
    success.value = t('admin.ops.feishu.saved')
  } catch (err) {
    error.value = errorMessage(err, 'admin.ops.feishu.saveFailed')
  } finally {
    saving.value = false
  }
}

async function test() {
  if (!canTest.value) return
  testing.value = true
  error.value = ''
  success.value = ''
  try {
    await opsAPI.testFeishuNotification()
    success.value = t('admin.ops.feishu.testSucceeded')
  } catch (err) {
    error.value = errorMessage(err, 'admin.ops.feishu.testFailed')
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50" aria-labelledby="ops-feishu-title">
    <h4 id="ops-feishu-title" class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.feishu.title') }}</h4>
    <p class="mt-1 text-xs text-gray-600 dark:text-gray-300">{{ t('admin.ops.feishu.description') }}</p>
    <p v-if="loading" class="mt-4 text-sm" role="status">{{ t('common.loading') }}</p>
    <form v-else-if="config" class="mt-4" @submit.prevent="save">
      <fieldset :disabled="busy" class="space-y-4">
        <label class="flex items-center justify-between gap-3 text-sm font-medium">
          <span>{{ t('admin.ops.feishu.enabled') }}</span>
          <input v-model="config.enabled" data-testid="feishu-enabled" type="checkbox" :disabled="clearCredentials" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
        </label>
        <div v-if="config.webhook_configured" class="text-xs text-gray-600 dark:text-gray-300">{{ t('admin.ops.feishu.configured') }}</div>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label for="ops-feishu-webhook" class="input-label">{{ t('admin.ops.feishu.webhook') }}</label>
            <input id="ops-feishu-webhook" v-model="webhook" type="password" class="input" autocomplete="new-password" :disabled="clearCredentials" :placeholder="config.webhook_configured ? t('admin.ops.feishu.keepExisting') : 'https://open.feishu.cn/open-apis/bot/v2/hook/…'" aria-describedby="ops-feishu-signing-hint" />
          </div>
          <div>
            <label for="ops-feishu-secret" class="input-label">{{ t('admin.ops.feishu.secret') }}</label>
            <input id="ops-feishu-secret" v-model="secret" type="password" class="input" maxlength="512" autocomplete="new-password" :disabled="clearCredentials" :placeholder="config.secret_configured ? t('admin.ops.feishu.keepExisting') : t('admin.ops.feishu.secretPlaceholder')" aria-describedby="ops-feishu-signing-hint" />
          </div>
        </div>
        <p id="ops-feishu-signing-hint" class="text-xs text-gray-600 dark:text-gray-300">{{ t('admin.ops.feishu.signingHint') }}</p>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label for="ops-feishu-severity" class="input-label">{{ t('admin.ops.feishu.minSeverity') }}</label>
            <select id="ops-feishu-severity" v-model="config.min_severity" class="input">
              <option value="">{{ t('admin.ops.email.minSeverityAll') }}</option>
              <option value="info">{{ t('common.info') }}</option>
              <option value="warning">{{ t('common.warning') }}</option>
              <option value="critical">{{ t('common.critical') }}</option>
            </select>
          </div>
          <div>
            <label for="ops-feishu-limit" class="input-label">{{ t('admin.ops.feishu.rateLimit') }}</label>
            <input id="ops-feishu-limit" v-model.number="config.rate_limit_per_hour" type="number" min="1" max="3600" step="1" class="input" required />
          </div>
        </div>
        <label class="flex items-center gap-2 text-sm">
          <input v-model="config.include_resolved_alerts" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          {{ t('admin.ops.feishu.includeResolved') }}
        </label>
        <label v-if="config.webhook_configured" class="flex items-center gap-2 text-xs text-red-700 dark:text-red-300">
          <input v-model="clearCredentials" data-testid="feishu-clear" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
          {{ t('admin.ops.feishu.clearCredentials') }}
        </label>
        <div class="flex flex-wrap items-center gap-2">
          <button class="btn btn-secondary" type="submit" data-testid="feishu-save" :disabled="busy || !dirty">{{ saving ? t('common.saving') : t('admin.ops.feishu.save') }}</button>
          <button class="btn btn-secondary" type="button" data-testid="feishu-test" :disabled="!canTest" @click="test">{{ testing ? t('admin.ops.feishu.testing') : t('admin.ops.feishu.test') }}</button>
        </div>
        <p class="text-xs text-gray-600 dark:text-gray-300">{{ t('admin.ops.feishu.saveHint') }}</p>
      </fieldset>
    </form>
    <p v-if="error" class="mt-3 break-words text-sm text-red-700 dark:text-red-300" role="alert">{{ error }}</p>
    <button v-if="!config && !loading" type="button" class="btn btn-secondary mt-3" @click="load">{{ t('admin.ops.feishu.retry') }}</button>
    <p v-if="success" class="mt-3 text-sm text-green-700 dark:text-green-300" role="status">{{ success }}</p>
  </section>
</template>
