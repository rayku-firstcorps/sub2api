<template>
  <section class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ text.title }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ text.description }}</p>
    </div>
    <div class="space-y-5 p-6">
      <div class="flex items-start justify-between gap-4 border-b border-gray-100 pb-5 dark:border-dark-700">
        <div>
          <div class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ text.recording }}</div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ text.recordingHint }}</p>
        </div>
        <Toggle
          :model-value="recordingEnabled"
          :disabled="loadingRecording || savingRecording"
          :aria-label="text.recording"
          @update:model-value="saveRecording"
        />
      </div>

      <div class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ text.retentionDays }}
          </label>
          <input v-model.number="retentionDays" type="number" min="1" max="365" class="input w-32" />
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ text.retentionHint }}</p>
        </div>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="loading || starting || active"
          @click="showConfirm = true"
        >
          <Icon name="database" size="sm" />
          {{ starting ? text.starting : active ? text.running : text.start }}
        </button>
      </div>

      <div class="rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-wrap items-center justify-between gap-2 text-sm">
          <span class="font-medium text-gray-700 dark:text-gray-200">{{ text.status }}</span>
          <span :class="statusClass">{{ statusText }}</span>
        </div>
        <div v-if="task" class="mt-2 flex flex-wrap gap-x-6 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
          <span>{{ text.processed }}: {{ task.deleted_rows.toLocaleString() }}</span>
          <span v-if="task.finished_at">{{ text.finished }}: {{ formatTime(task.finished_at) }}</span>
        </div>
        <p v-if="task?.error_message" class="mt-2 text-xs text-red-600 dark:text-red-400">
          {{ task.error_message }}
        </p>
      </div>

      <p class="flex items-start gap-2 text-xs text-amber-700 dark:text-amber-300">
        <Icon name="exclamationTriangle" size="sm" class="mt-0.5 flex-shrink-0" />
        {{ text.safety }}
      </p>
    </div>
  </section>

  <ConfirmDialog
    :show="showConfirm"
    :title="text.confirmTitle"
    :message="text.confirmMessage.replace('{days}', String(retentionDays))"
    :confirm-text="text.start"
    danger
    @confirm="startCleanup"
    @cancel="showConfirm = false"
  />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import type { UsageCleanupTask } from '@/api/admin/usage'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const { locale } = useI18n()
const appStore = useAppStore()
const retentionDays = ref(7)
const loading = ref(true)
const starting = ref(false)
const showConfirm = ref(false)
const task = ref<UsageCleanupTask | null>(null)
const recordingEnabled = ref(true)
const loadingRecording = ref(true)
const savingRecording = ref(false)
let pollTimer: ReturnType<typeof setTimeout> | undefined

const zh = computed(() => locale.value.startsWith('zh'))
const text = computed(() => zh.value ? {
  title: '请求上下文清理', description: '清空历史用量记录中的请求上下文，保留计费、统计和审计数据。',
  recording: '记录请求上下文', recordingHint: '关闭后停止记录新的请求上下文，不影响用量和计费；历史上下文不会自动删除。',
  retentionDays: '保留最近天数', retentionHint: '仅处理此天数之前的记录，可设置 1-365 天。',
  start: '启动清理', starting: '正在启动...', running: '清理进行中', status: '最近任务',
  processed: '已清理记录', finished: '完成时间', idle: '尚未运行', pending: '等待执行',
  succeeded: '已完成', failed: '失败', canceled: '已取消', confirmTitle: '确认清理请求上下文',
  confirmMessage: '将清空 {days} 天以前记录中的请求上下文。该操作不可恢复，但不会删除用量和计费记录。',
  safety: '任务采用 500 行以内的小批次、短超时和批次间限速；同一时间只允许一个清理任务，不会自动执行 VACUUM FULL。',
  started: '清理任务已进入队列。', invalid: '保留天数必须在 1 到 365 之间。',
  recordingSaved: '请求上下文记录设置已更新。'
} : {
  title: 'Request context cleanup', description: 'Clear request context from historical usage rows while preserving billing, statistics, and audit data.',
  recording: 'Record request context', recordingHint: 'When disabled, new request context is not stored. Usage and billing are unaffected, and historical context is not deleted.',
  retentionDays: 'Keep recent days', retentionHint: 'Only rows older than this value are processed (1-365 days).',
  start: 'Start cleanup', starting: 'Starting...', running: 'Cleanup in progress', status: 'Latest task',
  processed: 'Contexts cleared', finished: 'Finished', idle: 'Not run yet', pending: 'Pending',
  succeeded: 'Succeeded', failed: 'Failed', canceled: 'Canceled', confirmTitle: 'Confirm request context cleanup',
  confirmMessage: 'Request context older than {days} days will be cleared. This cannot be undone, but usage and billing rows are preserved.',
  safety: 'The task uses batches of at most 500 rows, short timeouts, and throttling. Only one task can run at a time and VACUUM FULL is never started automatically.',
  started: 'Cleanup task queued.', invalid: 'Retention must be between 1 and 365 days.',
  recordingSaved: 'Request context recording setting updated.'
})

const active = computed(() => task.value?.status === 'pending' || task.value?.status === 'running')
const statusText = computed(() => {
  if (!task.value) return text.value.idle
  return text.value[task.value.status as 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled'] || task.value.status
})
const statusClass = computed(() => active.value ? 'text-amber-600 dark:text-amber-300' : task.value?.status === 'failed' ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400')

function schedulePoll(): void {
  if (pollTimer) clearTimeout(pollTimer)
  if (active.value) pollTimer = setTimeout(loadStatus, 3000)
}

async function loadStatus(): Promise<void> {
  try {
    task.value = await adminAPI.usage.getRequestContextCleanupTask()
  } catch (error) {
    if (!task.value) appStore.showError(extractApiErrorMessage(error, 'Failed to load cleanup status'))
  } finally {
    loading.value = false
    schedulePoll()
  }
}

async function loadRecording(): Promise<void> {
  try {
    const settings = await adminAPI.settings.getSettings()
    recordingEnabled.value = settings.usage_log_request_context_enabled !== false
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, 'Failed to load request context recording setting'))
  } finally {
    loadingRecording.value = false
  }
}

async function saveRecording(enabled: boolean): Promise<void> {
  const previous = recordingEnabled.value
  recordingEnabled.value = enabled
  savingRecording.value = true
  try {
    const settings = await adminAPI.settings.updateSettings({ usage_log_request_context_enabled: enabled })
    recordingEnabled.value = settings.usage_log_request_context_enabled !== false
    appStore.showSuccess(text.value.recordingSaved)
  } catch (error) {
    recordingEnabled.value = previous
    appStore.showError(extractApiErrorMessage(error, 'Failed to update request context recording setting'))
  } finally {
    savingRecording.value = false
  }
}

async function startCleanup(): Promise<void> {
  showConfirm.value = false
  if (!Number.isInteger(retentionDays.value) || retentionDays.value < 1 || retentionDays.value > 365) {
    appStore.showError(text.value.invalid)
    return
  }
  starting.value = true
  try {
    task.value = await adminAPI.usage.createRequestContextCleanupTask(retentionDays.value)
    appStore.showSuccess(text.value.started)
    schedulePoll()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, 'Failed to start cleanup'))
    await loadStatus()
  } finally {
    starting.value = false
  }
}

function formatTime(value: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

onMounted(() => {
  void loadStatus()
  void loadRecording()
})
onBeforeUnmount(() => { if (pollTimer) clearTimeout(pollTimer) })
</script>
