<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-16">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <template v-else>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.description') }}</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="statusLoading" @click="loadStatus(false)">
              <Icon name="refresh" size="sm" :class="statusLoading ? 'animate-spin' : ''" />
              {{ t('admin.riskControl.refreshStatus') }}
            </button>
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" @click="openKeywordSettings">
              <Icon name="filter" size="sm" />
              {{ t('admin.riskControl.keyword.openSettings') }}
            </button>
            <button type="button" class="btn btn-primary inline-flex items-center gap-2" @click="openSettings">
              <Icon name="cog" size="sm" />
              {{ t('admin.riskControl.openSettings') }}
            </button>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
          <div
            v-for="item in overviewItems"
            :key="item.key"
            class="rounded-lg border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="flex min-w-0 items-center gap-3">
              <div class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg" :class="item.iconClass">
                <Icon :name="item.icon" size="sm" />
              </div>
              <div class="min-w-0 flex-1">
                <div class="flex min-w-0 items-center justify-between gap-2">
                  <p class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">{{ item.label }}</p>
                  <span
                    v-if="item.badge"
                    class="inline-flex flex-shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-medium"
                    :class="item.badgeClass"
                  >
                    {{ item.badge }}
                  </span>
                </div>
                <div class="mt-1 flex min-w-0 items-baseline gap-2">
                  <p class="truncate text-xl font-semibold leading-7 text-gray-900 dark:text-white">{{ item.value }}</p>
                  <p v-if="item.meta" class="truncate text-xs text-gray-500 dark:text-gray-400">{{ item.meta }}</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.workerStatus') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.workerStatusHint') }}</p>
            </div>
            <div class="flex flex-wrap items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
              <span>{{ t('admin.riskControl.autoRefresh') }}</span>
              <span v-if="status?.last_cleanup_at">
                {{ t('admin.riskControl.lastCleanup', { time: formatDateTime(status.last_cleanup_at) }) }}
              </span>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-6 p-6 xl:grid-cols-[minmax(0,360px)_1fr]">
            <div class="space-y-4">
              <div class="rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div class="flex items-center justify-between gap-3">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.queueUsage') }}</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                      {{ formatNumber(status?.queue_length ?? 0) }} / {{ formatNumber(status?.queue_size ?? configForm.queue_size) }}
                    </p>
                  </div>
                  <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ queueUsagePercent }}</span>
                </div>
                <div class="mt-4 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                  <div class="h-full rounded-full bg-primary-500 transition-all duration-300" :style="queueUsageStyle"></div>
                </div>
              </div>

              <div class="grid grid-cols-2 gap-3">
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.activeWorkers') }}</p>
                  <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ status?.active_workers ?? 0 }}</p>
                </div>
                <div class="rounded-lg bg-emerald-50 p-4 dark:bg-emerald-900/10">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.idleWorkers') }}</p>
                  <p class="mt-2 text-2xl font-semibold text-emerald-700 dark:text-emerald-300">{{ status?.idle_workers ?? configForm.worker_count }}</p>
                </div>
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.processed') }}</p>
                  <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatNumber(status?.processed ?? 0) }}</p>
                </div>
                <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                  <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.droppedErrors') }}</p>
                  <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ formatNumber((status?.dropped ?? 0) + (status?.errors ?? 0)) }}</p>
                </div>
              </div>
            </div>

            <div>
              <div class="mb-3 flex items-center justify-between gap-3">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.workerPool') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.riskControl.workerPoolMeta', { active: status?.active_workers ?? 0, idle: status?.idle_workers ?? configForm.worker_count, total: status?.worker_count ?? configForm.worker_count }) }}
                  </p>
                </div>
                <span class="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  {{ modeLabel(status?.mode ?? configForm.mode) }}
                </span>
              </div>
              <div class="grid grid-cols-2 gap-2 sm:grid-cols-4 md:grid-cols-6 xl:grid-cols-8 2xl:grid-cols-10">
                <div
                  v-for="worker in workerSlots"
                  :key="worker.id"
                  class="flex h-12 items-center justify-between rounded-lg border px-3 transition-colors"
                  :class="workerSlotClass(worker.state)"
                  :title="worker.label"
                >
                  <span class="text-sm font-semibold">#{{ worker.id }}</span>
                  <span class="h-2.5 w-2.5 rounded-full" :class="workerDotClass(worker.state)"></span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ activeRecordTab === 'keyword' ? t('admin.riskControl.keyword.records') : t('admin.riskControl.records') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ activeRecordTab === 'keyword' ? t('admin.riskControl.keyword.recordsHint') : t('admin.riskControl.recordsHint') }}</p>
              </div>
              <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="logsLoading" @click="loadLogs">
                <Icon name="refresh" size="sm" :class="logsLoading ? 'animate-spin' : ''" />
                {{ t('admin.riskControl.refresh') }}
              </button>
            </div>

            <div class="inline-flex w-fit rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
              <button
                type="button"
                class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                :class="activeRecordTab === 'keyword' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
                @click="switchRecordTab('keyword')"
              >
                {{ t('admin.riskControl.keyword.records') }}
              </button>
              <button
                type="button"
                class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                :class="activeRecordTab === 'moderation' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
                @click="switchRecordTab('moderation')"
              >
                {{ t('admin.riskControl.records') }}
              </button>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-6">
              <Select v-model="filters.result" :options="resultOptions" @change="reloadLogsFromFirstPage" />
              <Select v-model="filters.group_id" :options="groupFilterOptions" @change="reloadLogsFromFirstPage" />
              <Select v-model="filters.endpoint" :options="endpointOptions" @change="reloadLogsFromFirstPage" />
              <input v-model.trim="filters.search" type="search" class="input" :placeholder="t('admin.riskControl.filters.search')" @keyup.enter="reloadLogsFromFirstPage" />
              <input v-model="filters.from" type="datetime-local" class="input" :title="t('admin.riskControl.filters.from')" @change="reloadLogsFromFirstPage" />
              <input v-model="filters.to" type="datetime-local" class="input" :title="t('admin.riskControl.filters.to')" @change="reloadLogsFromFirstPage" />
            </div>
          </div>

          <div v-if="activeRecordTab === 'keyword'" class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.time') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.group') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.user') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.endpoint') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.keyword.match') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.input') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
                <tr v-if="logsLoading">
                  <td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</td>
                </tr>
                <tr v-else-if="keywordLogs.length === 0">
                  <td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.keyword.emptyLogs') }}</td>
                </tr>
                <template v-else>
                  <tr v-for="row in keywordLogs" :key="row.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.group_name || '-' }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.user_email || '-' }}</div>
                      <div v-if="row.user_id" class="text-xs text-gray-400">UID {{ row.user_id }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.endpoint || '-' }}</div>
                      <div class="text-xs text-gray-400">{{ row.provider || '-' }} / {{ row.model || '-' }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="row.match_type === 'regex' ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300' : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'">
                        {{ row.match_type === 'regex' ? t('admin.riskControl.keyword.regex') : t('admin.riskControl.keyword.keyword') }}
                      </span>
                      <div class="mt-1 text-xs text-gray-400">{{ row.rule_name || '-' }} / {{ row.matched_text || '-' }}</div>
                    </td>
                    <td class="w-[360px] max-w-sm px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <span class="block truncate" :title="row.input_excerpt || '-'">{{ row.input_excerpt || '-' }}</span>
                      <span class="mt-1 block truncate font-mono text-xs text-gray-400">{{ row.input_hash || '-' }}</span>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>

          <div v-else class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.time') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.group') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.user') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.apiKey') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.endpoint') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.result') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.highest') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.actionMeta') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.latency') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.input') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
                <tr v-if="logsLoading">
                  <td colspan="10" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</td>
                </tr>
                <tr v-else-if="logs.length === 0">
                  <td colspan="10" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.emptyLogs') }}</td>
                </tr>
                <template v-else>
                  <tr v-for="row in logs" :key="row.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.group_name || '-' }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.user_email || '-' }}</div>
                      <div v-if="row.user_id" class="text-xs text-gray-400">UID {{ row.user_id }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.api_key_name || '-' }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.endpoint || '-' }}</div>
                      <div class="text-xs text-gray-400">{{ row.provider || '-' }} / {{ row.model || '-' }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4">
                      <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="resultBadgeClass(row)">
                        {{ resultLabel(row) }}
                      </span>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.highest_category || '-' }}</div>
                      <div class="text-xs text-gray-400">{{ percent(row.highest_score) }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ violationCountText(row) }}</div>
                      <div class="text-xs text-gray-400">
                        {{ row.email_sent ? t('admin.riskControl.emailSent') : t('admin.riskControl.emailNotSent') }}
                        <span v-if="row.auto_banned"> / {{ t('admin.riskControl.autoBanned') }}</span>
                      </div>
                      <button
                        v-if="canUnbanRow(row)"
                        type="button"
                        class="mt-2 inline-flex items-center gap-1 rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 transition-colors hover:bg-emerald-100 disabled:cursor-not-allowed disabled:opacity-60 dark:border-emerald-900/60 dark:bg-emerald-900/20 dark:text-emerald-300 dark:hover:bg-emerald-900/30"
                        :disabled="unbanningUserID === row.user_id"
                        @click="unbanUser(row)"
                      >
                        <Icon name="checkCircle" size="xs" :class="unbanningUserID === row.user_id ? 'animate-spin' : ''" />
                        {{ unbanningUserID === row.user_id ? t('common.processing') : t('admin.riskControl.unbanUser') }}
                      </button>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ latencyText(row.upstream_latency_ms) }}</div>
                      <div v-if="row.queue_delay_ms !== null && row.queue_delay_ms !== undefined" class="text-xs text-gray-400">
                        {{ t('admin.riskControl.queueDelay', { ms: row.queue_delay_ms }) }}
                      </div>
                    </td>
                    <td class="w-[320px] max-w-sm px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <button
                        type="button"
                        class="group flex w-full min-w-0 items-center gap-2 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                        :title="inputSummaryText(row)"
                        @click="openInputDetail(row)"
                      >
                        <span class="min-w-0 flex-1 truncate">{{ inputSummaryText(row) }}</span>
                        <Icon name="eye" size="xs" class="flex-shrink-0 text-gray-300 transition-colors group-hover:text-primary-500 dark:text-gray-500" />
                      </button>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>

          <Pagination
            v-if="pagination.total > 0"
            :page="pagination.page"
            :total="pagination.total"
            :page-size="pagination.page_size"
            @update:page="onPageChange"
            @update:pageSize="onPageSizeChange"
          />
        </div>
      </template>

      <BaseDialog :show="settingsOpen" :title="t('admin.riskControl.settingsTitle')" width="extra-wide" @close="settingsOpen = false">
        <div class="space-y-6">
          <div class="flex gap-2 overflow-x-auto border-b border-gray-100 pb-3 dark:border-dark-700">
            <button
              v-for="tab in settingsTabs"
              :key="tab.id"
              type="button"
              class="inline-flex whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium transition-colors"
              :class="activeSettingsTab === tab.id ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white'"
              @click="activeSettingsTab = tab.id"
            >
              {{ tab.label }}
            </button>
          </div>

          <div v-if="activeSettingsTab === 'basic'" class="space-y-5">
            <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.enabled') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.enabledHint') }}</p>
                </div>
                <Toggle v-model="configForm.enabled" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.mode') }}</label>
                <Select v-model="configForm.mode" :options="modeOptions" />
                <p class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ modeDescription(configForm.mode) }}</p>
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.baseUrl') }}</label>
                <input v-model.trim="configForm.base_url" type="url" class="input" placeholder="https://api.openai.com" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.model') }}</label>
                <input v-model.trim="configForm.model" type="text" class="input" placeholder="omni-moderation-latest" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.timeoutMs') }}</label>
                <input v-model.number="configForm.timeout_ms" type="number" min="500" max="30000" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.retryCount') }}</label>
                <input v-model.number="configForm.retry_count" type="number" min="0" max="5" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.sampleRate') }}</label>
                <div class="relative">
                  <input v-model.number="configForm.sample_rate" type="number" min="0" max="100" step="1" class="input pr-8" />
                  <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">%</span>
                </div>
              </div>
            </div>

            <div class="overflow-hidden rounded-xl border border-gray-100 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
              <div class="flex flex-col gap-4 border-b border-gray-100 bg-gray-50 px-4 py-4 dark:border-dark-700 dark:bg-dark-800/60 lg:flex-row lg:items-center lg:justify-between">
                <div class="flex items-start gap-3">
                  <span class="mt-0.5 flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
                    <Icon name="key" size="md" />
                  </span>
                  <div>
                    <label class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.apiKeys') }}</label>
                    <p class="mt-1 max-w-3xl text-xs leading-5 text-gray-500 dark:text-gray-400">
                      {{ t('admin.riskControl.apiKeysHint', { count: configForm.api_key_count }) }}
                    </p>
                  </div>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center gap-2"
                    :disabled="apiKeyTesting || inputApiKeyCount === 0 || configForm.clear_api_key"
                    @click="testApiKeys(true)"
                  >
                    <Icon name="beaker" size="sm" :class="apiKeyTesting ? 'animate-pulse' : ''" />
                    {{ apiKeyTesting ? t('admin.riskControl.testingApiKeys') : t('admin.riskControl.testInputApiKeys') }}
                  </button>
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center gap-2"
                    :disabled="apiKeyTesting || effectiveStoredApiKeyCount === 0 || pendingDeletedApiKeyCount > 0 || configForm.clear_api_key || configForm.api_keys_mode === 'replace'"
                    @click="testApiKeys(false)"
                  >
                    <Icon name="shield" size="sm" />
                    {{ storedApiKeyTestButtonText }}
                  </button>
                  <button
                    v-if="configForm.api_key_configured"
                    type="button"
                    class="btn btn-secondary inline-flex items-center gap-2"
                    @click="toggleClearApiKey"
                  >
                    <Icon :name="configForm.clear_api_key ? 'x' : 'trash'" size="sm" />
                    {{ configForm.clear_api_key ? t('admin.riskControl.keepApiKey') : t('admin.riskControl.clearApiKey') }}
                  </button>
                </div>
              </div>

              <div class="grid grid-cols-1 gap-4 p-4 xl:grid-cols-[minmax(0,1fr)_minmax(360px,440px)]">
                <div class="space-y-3">
                  <div class="flex flex-col gap-2 rounded-lg border border-gray-100 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-900/30 sm:flex-row sm:items-center sm:justify-between">
                    <div class="text-xs leading-5 text-gray-500 dark:text-gray-400">
                      <span class="font-medium text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.apiKeysWriteMode') }}</span>
                      <span class="ml-2">{{ apiKeysModeHint }}</span>
                    </div>
                    <div class="inline-flex rounded-lg bg-white p-1 shadow-sm dark:bg-dark-800">
                      <button
                        type="button"
                        class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
                        :class="configForm.api_keys_mode === 'append' ? 'bg-primary-500 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
                        :disabled="configForm.clear_api_key"
                        @click="setAPIKeysMode('append')"
                      >
                        {{ t('admin.riskControl.apiKeysModeAppend') }}
                      </button>
                      <button
                        type="button"
                        class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
                        :class="configForm.api_keys_mode === 'replace' ? 'bg-amber-500 text-white shadow-sm' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'"
                        :disabled="configForm.clear_api_key"
                        @click="setAPIKeysMode('replace')"
                      >
                        {{ t('admin.riskControl.apiKeysModeReplace') }}
                      </button>
                    </div>
                  </div>
                  <textarea
                    v-model="configForm.api_keys_text"
                    class="input min-h-44 resize-y font-mono text-sm"
                    :placeholder="apiKeysPlaceholder"
                    autocomplete="new-password"
                    :disabled="configForm.clear_api_key"
                  ></textarea>
                  <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span class="inline-flex rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-700">
                      {{ t('admin.riskControl.inputApiKeyCount', { count: inputApiKeyCount }) }}
                    </span>
                    <span v-if="configForm.api_key_configured" class="inline-flex rounded-md bg-gray-100 px-2 py-1 dark:bg-dark-700">
                      {{ t('admin.riskControl.storedApiKeyCount', { count: configForm.api_key_count }) }}
                    </span>
                    <span v-if="configForm.clear_api_key" class="inline-flex rounded-md bg-red-50 px-2 py-1 text-red-700 dark:bg-red-900/20 dark:text-red-300">
                      {{ t('admin.riskControl.apiKeyWillClear') }}
                    </span>
                    <span v-else-if="pendingDeletedApiKeyCount > 0" class="inline-flex rounded-md bg-amber-50 px-2 py-1 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                      {{ t('admin.riskControl.apiKeyPendingDeleteCount', { count: pendingDeletedApiKeyCount }) }}
                    </span>
                    <span v-if="configForm.api_keys_mode === 'replace'" class="inline-flex rounded-md bg-amber-50 px-2 py-1 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                      {{ t('admin.riskControl.apiKeysReplaceWarning') }}
                    </span>
                  </div>

                  <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/30" @paste="handleModerationImagePaste">
                    <div class="mb-3 flex items-center justify-between gap-3">
                      <div>
                        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.auditTestInput') }}</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestInputHint') }}</p>
                      </div>
                      <button
                        v-if="moderationTestPrompt || moderationTestImages.length > 0 || moderationTestResult"
                        type="button"
                        class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-500 hover:bg-white hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white"
                        @click="clearModerationTestInput"
                      >
                        <Icon name="x" size="xs" />
                        {{ t('admin.riskControl.clearAuditTest') }}
                      </button>
                    </div>
                    <textarea
                      v-model="moderationTestPrompt"
                      class="input min-h-24 resize-y text-sm"
                      :placeholder="t('admin.riskControl.auditTestPromptPlaceholder')"
                    ></textarea>
                    <div
                      class="mt-3 rounded-lg border border-dashed border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800"
                      @dragover.prevent
                      @drop.prevent="handleModerationImageDrop"
                    >
                      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                        <div class="flex items-start gap-2">
                          <Icon name="upload" size="md" class="mt-0.5 text-gray-400" />
                          <div>
                            <p class="text-sm font-medium text-gray-800 dark:text-gray-100">{{ t('admin.riskControl.auditTestImages') }}</p>
                            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.auditTestImagesHint') }}</p>
                          </div>
                        </div>
                        <label class="btn btn-secondary inline-flex cursor-pointer items-center gap-2">
                          <Icon name="plus" size="sm" />
                          {{ t('admin.riskControl.addAuditTestImage') }}
                          <input type="file" accept="image/*" multiple class="sr-only" @change="handleModerationImageUpload" />
                        </label>
                      </div>
                      <div v-if="moderationTestImages.length > 0" class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
                        <div
                          v-for="(image, index) in moderationTestImages"
                          :key="image.slice(0, 64) + index"
                          class="group relative aspect-square overflow-hidden rounded-lg border border-gray-100 bg-gray-100 dark:border-dark-700 dark:bg-dark-700"
                        >
                          <img :src="image" alt="" class="h-full w-full object-cover" />
                          <button
                            type="button"
                            class="absolute right-1.5 top-1.5 flex h-7 w-7 items-center justify-center rounded-full bg-black/60 text-white opacity-0 transition-opacity group-hover:opacity-100"
                            @click="removeModerationTestImage(index)"
                          >
                            <Icon name="x" size="xs" :stroke-width="2" />
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <div class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/30">
                  <div class="mb-3 flex items-start justify-between gap-3">
                    <div class="min-w-0">
                      <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.apiKeyHealth') }}</p>
                      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.apiKeyFreezeRule') }}</p>
                    </div>
                    <span class="inline-flex shrink-0 items-center whitespace-nowrap rounded-full bg-white px-2 py-0.5 text-[11px] font-medium leading-5 text-gray-600 shadow-sm dark:bg-dark-800 dark:text-gray-300">
                      {{ t('admin.riskControl.apiKeyRows', { count: apiKeyRows.length }) }}
                    </span>
                  </div>

                  <div v-if="apiKeyRows.length === 0" class="flex min-h-32 flex-col items-center justify-center rounded-lg border border-dashed border-gray-200 bg-white px-4 py-6 text-center dark:border-dark-700 dark:bg-dark-800">
                    <Icon name="infoCircle" size="lg" class="text-gray-300 dark:text-dark-500" />
                    <p class="mt-2 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.apiKeyHealthEmpty') }}</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.apiKeyHealthEmptyHint') }}</p>
                  </div>
                  <div v-else class="space-y-2">
                    <div class="space-y-2" :class="apiKeyRowsExpanded ? 'max-h-72 overflow-y-auto pr-1' : ''">
                      <div
                        v-for="(row, index) in visibleApiKeyRows"
                        :key="apiKeyRowKey(row, index)"
                        class="rounded-lg border bg-white p-2.5 shadow-sm dark:bg-dark-800"
                        :class="isStoredApiKeyPendingDelete(row) ? 'border-amber-200 opacity-70 dark:border-amber-800/60' : 'border-gray-100 dark:border-dark-700'"
                      >
                        <div class="flex items-start justify-between gap-2">
                          <div class="min-w-0">
                            <div class="flex min-w-0 flex-wrap items-center gap-2">
                              <span class="truncate font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ row.masked || '-' }}</span>
                              <span
                                class="inline-flex rounded-md px-1.5 py-0.5 text-[11px] font-medium"
                                :class="row.configured ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-purple-50 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300'"
                              >
                                {{ isStoredApiKeyPendingDelete(row) ? t('admin.riskControl.apiKeyPendingDelete') : row.configured ? t('admin.riskControl.apiKeyConfigured') : t('admin.riskControl.apiKeyTemporary') }}
                              </span>
                            </div>
                            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ apiKeyStatusMeta(row) }}</p>
                          </div>
                          <div class="flex flex-shrink-0 items-center gap-1.5">
                            <span class="inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium" :class="apiKeyStatusBadgeClass(row.status)">
                              <span class="h-1.5 w-1.5 rounded-full" :class="apiKeyStatusDotClass(row.status)"></span>
                              {{ apiKeyStatusLabel(row.status) }}
                            </span>
                            <button
                              v-if="row.configured && !configForm.clear_api_key"
                              type="button"
                              class="inline-flex h-7 w-7 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
                              :title="isStoredApiKeyPendingDelete(row) ? t('admin.riskControl.undoDeleteApiKey') : t('admin.riskControl.deleteApiKey')"
                              @click="toggleDeleteStoredApiKey(row)"
                            >
                              <Icon :name="isStoredApiKeyPendingDelete(row) ? 'refresh' : 'trash'" size="xs" />
                            </button>
                          </div>
                        </div>
                        <p v-if="row.last_error" class="mt-1.5 rounded-md bg-amber-50 px-2 py-1.5 text-xs leading-5 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                          {{ row.last_error }}
                        </p>
                      </div>
                    </div>

                    <div v-if="canToggleApiKeyRows" class="flex items-center justify-between gap-3 rounded-lg border border-dashed border-gray-200 bg-white px-3 py-2 text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
                      <span class="min-w-0 truncate">
                        {{ apiKeyRowsExpanded ? t('admin.riskControl.apiKeyRowsExpanded', { count: apiKeyRows.length }) : t('admin.riskControl.apiKeyRowsCollapsed', { count: hiddenApiKeyRowCount }) }}
                      </span>
                      <button
                        type="button"
                        class="inline-flex shrink-0 items-center gap-1 rounded-md px-2 py-1 font-medium text-primary-600 transition-colors hover:bg-primary-50 hover:text-primary-700 dark:text-primary-300 dark:hover:bg-primary-900/20"
                        @click="apiKeyRowsExpanded = !apiKeyRowsExpanded"
                      >
                        <Icon :name="apiKeyRowsExpanded ? 'chevronUp' : 'chevronDown'" size="xs" />
                        {{ apiKeyRowsExpanded ? t('admin.riskControl.collapseApiKeyRows') : t('admin.riskControl.expandApiKeyRows') }}
                      </button>
                    </div>
                  </div>

                  <div v-if="moderationTestResult" class="mt-4 rounded-lg border border-gray-100 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
                    <div class="flex items-start justify-between gap-3">
                      <div>
                        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.auditTestResult') }}</p>
                        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                          {{ t('admin.riskControl.auditTestHighest', { category: moderationTestResult.highest_category || '-', score: percent(moderationTestResult.highest_score) }) }}
                        </p>
                      </div>
                      <span class="inline-flex rounded-full px-2 py-1 text-xs font-medium" :class="moderationTestResult.flagged ? 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300' : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'">
                        {{ moderationTestResult.flagged ? t('admin.riskControl.auditTestFlagged') : t('admin.riskControl.auditTestPassed') }}
                      </span>
                    </div>
                    <div class="mt-3">
                      <div class="mb-2 flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
                        <span>{{ t('admin.riskControl.auditTestComposite') }}</span>
                        <span class="font-semibold text-gray-900 dark:text-white">{{ percent(moderationTestResult.composite_score) }}</span>
                      </div>
                      <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                        <div class="h-full rounded-full" :class="moderationTestResult.flagged ? 'bg-red-500' : 'bg-emerald-500'" :style="{ width: percentWidth(moderationTestResult.composite_score) }"></div>
                      </div>
                    </div>
                    <div class="mt-3 max-h-52 space-y-2 overflow-y-auto pr-1">
                      <div v-for="score in moderationScoreRows" :key="score.category">
                        <div class="mb-1 flex items-center justify-between gap-3 text-xs">
                          <span class="truncate text-gray-600 dark:text-gray-300">{{ score.category }}</span>
                          <span class="font-mono text-gray-500 dark:text-gray-400">{{ percent(score.score) }} / {{ percent(score.threshold) }}</span>
                        </div>
                        <div class="h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                          <div class="h-full rounded-full" :class="score.hit ? 'bg-red-500' : 'bg-primary-500'" :style="{ width: percentWidth(score.score) }"></div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'scope'" class="space-y-5">
            <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.groupScope') }}</h3>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.groupScopeHint') }}</p>
              </div>
              <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
                <button
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                  :class="configForm.all_groups ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
                  @click="configForm.all_groups = true"
                >
                  {{ t('admin.riskControl.allGroups') }}
                </button>
                <button
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                  :class="!configForm.all_groups ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
                  @click="configForm.all_groups = false"
                >
                  {{ t('admin.riskControl.selectedGroups') }}
                </button>
              </div>
            </div>

            <div v-if="!configForm.all_groups" class="space-y-4">
              <div class="relative">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input v-model.trim="groupSearch" type="search" class="input pl-9" :placeholder="t('admin.riskControl.searchGroups')" />
              </div>
              <div class="grid max-h-[420px] grid-cols-1 gap-3 overflow-y-auto pr-1 md:grid-cols-2 xl:grid-cols-3">
                <button
                  v-for="group in filteredGroups"
                  :key="group.id"
                  type="button"
                  class="flex min-h-20 items-center justify-between rounded-lg border p-4 text-left transition-colors"
                  :class="isGroupSelected(group.id) ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20' : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                  @click="toggleGroup(group.id)"
                >
                  <span class="min-w-0">
                    <span class="block truncate text-sm font-semibold text-gray-900 dark:text-white">{{ group.name }}</span>
                    <span class="mt-1 inline-flex rounded-md bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ group.platform }}</span>
                  </span>
                  <span
                    class="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full border"
                    :class="isGroupSelected(group.id) ? 'border-primary-500 bg-primary-500 text-white' : 'border-gray-300 text-transparent dark:border-dark-500'"
                  >
                    <Icon name="check" size="xs" :stroke-width="2" />
                  </span>
                </button>
                <p v-if="filteredGroups.length === 0" class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.noGroups') }}</p>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'runtime'" class="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.riskControl.workerCount') }}</label>
              <input v-model.number="configForm.worker_count" type="number" min="1" max="32" class="input" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.queueSize') }}</label>
              <input v-model.number="configForm.queue_size" type="number" min="100" max="100000" class="input" />
            </div>
            <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
              <div>
                <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.recordNonHits') }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.recordNonHitsHint') }}</p>
              </div>
              <Toggle v-model="configForm.record_non_hits" />
            </div>
            <div class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700 lg:col-span-2">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.preHashCheck') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.preHashCheckHint') }}</p>
                </div>
                <Toggle v-model="configForm.pre_hash_check_enabled" />
              </div>
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-900/30">
                <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">
                      {{ t('admin.riskControl.flaggedHashCount', { count: formatNumber(status?.flagged_hash_count ?? 0) }) }}
                    </p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.flaggedHashHint') }}</p>
                  </div>
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center justify-center gap-2 text-red-600 hover:text-red-700 dark:text-red-300"
                    :disabled="hashActionLoading || (status?.flagged_hash_count ?? 0) === 0"
                    @click="clearFlaggedHashes"
                  >
                    <Icon name="trash" size="sm" :class="hashActionLoading ? 'animate-pulse' : ''" />
                    {{ t('admin.riskControl.clearFlaggedHashes') }}
                  </button>
                </div>
                <div class="mt-3 flex flex-col gap-2 sm:flex-row">
                  <input
                    v-model.trim="flaggedHashInput"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="t('admin.riskControl.flaggedHashPlaceholder')"
                  />
                  <button
                    type="button"
                    class="btn btn-secondary inline-flex items-center justify-center gap-2"
                    :disabled="hashActionLoading || !isFlaggedHashInputValid"
                    @click="deleteFlaggedHash"
                  >
                    <Icon name="trash" size="sm" />
                    {{ t('admin.riskControl.deleteFlaggedHash') }}
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div v-else-if="activeSettingsTab === 'response'" class="space-y-5">
            <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.riskControl.blockStatus') }}</label>
                <input v-model.number="configForm.block_status" type="number" min="400" max="599" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.blockMessage') }}</label>
                <input v-model.trim="configForm.block_message" type="text" class="input" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.emailOnHit') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.emailOnHitHint') }}</p>
                </div>
                <Toggle v-model="configForm.email_on_hit" />
              </div>
              <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.autoBan') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.autoBanHint') }}</p>
                </div>
                <Toggle v-model="configForm.auto_ban_enabled" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.banThreshold') }}</label>
                <input v-model.number="configForm.ban_threshold" type="number" min="1" max="1000" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.violationWindowHours') }}</label>
                <input v-model.number="configForm.violation_window_hours" type="number" min="1" max="8760" class="input" />
              </div>
            </div>
          </div>

          <div v-else class="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.riskControl.hitRetentionDays') }}</label>
              <input v-model.number="configForm.hit_retention_days" type="number" min="1" max="3650" class="input" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.nonHitRetentionDays') }}</label>
              <input v-model.number="configForm.non_hit_retention_days" type="number" min="1" max="3" class="input" />
            </div>
            <div class="rounded-lg border border-gray-100 p-4 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400 lg:col-span-2">
              <div class="flex flex-wrap items-center gap-3">
                <Icon name="database" size="md" class="text-gray-400" />
                <span>{{ t('admin.riskControl.cleanupStats', { hit: status?.last_cleanup_deleted_hit ?? 0, nonHit: status?.last_cleanup_deleted_non_hit ?? 0 }) }}</span>
              </div>
            </div>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end gap-2">
            <button type="button" class="btn btn-secondary" @click="settingsOpen = false">{{ t('common.cancel') }}</button>
            <button type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving" @click="saveConfig">
              <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
              <Icon v-else name="check" size="sm" />
              {{ saving ? t('common.saving') : t('admin.riskControl.saveConfig') }}
            </button>
          </div>
        </template>
      </BaseDialog>

      <BaseDialog :show="keywordSettingsOpen" :title="t('admin.riskControl.keyword.settingsTitle')" width="wide" @close="keywordSettingsOpen = false">
        <div class="space-y-5">
          <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.keyword.enabled') }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.keyword.enabledHint') }}</p>
            </div>
            <Toggle v-model="keywordForm.enabled" />
          </div>

          <div class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700">
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ kt('ruleEditor') }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ kt('ruleEditorHint') }}</p>
              </div>
              <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
                <button type="button" class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors" :class="keywordRuleViewMode === 'simple' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'" @click="switchKeywordRuleViewMode('simple')">
                  {{ kt('simpleRules') }}
                </button>
                <button type="button" class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors" :class="keywordRuleViewMode === 'advanced' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'" @click="switchKeywordRuleViewMode('advanced')">
                  {{ kt('advancedRules') }}
                </button>
              </div>
            </div>

            <div v-if="keywordRuleViewMode === 'simple'" class="grid grid-cols-1 gap-5 lg:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.riskControl.keyword.keywords') }}</label>
                <textarea v-model="keywordForm.keywords_text" class="input min-h-36 resize-y" :placeholder="t('admin.riskControl.keyword.keywordsPlaceholder')"></textarea>
              </div>
              <div>
                <label class="input-label">{{ t('admin.riskControl.keyword.whitelist') }}</label>
                <textarea v-model="keywordForm.whitelist_text" class="input min-h-36 resize-y" :placeholder="t('admin.riskControl.keyword.whitelistPlaceholder')"></textarea>
              </div>
            </div>

            <div v-else class="space-y-4">
              <div class="space-y-2">
                <div class="flex items-center justify-between gap-3">
                  <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ kt('keywordRules') }}</p>
                  <button type="button" class="btn btn-secondary inline-flex items-center gap-2" @click="addKeywordRule">
                    <Icon name="plus" size="sm" />
                    {{ kt('addKeywordRule') }}
                  </button>
                </div>
                <div class="space-y-2">
                  <div v-for="(rule, index) in keywordForm.keyword_rules" :key="rule.id || index" class="grid grid-cols-1 gap-2 rounded-lg bg-gray-50 p-3 dark:bg-dark-900/30 lg:grid-cols-[minmax(0,1fr)_180px_auto_auto] lg:items-center">
                    <input v-model.trim="rule.pattern" type="text" class="input" :placeholder="kt('rulePattern')" />
                    <Select :model-value="rule.match_mode" :options="keywordMatchModeOptions" @update:model-value="rule.match_mode = normalizeMatchMode(String($event || 'auto'))" />
                    <label class="inline-flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                      <Toggle v-model="rule.enabled" />
                      {{ t('admin.riskControl.keyword.enabledShort') }}
                    </label>
                    <button type="button" class="inline-flex h-9 w-9 items-center justify-center rounded-md text-gray-400 hover:bg-gray-100 hover:text-red-600 dark:hover:bg-dark-700" @click="removeKeywordRule(index)">
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                  <p v-if="keywordForm.keyword_rules.length === 0" class="text-sm text-gray-500 dark:text-gray-400">{{ kt('noKeywordRules') }}</p>
                </div>
              </div>

              <div class="space-y-2">
                <div class="flex items-center justify-between gap-3">
                  <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ kt('whitelistRules') }}</p>
                  <button type="button" class="btn btn-secondary inline-flex items-center gap-2" @click="addWhitelistRule">
                    <Icon name="plus" size="sm" />
                    {{ kt('addWhitelistRule') }}
                  </button>
                </div>
                <div class="space-y-2">
                  <div v-for="(rule, index) in keywordForm.whitelist_rules" :key="rule.id || index" class="space-y-3 rounded-lg bg-gray-50 p-3 dark:bg-dark-900/30">
                    <div class="grid grid-cols-1 gap-2 lg:grid-cols-[minmax(0,1fr)_180px_auto_auto] lg:items-center">
                      <input v-model.trim="rule.pattern" type="text" class="input" :placeholder="kt('rulePattern')" />
                      <Select :model-value="rule.match_mode" :options="keywordMatchModeOptions" @update:model-value="rule.match_mode = normalizeMatchMode(String($event || 'auto'))" />
                      <label class="inline-flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                        <Toggle v-model="rule.enabled" />
                        {{ t('admin.riskControl.keyword.enabledShort') }}
                      </label>
                      <button type="button" class="inline-flex h-9 w-9 items-center justify-center rounded-md text-gray-400 hover:bg-gray-100 hover:text-red-600 dark:hover:bg-dark-700" @click="removeWhitelistRule(index)">
                        <Icon name="trash" size="sm" />
                      </button>
                    </div>
                    <div v-if="keywordForm.keyword_rules.length > 0" class="flex flex-wrap gap-2">
                      <button
                        v-for="target in keywordForm.keyword_rules"
                        :key="`${rule.id}-${target.id}`"
                        type="button"
                        class="rounded-md border px-2 py-1 text-xs transition-colors"
                        :class="rule.target_rule_ids.includes(target.id) ? 'border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'border-gray-200 text-gray-500 dark:border-dark-600 dark:text-gray-400'"
                        @click="toggleWhitelistTarget(rule, target.id)"
                      >
                        {{ target.pattern || target.id }}
                      </button>
                      <span class="text-xs text-gray-400">{{ rule.target_rule_ids.length === 0 ? kt('whitelistTargetsAll') : kt('whitelistTargetsSome') }}</span>
                    </div>
                  </div>
                  <p v-if="keywordForm.whitelist_rules.length === 0" class="text-sm text-gray-500 dark:text-gray-400">{{ kt('noWhitelistRules') }}</p>
                </div>
              </div>
            </div>
          </div>

          <div class="rounded-lg border border-gray-100 p-4 dark:border-dark-700">
            <div class="mb-3 flex items-center justify-between gap-3">
              <div>
                <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.keyword.regexRules') }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.keyword.regexRulesHint') }}</p>
              </div>
              <button type="button" class="btn btn-secondary inline-flex items-center gap-2" @click="addKeywordRegexRule">
                <Icon name="plus" size="sm" />
                {{ t('admin.riskControl.keyword.addRegex') }}
              </button>
            </div>
            <div class="space-y-2">
              <div v-for="(rule, index) in keywordForm.regex_rules" :key="`${rule.name}-${index}`" class="grid grid-cols-1 gap-2 rounded-lg bg-gray-50 p-3 dark:bg-dark-900/30 lg:grid-cols-[160px_minmax(0,1fr)_auto_auto] lg:items-center">
                <input v-model.trim="rule.name" type="text" class="input" :placeholder="t('admin.riskControl.keyword.regexName')" />
                <input v-model.trim="rule.pattern" type="text" class="input font-mono text-sm" :placeholder="t('admin.riskControl.keyword.regexPattern')" />
                <label class="inline-flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                  <Toggle v-model="rule.enabled" />
                  {{ t('admin.riskControl.keyword.enabledShort') }}
                </label>
                <button type="button" class="inline-flex h-9 w-9 items-center justify-center rounded-md text-gray-400 hover:bg-gray-100 hover:text-red-600 dark:hover:bg-dark-700" @click="removeKeywordRegexRule(index)">
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>
          </div>

          <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900/30">
            <label class="input-label">{{ t('admin.riskControl.keyword.testInput') }}</label>
            <textarea v-model="keywordTestText" class="input min-h-24 resize-y" :placeholder="t('admin.riskControl.keyword.testPlaceholder')"></textarea>
            <div class="mt-3 flex flex-wrap items-center gap-2">
              <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="keywordTesting" @click="testKeywordFilter">
                <Icon name="beaker" size="sm" :class="keywordTesting ? 'animate-pulse' : ''" />
                {{ t('admin.riskControl.keyword.test') }}
              </button>
              <span v-if="keywordTestResult" class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="keywordTestResult.blocked ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300' : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'">
                {{ keywordTestResult.blocked ? t('admin.riskControl.keyword.testBlocked') : t('admin.riskControl.keyword.testPassed') }}
              </span>
              <span v-if="keywordTestResult?.blocked" class="text-sm text-gray-500 dark:text-gray-400">
                {{ keywordTestResult.match_type }} / {{ keywordTestResult.rule_name }} / {{ matchModeLabel(keywordTestResult.resolved_match_mode) }} / {{ keywordTestResult.matched_text }}
              </span>
            </div>
            <div v-if="keywordTestResult?.blocked" class="mt-3 grid grid-cols-1 gap-2 text-xs text-gray-500 dark:text-gray-400 sm:grid-cols-2">
              <div>{{ kt('testRule') }}: {{ keywordTestResult.rule_id || keywordTestResult.rule_name || '-' }}</div>
              <div>{{ kt('testSegment') }}: #{{ keywordTestResult.segment_index ?? 0 }}</div>
              <div class="sm:col-span-2">{{ kt('testExcerpt') }}: {{ keywordTestResult.segment_text || keywordTestResult.matched_text || '-' }}</div>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.riskControl.blockStatus') }}</label>
              <input v-model.number="keywordForm.block_status" type="number" min="400" max="599" class="input" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.hitRetentionDays') }}</label>
              <input v-model.number="keywordForm.hit_retention_days" type="number" min="1" max="3650" class="input" />
            </div>
            <div class="lg:col-span-2">
              <label class="input-label">{{ t('admin.riskControl.blockMessage') }}</label>
              <input v-model.trim="keywordForm.block_message" type="text" class="input" />
            </div>
          </div>

          <div class="space-y-4">
            <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.keyword.groupScope') }}</h3>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.keyword.groupScopeHint') }}</p>
              </div>
              <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
                <button type="button" class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors" :class="keywordForm.all_groups ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'" @click="keywordForm.all_groups = true">
                  {{ t('admin.riskControl.allGroups') }}
                </button>
                <button type="button" class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors" :class="!keywordForm.all_groups ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 dark:text-gray-400'" @click="keywordForm.all_groups = false">
                  {{ t('admin.riskControl.selectedGroups') }}
                </button>
              </div>
            </div>
            <div v-if="!keywordForm.all_groups" class="grid max-h-60 grid-cols-1 gap-3 overflow-y-auto pr-1 md:grid-cols-2">
              <div v-if="keywordGroupScopeInvalid" class="md:col-span-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300">
                {{ t('admin.riskControl.keyword.groupScopeEmptyHint') }}
              </div>
              <button
                v-for="group in groups"
                :key="group.id"
                type="button"
                class="flex items-center justify-between rounded-lg border p-3 text-left transition-colors"
                :class="isKeywordGroupSelected(group.id) ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20' : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                @click="toggleKeywordGroup(group.id)"
              >
                <span class="min-w-0">
                  <span class="block truncate text-sm font-semibold text-gray-900 dark:text-white">{{ group.name }}</span>
                  <span class="text-xs text-gray-400">{{ group.platform }}</span>
                </span>
                <Icon v-if="isKeywordGroupSelected(group.id)" name="check" size="sm" class="text-primary-500" />
              </button>
              <p v-if="groups.length === 0" class="text-sm text-gray-500 dark:text-gray-400">{{ keywordGroupStateText }}</p>
            </div>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end gap-2">
            <button type="button" class="btn btn-secondary" @click="keywordSettingsOpen = false">{{ t('common.cancel') }}</button>
            <button type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="keywordSaving" @click="saveKeywordConfig">
              <Icon v-if="keywordSaving" name="refresh" size="sm" class="animate-spin" />
              <Icon v-else name="check" size="sm" />
              {{ keywordSaving ? t('common.saving') : t('admin.riskControl.keyword.save') }}
            </button>
          </div>
        </template>
      </BaseDialog>

      <BaseDialog
        :show="inputDetailRow !== null"
        :title="t('admin.riskControl.inputDetailTitle')"
        width="wide"
        @close="closeInputDetail"
      >
        <div v-if="inputDetailRow" class="space-y-5">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.time') }}</p>
              <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ formatDateTime(inputDetailRow.created_at) }}</p>
            </div>
            <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.user') }}</p>
              <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ inputDetailRow.user_email || '-' }}</p>
            </div>
            <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.result') }}</p>
              <span class="mt-1 inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="resultBadgeClass(inputDetailRow)">
                {{ resultLabel(inputDetailRow) }}
              </span>
            </div>
            <div class="rounded-lg border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/70">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.highest') }}</p>
              <p class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">
                {{ inputDetailRow.highest_category || '-' }} / {{ percent(inputDetailRow.highest_score) }}
              </p>
            </div>
          </div>

          <div class="rounded-xl border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.inputDetailContent') }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ inputDetailRow.endpoint || '-' }} · {{ inputDetailRow.provider || '-' }} / {{ inputDetailRow.model || '-' }}
                </p>
              </div>
              <span v-if="inputDetailRow.group_name" class="inline-flex rounded-md bg-sky-50 px-2.5 py-1 text-xs font-medium text-sky-700 dark:bg-sky-900/20 dark:text-sky-300">
                {{ inputDetailRow.group_name }}
              </span>
            </div>
            <pre class="mt-4 max-h-[420px] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-gray-950 p-4 text-sm leading-6 text-gray-100 shadow-inner dark:bg-black/50">{{ inputDetailText }}</pre>
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end">
            <button type="button" class="btn btn-secondary" @click="closeInputDetail">{{ t('common.close') }}</button>
          </div>
        </template>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Pagination from '@/components/common/Pagination.vue'
import { adminAPI } from '@/api/admin'
import type {
  ContentModerationAPIKeyStatus,
  ContentModerationConfig,
  ContentModerationLog,
  ContentModerationRuntimeStatus,
  ContentModerationTestAuditResult,
  KeywordFilterConfig,
  KeywordFilterLog,
  KeywordFilterMatchMode,
  KeywordFilterRegexRule,
  KeywordFilterRule,
  KeywordFilterTestResponse,
  KeywordFilterWhitelistRule,
  ModerationMode,
  UpdateContentModerationConfig,
  UpdateKeywordFilterConfig,
} from '@/api/admin/riskControl'
import type { AdminGroup, SelectOption } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime as formatDateTimeValue } from '@/utils/format'

type SettingsTab = 'basic' | 'scope' | 'runtime' | 'response' | 'retention'
type RecordTab = 'keyword' | 'moderation'
type WorkerSlotState = 'active' | 'idle' | 'disabled'
type APIKeysWriteMode = 'append' | 'replace'
type KeywordRuleViewMode = 'simple' | 'advanced'
type OverviewIcon = 'shield' | 'key' | 'users' | 'document'
type OverviewItem = {
  key: string
  label: string
  value: string
  meta: string
  icon: OverviewIcon
  iconClass: string
  badge?: string
  badgeClass?: string
}
type ModerationScoreRow = {
  category: string
  score: number
  threshold: number
  hit: boolean
}

const maxModerationTestImages = 1
const maxModerationTestImageSize = 8 * 1024 * 1024
const maxVisibleApiKeyRows: number = 3
const defaultContentModerationBlockMessage = '内容审计命中风险规则，请调整输入后重试'
const defaultKeywordFilterBlockMessage = '输入内容命中关键词过滤规则，请调整后重试'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(true)
const saving = ref(false)
const keywordSaving = ref(false)
const logsLoading = ref(false)
const statusLoading = ref(false)
const groupsLoading = ref(false)
const groupsLoadFailed = ref(false)
const apiKeyTesting = ref(false)
const keywordTesting = ref(false)
const hashActionLoading = ref(false)
const unbanningUserID = ref<number | null>(null)
const settingsOpen = ref(false)
const keywordSettingsOpen = ref(false)
const activeRecordTab = ref<RecordTab>('keyword')
const activeSettingsTab = ref<SettingsTab>('basic')
const groupSearch = ref('')
const flaggedHashInput = ref('')
const groups = ref<AdminGroup[]>([])
const logs = ref<ContentModerationLog[]>([])
const keywordLogs = ref<KeywordFilterLog[]>([])
const status = ref<ContentModerationRuntimeStatus | null>(null)
const keywordRuleViewMode = ref<KeywordRuleViewMode>('simple')
const testedApiKeyStatuses = ref<ContentModerationAPIKeyStatus[]>([])
const pendingDeleteApiKeyHashes = ref<string[]>([])
const apiKeyRowsExpanded = ref<boolean>(false)
const moderationTestPrompt = ref('')
const moderationTestImages = ref<string[]>([])
const moderationTestResult = ref<ContentModerationTestAuditResult | null>(null)
const keywordTestText = ref('')
const keywordTestResult = ref<KeywordFilterTestResponse | null>(null)
const inputDetailRow = ref<ContentModerationLog | null>(null)
let statusTimer: number | null = null

const configForm = reactive({
  enabled: false,
  mode: 'pre_block' as ModerationMode,
  base_url: 'https://api.openai.com',
  model: 'omni-moderation-latest',
  api_keys_text: '',
  api_key_configured: false,
  api_key_masked: '',
  api_key_count: 0,
  api_key_masks: [] as string[],
  api_key_statuses: [] as ContentModerationAPIKeyStatus[],
  api_keys_mode: 'append' as APIKeysWriteMode,
  clear_api_key: false,
  timeout_ms: 3000,
  retry_count: 2,
  sample_rate: 100,
  all_groups: true,
  group_ids: [] as number[],
  record_non_hits: false,
  worker_count: 4,
  queue_size: 32768,
  block_status: 403,
  block_message: defaultContentModerationBlockMessage,
  email_on_hit: true,
  auto_ban_enabled: true,
  ban_threshold: 10,
  violation_window_hours: 720,
  hit_retention_days: 180,
  non_hit_retention_days: 3,
  pre_hash_check_enabled: false,
})

const keywordForm = reactive({
  enabled: false,
  all_groups: true,
  group_ids: [] as number[],
  keywords_text: '',
  whitelist_text: '',
  keyword_rules: [] as KeywordFilterRule[],
  whitelist_rules: [] as KeywordFilterWhitelistRule[],
  regex_rules: [] as KeywordFilterRegexRule[],
  block_status: 403,
  block_message: defaultKeywordFilterBlockMessage,
  hit_retention_days: 180,
})

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 1,
})

const filters = reactive({
  result: '',
  group_id: 0,
  endpoint: '',
  search: '',
  from: '',
  to: '',
})

const settingsTabs = computed<Array<{ id: SettingsTab; label: string }>>(() => [
  { id: 'basic', label: t('admin.riskControl.tabs.basic') },
  { id: 'scope', label: t('admin.riskControl.tabs.scope') },
  { id: 'runtime', label: t('admin.riskControl.tabs.runtime') },
  { id: 'response', label: t('admin.riskControl.tabs.response') },
  { id: 'retention', label: t('admin.riskControl.tabs.retention') },
])

const modeOptions = computed<SelectOption[]>(() => [
  { value: 'pre_block', label: t('admin.riskControl.modePreBlock') },
  { value: 'observe', label: t('admin.riskControl.modeObserve') },
  { value: 'off', label: t('admin.riskControl.modeOff') },
])

const keywordMatchModeOptions = computed<SelectOption[]>(() => [
  { value: 'auto', label: kt('matchModeAuto') },
  { value: 'contains', label: kt('matchModeContains') },
  { value: 'fuzzy', label: kt('matchModeFuzzy') },
  { value: 'token', label: kt('matchModeToken') },
  { value: 'exact_phrase', label: kt('matchModeExactPhrase') },
  { value: 'cjk_token', label: kt('matchModeCJKToken') },
])

const resultOptions = computed<SelectOption[]>(() => {
  if (activeRecordTab.value === 'keyword') {
    return [
      { value: '', label: t('admin.riskControl.keyword.resultAll') },
      { value: 'keyword', label: t('admin.riskControl.keyword.keyword') },
      { value: 'regex', label: t('admin.riskControl.keyword.regex') },
    ]
  }
  return [
    { value: '', label: t('admin.riskControl.result.all') },
    { value: 'hit', label: t('admin.riskControl.result.hit') },
    { value: 'blocked', label: t('admin.riskControl.result.blocked') },
    { value: 'pass', label: t('admin.riskControl.result.pass') },
    { value: 'error', label: t('admin.riskControl.result.error') },
  ]
})

const endpointOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.riskControl.filters.allEndpoints') },
  { value: '/v1/messages', label: '/v1/messages' },
  { value: '/v1/responses', label: '/v1/responses' },
  { value: '/v1/chat/completions', label: '/v1/chat/completions' },
  { value: '/v1beta/models', label: '/v1beta/models' },
  ...(activeRecordTab.value === 'keyword' ? [] : [
    { value: '/v1/images/generations', label: '/v1/images/generations' },
    { value: '/v1/images/edits', label: '/v1/images/edits' },
  ]),
])

const groupFilterOptions = computed<SelectOption[]>(() => [
  { value: 0, label: t('admin.riskControl.filters.allGroups') },
  ...groups.value.map((group) => ({
    value: group.id,
    label: `${group.name} (${group.platform})`,
  })),
])

const selectedGroupCount = computed(() => String(configForm.group_ids.length))

const filteredGroups = computed(() => {
  const keyword = groupSearch.value.trim().toLowerCase()
  if (!keyword) return groups.value
  return groups.value.filter((group) => {
    return group.name.toLowerCase().includes(keyword) || String(group.platform).toLowerCase().includes(keyword)
  })
})

const keywordGroupStateText = computed(() => {
  if (groupsLoading.value) return t('common.loading')
  if (groupsLoadFailed.value) return t('admin.riskControl.keyword.groupsLoadFailed')
  return t('admin.riskControl.noGroups')
})

const keywordGroupScopeInvalid = computed(() => !keywordForm.all_groups && keywordForm.group_ids.length === 0)

const inputApiKeyCount = computed(() => parseApiKeys(configForm.api_keys_text).length)

const pendingDeletedApiKeyCount = computed(() => pendingDeleteApiKeyHashes.value.length)

const effectiveStoredApiKeyCount = computed(() => Math.max(0, configForm.api_key_count - pendingDeletedApiKeyCount.value))

const apiKeysPlaceholder = computed(() => (
  configForm.api_keys_mode === 'replace'
    ? t('admin.riskControl.apiKeysPlaceholderReplace')
    : t('admin.riskControl.apiKeysPlaceholder')
))

const apiKeysModeHint = computed(() => (
  configForm.api_keys_mode === 'replace'
    ? t('admin.riskControl.apiKeysModeReplaceHint')
    : t('admin.riskControl.apiKeysModeAppendHint')
))

const hasModerationAuditInput = computed(() => {
  return moderationTestPrompt.value.trim() !== '' || moderationTestImages.value.length > 0
})

const isFlaggedHashInputValid = computed(() => /^[a-fA-F0-9]{64}$/.test(flaggedHashInput.value.trim()))

const storedApiKeyTestButtonText = computed(() => {
  if (apiKeyTesting.value) return t('admin.riskControl.testingApiKeys')
  if (hasModerationAuditInput.value) return t('admin.riskControl.testContentWithStoredApiKey')
  return t('admin.riskControl.testStoredApiKeys')
})

const savedApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => {
  const rows = status.value?.api_key_statuses?.length
    ? status.value.api_key_statuses
    : configForm.api_key_statuses
  return Array.isArray(rows) ? rows : []
})

const apiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => [
  ...savedApiKeyRows.value,
  ...testedApiKeyStatuses.value,
])

const visibleApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => {
  if (apiKeyRowsExpanded.value) return apiKeyRows.value
  return apiKeyRows.value.slice(0, maxVisibleApiKeyRows)
})

const hiddenApiKeyRowCount = computed<number>(() => Math.max(0, apiKeyRows.value.length - visibleApiKeyRows.value.length))

const canToggleApiKeyRows = computed<boolean>(() => apiKeyRows.value.length > maxVisibleApiKeyRows)

const activeSavedApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => (
  savedApiKeyRows.value.filter((row) => !isStoredApiKeyPendingDelete(row))
))

const apiKeyHealthBadges = computed<Array<{ status: ContentModerationAPIKeyStatus['status']; count: number }>>(() => {
  const counts: Record<ContentModerationAPIKeyStatus['status'], number> = {
    ok: 0,
    error: 0,
    frozen: 0,
    unknown: 0,
  }
  for (const row of activeSavedApiKeyRows.value) {
    counts[row.status] = (counts[row.status] ?? 0) + 1
  }
  if (activeSavedApiKeyRows.value.length === 0 && effectiveStoredApiKeyCount.value > 0) {
    counts.unknown = effectiveStoredApiKeyCount.value
  }
  return (['ok', 'frozen', 'error', 'unknown'] as Array<ContentModerationAPIKeyStatus['status']>)
    .map((item) => ({ status: item, count: counts[item] }))
    .filter((item) => item.count > 0)
})

const apiKeyHealthSummary = computed(() => {
  if (!configForm.api_key_configured) return ''
  if (apiKeyHealthBadges.value.length === 0) return t('admin.riskControl.apiKeyStatusUnknown')
  return apiKeyHealthBadges.value
    .map((badge) => `${apiKeyStatusLabel(badge.status)} ${badge.count}`)
    .join(' · ')
})

const overviewItems = computed<OverviewItem[]>(() => [
  {
    key: 'status',
    label: t('admin.riskControl.overview.status'),
    value: configForm.enabled ? t('admin.riskControl.overview.enabled') : t('admin.riskControl.overview.disabled'),
    meta: modeLabel(configForm.mode),
    icon: 'shield',
    iconClass: configForm.enabled
      ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-300'
      : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400',
    badge: runtimeBadgeText.value,
    badgeClass: runtimeBadgeClass.value,
  },
  {
    key: 'api-key',
    label: t('admin.riskControl.overview.apiKey'),
    value: configForm.api_key_configured ? t('admin.riskControl.apiKeyCount', { count: configForm.api_key_count }) : t('admin.riskControl.notConfigured'),
    meta: configForm.api_key_configured ? apiKeyHealthSummary.value || configForm.model || '-' : configForm.model || '-',
    icon: 'key',
    iconClass: 'bg-sky-50 text-sky-600 dark:bg-sky-900/20 dark:text-sky-300',
  },
  {
    key: 'scope',
    label: t('admin.riskControl.overview.groupScope'),
    value: configForm.all_groups ? t('admin.riskControl.allGroups') : selectedGroupCount.value,
    meta: configForm.all_groups ? t('admin.riskControl.allGroupsHint') : t('admin.riskControl.selectedGroupsHint'),
    icon: 'users',
    iconClass: 'bg-violet-50 text-violet-600 dark:bg-violet-900/20 dark:text-violet-300',
  },
  {
    key: 'logs',
    label: t('admin.riskControl.overview.logs'),
    value: formatNumber(pagination.total),
    meta: t('admin.riskControl.overview.currentFilter'),
    icon: 'document',
    iconClass: 'bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-300',
  },
])

const moderationScoreRows = computed<ModerationScoreRow[]>(() => {
  const result = moderationTestResult.value
  if (!result) return []
  return Object.entries(result.category_scores || {})
    .map(([category, score]) => {
      const threshold = result.thresholds?.[category] ?? 1
      return {
        category,
        score,
        threshold,
        hit: score >= threshold,
      }
    })
    .sort((a, b) => b.score - a.score)
})

const inputDetailText = computed(() => {
  if (!inputDetailRow.value) return '-'
  return inputDetailRow.value.input_excerpt || inputDetailRow.value.error || '-'
})

const queueUsagePercent = computed(() => `${Math.min(100, Math.max(0, status.value?.queue_usage_percent ?? 0)).toFixed(1)}%`)

const queueUsageStyle = computed(() => ({
  width: queueUsagePercent.value,
}))

const workerSlots = computed(() => {
  const total = Math.max(0, status.value?.worker_count ?? configForm.worker_count)
  const active = Math.max(0, status.value?.active_workers ?? 0)
  const enabled = Boolean(status.value?.risk_control_enabled && status.value?.enabled && status.value?.mode !== 'off')
  return Array.from({ length: total }, (_, index) => ({
    id: index + 1,
    state: (!enabled ? 'disabled' : index < active ? 'active' : 'idle') as WorkerSlotState,
    label: !enabled
      ? t('admin.riskControl.workerDisabled')
      : index < active
        ? t('admin.riskControl.workerActive')
        : t('admin.riskControl.workerIdle'),
  }))
})

const runtimeBadgeText = computed(() => {
  if (!status.value?.risk_control_enabled) return t('admin.riskControl.riskSwitchOff')
  if (!configForm.enabled || configForm.mode === 'off') return t('admin.riskControl.overview.disabled')
  return t('admin.riskControl.overview.enabled')
})

const runtimeBadgeClass = computed(() => {
  if (!status.value?.risk_control_enabled || !configForm.enabled || configForm.mode === 'off') {
    return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
  return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
})

function applyConfig(config: ContentModerationConfig) {
  configForm.enabled = config.enabled
  configForm.mode = config.mode
  configForm.base_url = config.base_url || 'https://api.openai.com'
  configForm.model = config.model || 'omni-moderation-latest'
  configForm.api_keys_text = ''
  configForm.api_key_configured = config.api_key_configured
  configForm.api_key_masked = config.api_key_masked || ''
  configForm.api_key_count = config.api_key_count || 0
  configForm.api_key_masks = Array.isArray(config.api_key_masks) ? [...config.api_key_masks] : []
  configForm.api_key_statuses = Array.isArray(config.api_key_statuses) ? [...config.api_key_statuses] : []
  configForm.api_keys_mode = 'append'
  configForm.clear_api_key = false
  pendingDeleteApiKeyHashes.value = []
  testedApiKeyStatuses.value = []
  apiKeyRowsExpanded.value = false
  configForm.timeout_ms = config.timeout_ms || 3000
  configForm.retry_count = config.retry_count ?? 2
  configForm.sample_rate = config.sample_rate ?? 100
  configForm.all_groups = config.all_groups
  configForm.group_ids = Array.isArray(config.group_ids) ? [...config.group_ids] : []
  configForm.record_non_hits = config.record_non_hits
  configForm.worker_count = config.worker_count || 4
  configForm.queue_size = config.queue_size || 32768
  configForm.block_status = config.block_status || 403
  configForm.block_message = config.block_message || defaultContentModerationBlockMessage
  configForm.email_on_hit = config.email_on_hit ?? true
  configForm.auto_ban_enabled = config.auto_ban_enabled ?? true
  configForm.ban_threshold = config.ban_threshold || 10
  configForm.violation_window_hours = config.violation_window_hours || 720
  configForm.hit_retention_days = config.hit_retention_days || 180
  configForm.non_hit_retention_days = Math.min(Math.max(config.non_hit_retention_days || 3, 1), 3)
  configForm.pre_hash_check_enabled = config.pre_hash_check_enabled ?? false
}

function applyKeywordConfig(config: KeywordFilterConfig) {
  keywordForm.enabled = config.enabled
  keywordForm.all_groups = config.all_groups
  keywordForm.group_ids = Array.isArray(config.group_ids) ? [...config.group_ids] : []
  keywordForm.keywords_text = (config.keywords || []).join('\n')
  keywordForm.whitelist_text = (config.whitelist || []).join('\n')
  keywordForm.keyword_rules = normalizeKeywordRules(config.keyword_rules || [], config.keywords || [])
  keywordForm.whitelist_rules = normalizeWhitelistRules(config.whitelist_rules || [], config.whitelist || [])
  syncKeywordSimpleTextFromRules()
  keywordForm.regex_rules = Array.isArray(config.regex_rules) ? config.regex_rules.map((rule) => ({ ...rule })) : []
  keywordForm.block_status = config.block_status || 403
  keywordForm.block_message = config.block_message || defaultKeywordFilterBlockMessage
  keywordForm.hit_retention_days = config.hit_retention_days || 180
  keywordRuleViewMode.value = hasAdvancedKeywordConfig(config) ? 'advanced' : 'simple'
}

async function loadGroups(options: { silent?: boolean } = {}) {
  groupsLoading.value = true
  groupsLoadFailed.value = false
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch (err: unknown) {
    groupsLoadFailed.value = true
    if (!options.silent) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.keyword.groupsLoadFailed')))
    }
  } finally {
    groupsLoading.value = false
  }
}

async function loadAll() {
  loading.value = true
  try {
    const [config, runtimeStatus] = await Promise.all([
      adminAPI.riskControl.getConfig(),
      adminAPI.riskControl.getStatus(),
    ])
    await loadGroups({ silent: true })
    applyConfig(config)
    const keywordConfig = await adminAPI.riskControl.getKeywordFilterConfig()
    applyKeywordConfig(keywordConfig)
    status.value = runtimeStatus
    if (Array.isArray(runtimeStatus.api_key_statuses)) {
      configForm.api_key_statuses = [...runtimeStatus.api_key_statuses]
      prunePendingDeleteAPIKeyHashes()
    }
    await loadLogs()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function loadStatus(silent = true) {
  statusLoading.value = true
  try {
    const runtimeStatus = await adminAPI.riskControl.getStatus()
    status.value = runtimeStatus
    if (Array.isArray(runtimeStatus.api_key_statuses)) {
      configForm.api_key_statuses = [...runtimeStatus.api_key_statuses]
      prunePendingDeleteAPIKeyHashes()
    }
  } catch (err: unknown) {
    if (!silent) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.statusFailed')))
    }
  } finally {
    statusLoading.value = false
  }
}

async function saveConfig() {
  saving.value = true
  try {
    const payload: UpdateContentModerationConfig = {
      enabled: configForm.enabled,
      mode: configForm.mode,
      base_url: configForm.base_url,
      model: configForm.model,
      timeout_ms: Number(configForm.timeout_ms) || 3000,
      retry_count: Number(configForm.retry_count) || 0,
      sample_rate: Number(configForm.sample_rate) || 0,
      all_groups: configForm.all_groups,
      group_ids: configForm.all_groups ? [] : [...configForm.group_ids],
      record_non_hits: configForm.record_non_hits,
      clear_api_key: configForm.clear_api_key,
      worker_count: Number(configForm.worker_count) || 4,
      queue_size: Number(configForm.queue_size) || 32768,
      block_status: Number(configForm.block_status) || 403,
      block_message: configForm.block_message || defaultContentModerationBlockMessage,
      email_on_hit: configForm.email_on_hit,
      auto_ban_enabled: configForm.auto_ban_enabled,
      ban_threshold: Number(configForm.ban_threshold) || 10,
      violation_window_hours: Number(configForm.violation_window_hours) || 720,
      hit_retention_days: Number(configForm.hit_retention_days) || 180,
      non_hit_retention_days: Math.min(Math.max(Number(configForm.non_hit_retention_days) || 3, 1), 3),
      pre_hash_check_enabled: configForm.pre_hash_check_enabled,
    }
    const keys = parseApiKeys(configForm.api_keys_text)
    if (!payload.clear_api_key && configForm.api_keys_mode === 'replace' && keys.length === 0) {
      appStore.showError(t('admin.riskControl.apiKeysReplaceNoInput'))
      return
    }
    if (keys.length > 0) {
      payload.api_keys = keys
      payload.api_keys_mode = configForm.api_keys_mode
      payload.clear_api_key = false
    }
    if (!payload.clear_api_key && configForm.api_keys_mode !== 'replace' && pendingDeleteApiKeyHashes.value.length > 0) {
      payload.delete_api_key_hashes = [...pendingDeleteApiKeyHashes.value]
    }

    const updated = await adminAPI.riskControl.updateConfig(payload)
    applyConfig(updated)
    settingsOpen.value = false
    appStore.showSuccess(t('admin.riskControl.saved'))
    await Promise.all([loadStatus(true), loadLogs()])
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function loadLogs() {
  logsLoading.value = true
  try {
    const params = {
      page: pagination.page,
      page_size: pagination.page_size,
      result: filters.result || undefined,
      group_id: filters.group_id || undefined,
      endpoint: filters.endpoint || undefined,
      search: filters.search || undefined,
      from: normalizeDateTimeLocal(filters.from),
      to: normalizeDateTimeLocal(filters.to),
    }
    const result = activeRecordTab.value === 'keyword'
      ? await adminAPI.riskControl.listKeywordFilterLogs({
        page: params.page,
        page_size: params.page_size,
        match_type: filters.result || undefined,
        group_id: params.group_id,
        endpoint: params.endpoint,
        search: params.search,
        from: params.from,
        to: params.to,
      })
      : await adminAPI.riskControl.listLogs(params)
    if (activeRecordTab.value === 'keyword') {
      keywordLogs.value = result.items as KeywordFilterLog[]
    } else {
      logs.value = result.items as ContentModerationLog[]
    }
    pagination.total = result.total
    pagination.page = result.page
    pagination.page_size = result.page_size
    pagination.pages = result.pages
  } catch (err: unknown) {
    const fallback = activeRecordTab.value === 'keyword'
      ? t('admin.riskControl.keyword.logsFailed')
      : t('admin.riskControl.logsFailed')
    appStore.showError(extractApiErrorMessage(err, fallback))
  } finally {
    logsLoading.value = false
  }
}

function buildKeywordPayload(): UpdateKeywordFilterConfig {
  syncKeywordRuleStateForCurrentMode()
  const keywordRules = sanitizeKeywordRules(keywordForm.keyword_rules)
  const whitelistRules = pruneWhitelistTargets(
    sanitizeWhitelistRules(keywordForm.whitelist_rules),
    keywordRules,
  )
  keywordForm.keyword_rules = keywordRules
  keywordForm.whitelist_rules = whitelistRules
  syncKeywordSimpleTextFromRules()

  return {
    enabled: keywordForm.enabled,
    all_groups: keywordForm.all_groups,
    group_ids: keywordForm.all_groups ? [] : [...keywordForm.group_ids],
    keywords: keywordRules.map((rule) => rule.pattern),
    whitelist: whitelistRules.map((rule) => rule.pattern),
    keyword_rules: keywordRules,
    whitelist_rules: whitelistRules,
    regex_rules: keywordForm.regex_rules.map((rule) => ({ ...rule })),
    block_status: Number(keywordForm.block_status) || 403,
    block_message: keywordForm.block_message || defaultKeywordFilterBlockMessage,
    hit_retention_days: Number(keywordForm.hit_retention_days) || 180,
  }
}

async function saveKeywordConfig() {
  keywordSaving.value = true
  try {
    if (keywordGroupScopeInvalid.value) {
      appStore.showError(t('admin.riskControl.keyword.groupScopeEmptyHint'))
      return
    }
    const payload = buildKeywordPayload()
    const updated = await adminAPI.riskControl.updateKeywordFilterConfig(payload)
    applyKeywordConfig(updated)
    keywordSettingsOpen.value = false
    appStore.showSuccess(t('admin.riskControl.keyword.saved'))
    if (activeRecordTab.value === 'keyword') {
      await loadLogs()
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.keyword.saveFailed')))
  } finally {
    keywordSaving.value = false
  }
}

async function testKeywordFilter() {
  keywordTesting.value = true
  try {
    keywordTestResult.value = await adminAPI.riskControl.testKeywordFilter({
      text: keywordTestText.value,
      config: buildKeywordPayload(),
    })
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.keyword.testFailed')))
  } finally {
    keywordTesting.value = false
  }
}

function canUnbanRow(row: ContentModerationLog): boolean {
  return Boolean(row.auto_banned && row.user_id && row.user_status === 'disabled')
}

function inputSummaryText(row: ContentModerationLog): string {
  return row.input_excerpt || row.error || '-'
}

function openInputDetail(row: ContentModerationLog) {
  inputDetailRow.value = row
}

function closeInputDetail() {
  inputDetailRow.value = null
}

async function unbanUser(row: ContentModerationLog) {
  if (!row.user_id || unbanningUserID.value !== null) return
  unbanningUserID.value = row.user_id
  try {
    const result = await adminAPI.riskControl.unbanUser(row.user_id)
    logs.value = logs.value.map((item) => {
      if (item.user_id !== row.user_id) return item
      return { ...item, user_status: result.status }
    })
    appStore.showSuccess(t('admin.riskControl.unbanSuccess'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.unbanFailed')))
  } finally {
    unbanningUserID.value = null
  }
}

async function deleteFlaggedHash() {
  if (!isFlaggedHashInputValid.value || hashActionLoading.value) return
  hashActionLoading.value = true
  try {
    const result = await adminAPI.riskControl.deleteFlaggedHash(flaggedHashInput.value)
    flaggedHashInput.value = ''
    await loadStatus(true)
    appStore.showSuccess(result.deleted ? t('admin.riskControl.flaggedHashDeleted') : t('admin.riskControl.flaggedHashNotFound'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.flaggedHashDeleteFailed')))
  } finally {
    hashActionLoading.value = false
  }
}

async function clearFlaggedHashes() {
  if (hashActionLoading.value) return
  const confirmed = window.confirm(t('admin.riskControl.clearFlaggedHashesConfirm'))
  if (!confirmed) return
  hashActionLoading.value = true
  try {
    const result = await adminAPI.riskControl.clearFlaggedHashes()
    await loadStatus(true)
    appStore.showSuccess(t('admin.riskControl.flaggedHashesCleared', { count: result.deleted }))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.flaggedHashesClearFailed')))
  } finally {
    hashActionLoading.value = false
  }
}

function openSettings() {
  activeSettingsTab.value = 'basic'
  settingsOpen.value = true
}

function openKeywordSettings() {
  keywordSettingsOpen.value = true
  if (groups.value.length === 0 && !groupsLoading.value) {
    void loadGroups()
  }
}

function switchRecordTab(tab: RecordTab) {
  if (activeRecordTab.value === tab) return
  activeRecordTab.value = tab
  filters.result = ''
  pagination.page = 1
  void loadLogs()
}

function reloadLogsFromFirstPage() {
  pagination.page = 1
  void loadLogs()
}

function addKeywordRegexRule() {
  keywordForm.regex_rules.push({ name: '', pattern: '', enabled: false })
}

function removeKeywordRegexRule(index: number) {
  keywordForm.regex_rules.splice(index, 1)
}

function addKeywordRule() {
  keywordForm.keyword_rules.push(createKeywordRule('', 'keyword'))
}

function removeKeywordRule(index: number) {
  keywordForm.keyword_rules.splice(index, 1)
  const validIDs = new Set(keywordForm.keyword_rules.map((rule) => rule.id).filter(Boolean))
  keywordForm.whitelist_rules = keywordForm.whitelist_rules.map((rule) => ({
    ...rule,
    target_rule_ids: rule.target_rule_ids.filter((id) => validIDs.has(id)),
  }))
}

function addWhitelistRule() {
  keywordForm.whitelist_rules.push(createWhitelistRule(''))
}

function removeWhitelistRule(index: number) {
  keywordForm.whitelist_rules.splice(index, 1)
}

function toggleWhitelistTarget(rule: KeywordFilterWhitelistRule, targetID: string) {
  if (!targetID) return
  const index = rule.target_rule_ids.indexOf(targetID)
  if (index >= 0) {
    rule.target_rule_ids.splice(index, 1)
  } else {
    rule.target_rule_ids.push(targetID)
  }
}

function toggleKeywordGroup(groupID: number) {
  const index = keywordForm.group_ids.indexOf(groupID)
  if (index >= 0) {
    keywordForm.group_ids.splice(index, 1)
  } else {
    keywordForm.group_ids.push(groupID)
  }
}

function isKeywordGroupSelected(groupID: number): boolean {
  return keywordForm.group_ids.includes(groupID)
}

function onPageChange(page: number) {
  pagination.page = page
  void loadLogs()
}

function onPageSizeChange(pageSize: number) {
  pagination.page = 1
  pagination.page_size = pageSize
  void loadLogs()
}

function toggleClearApiKey() {
  configForm.clear_api_key = !configForm.clear_api_key
  if (configForm.clear_api_key) {
    configForm.api_keys_text = ''
    configForm.api_keys_mode = 'append'
    testedApiKeyStatuses.value = []
    pendingDeleteApiKeyHashes.value = []
  }
}

function setAPIKeysMode(mode: APIKeysWriteMode) {
  configForm.api_keys_mode = mode
  if (mode === 'replace') {
    pendingDeleteApiKeyHashes.value = []
  }
}

async function testApiKeys(useInputKeys: boolean) {
  const keys = useInputKeys ? parseApiKeys(configForm.api_keys_text) : []
  if (useInputKeys && keys.length === 0) {
    appStore.showError(t('admin.riskControl.apiKeyTestNoInput'))
    return
  }
  apiKeyTesting.value = true
  try {
    const result = await adminAPI.riskControl.testAPIKeys({
      api_keys: keys,
      base_url: configForm.base_url,
      model: configForm.model,
      timeout_ms: Number(configForm.timeout_ms) || 3000,
      prompt: moderationTestPrompt.value,
      images: moderationTestImages.value,
    })
    moderationTestResult.value = result.audit_result ?? null
    if (useInputKeys) {
      testedApiKeyStatuses.value = result.items.map((item) => ({ ...item, configured: false }))
    } else {
      mergeConfiguredAPIKeyStatuses(result.items)
      testedApiKeyStatuses.value = []
      await loadStatus(true)
    }
    appStore.showSuccess(t('admin.riskControl.apiKeyTestDone', { count: result.items.length }))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.apiKeyTestFailed')))
  } finally {
    apiKeyTesting.value = false
  }
}

function mergeConfiguredAPIKeyStatuses(items: ContentModerationAPIKeyStatus[]) {
  if (!hasModerationAuditInput.value || configForm.api_key_statuses.length === 0) {
    configForm.api_key_statuses = items
    return
  }
  const updates = new Map(items.map((item) => [item.key_hash, item]))
  configForm.api_key_statuses = configForm.api_key_statuses.map((item) => updates.get(item.key_hash) ?? item)
}

function toggleDeleteStoredApiKey(row: ContentModerationAPIKeyStatus) {
  if (!row.configured || !row.key_hash) return
  const index = pendingDeleteApiKeyHashes.value.indexOf(row.key_hash)
  if (index >= 0) {
    pendingDeleteApiKeyHashes.value.splice(index, 1)
    return
  }
  pendingDeleteApiKeyHashes.value.push(row.key_hash)
}

function isStoredApiKeyPendingDelete(row: ContentModerationAPIKeyStatus): boolean {
  return row.configured && row.key_hash !== '' && pendingDeleteApiKeyHashes.value.includes(row.key_hash)
}

function prunePendingDeleteAPIKeyHashes() {
  const currentHashes = new Set(savedApiKeyRows.value.map((row) => row.key_hash).filter(Boolean))
  pendingDeleteApiKeyHashes.value = pendingDeleteApiKeyHashes.value.filter((hash) => currentHashes.has(hash))
}

function clearModerationTestInput() {
  moderationTestPrompt.value = ''
  moderationTestImages.value = []
  moderationTestResult.value = null
}

function removeModerationTestImage(index: number) {
  moderationTestImages.value.splice(index, 1)
}

async function handleModerationImageUpload(event: Event) {
  const input = event.target as HTMLInputElement
  await addModerationTestFiles(input.files)
  input.value = ''
}

async function handleModerationImageDrop(event: DragEvent) {
  await addModerationTestFiles(event.dataTransfer?.files ?? null)
}

async function handleModerationImagePaste(event: ClipboardEvent) {
  const files = Array.from(event.clipboardData?.files ?? []).filter((file) => file.type.startsWith('image/'))
  if (files.length === 0) return
  event.preventDefault()
  await addModerationTestFiles(files)
}

async function addModerationTestFiles(files: FileList | File[] | null) {
  if (!files) return
  const items = Array.from(files).filter((file) => file.type.startsWith('image/'))
  for (const file of items) {
    if (moderationTestImages.value.length >= maxModerationTestImages) {
      appStore.showError(t('admin.riskControl.auditTestImageLimit', { count: maxModerationTestImages }))
      return
    }
    if (file.size > maxModerationTestImageSize) {
      appStore.showError(t('admin.riskControl.auditTestImageTooLarge'))
      continue
    }
    try {
      moderationTestImages.value.push(await fileToDataURL(file))
    } catch {
      appStore.showError(t('admin.riskControl.auditTestImageReadFailed'))
    }
  }
}

function fileToDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

function toggleGroup(groupID: number) {
  const index = configForm.group_ids.indexOf(groupID)
  if (index >= 0) {
    configForm.group_ids.splice(index, 1)
  } else {
    configForm.group_ids.push(groupID)
  }
}

function isGroupSelected(groupID: number): boolean {
  return configForm.group_ids.includes(groupID)
}

function modeLabel(mode: ModerationMode): string {
  const found = modeOptions.value.find((option) => option.value === mode)
  return found?.label ?? mode
}

function modeDescription(mode: ModerationMode): string {
  const descriptions: Record<ModerationMode, string> = {
    pre_block: t('admin.riskControl.modePreBlockDesc'),
    observe: t('admin.riskControl.modeObserveDesc'),
    off: t('admin.riskControl.modeOffDesc'),
  }
  return descriptions[mode] ?? ''
}

function resultLabel(row: ContentModerationLog): string {
  if (row.action === 'block') return t('admin.riskControl.action.block')
  if (row.action === 'error' || row.error) return t('admin.riskControl.action.error')
  if (row.flagged) return t('admin.riskControl.result.hit')
  return t('admin.riskControl.result.pass')
}

function resultBadgeClass(row: ContentModerationLog): string {
  if (row.action === 'block') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (row.action === 'error' || row.error) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (row.flagged) return 'bg-pink-100 text-pink-700 dark:bg-pink-900/30 dark:text-pink-300'
  return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
}

function workerSlotClass(state: WorkerSlotState): string {
  if (state === 'active') {
    return 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900/60 dark:bg-sky-900/20 dark:text-sky-300'
  }
  if (state === 'idle') {
    return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-900/20 dark:text-emerald-300'
  }
  return 'border-gray-100 bg-white text-gray-400 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-500'
}

function workerDotClass(state: WorkerSlotState): string {
  if (state === 'active') return 'bg-sky-500'
  if (state === 'idle') return 'bg-emerald-500'
  return 'bg-gray-300 dark:bg-dark-500'
}

function percent(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return `${(value * 100).toFixed(1)}%`
}

function percentWidth(value: number): string {
  if (!Number.isFinite(value)) return '0%'
  return `${Math.min(100, Math.max(0, value * 100)).toFixed(1)}%`
}

function latencyText(value: number | null): string {
  if (value === null || value === undefined) return '-'
  return `${value} ms`
}

function apiKeyRowKey(row: ContentModerationAPIKeyStatus, index: number): string {
  return `${row.configured ? 'saved' : 'test'}-${row.key_hash || index}`
}

function apiKeyStatusLabel(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const labels: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: t('admin.riskControl.apiKeyStatusOk'),
    error: t('admin.riskControl.apiKeyStatusError'),
    frozen: t('admin.riskControl.apiKeyStatusFrozen'),
    unknown: t('admin.riskControl.apiKeyStatusUnknown'),
  }
  return labels[statusValue] ?? labels.unknown
}

function apiKeyStatusBadgeClass(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const classes: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300',
    error: 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300',
    frozen: 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300',
    unknown: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
  }
  return classes[statusValue] ?? classes.unknown
}

function apiKeyStatusDotClass(statusValue: ContentModerationAPIKeyStatus['status']): string {
  const classes: Record<ContentModerationAPIKeyStatus['status'], string> = {
    ok: 'bg-emerald-500',
    error: 'bg-amber-500',
    frozen: 'bg-red-500',
    unknown: 'bg-gray-400',
  }
  return classes[statusValue] ?? classes.unknown
}

function apiKeyStatusMeta(row: ContentModerationAPIKeyStatus): string {
  const parts: string[] = []
  parts.push(t('admin.riskControl.apiKeyFailureCount', { count: row.failure_count || 0 }))
  if (row.last_latency_ms > 0) {
    parts.push(t('admin.riskControl.apiKeyLatency', { ms: row.last_latency_ms }))
  }
  if (row.last_http_status > 0) {
    parts.push(t('admin.riskControl.apiKeyHTTPStatus', { status: row.last_http_status }))
  }
  if (row.frozen_until) {
    parts.push(t('admin.riskControl.apiKeyFrozenUntil', { time: formatDateTime(row.frozen_until) }))
  } else if (row.last_checked_at) {
    parts.push(t('admin.riskControl.apiKeyLastChecked', { time: formatDateTime(row.last_checked_at) }))
  } else {
    parts.push(t('admin.riskControl.apiKeyNotTested'))
  }
  return parts.join(' / ')
}

function parseApiKeys(value: string): string[] {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter((item, index, arr) => item && arr.indexOf(item) === index)
}

function parseLines(value: string): string[] {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter((item, index, arr) => item && arr.indexOf(item) === index)
}

function switchKeywordRuleViewMode(mode: KeywordRuleViewMode) {
  if (keywordRuleViewMode.value === mode) return
  syncKeywordRuleStateForCurrentMode()
  keywordRuleViewMode.value = mode
}

function syncKeywordRuleStateForCurrentMode() {
  if (keywordRuleViewMode.value === 'simple') {
    syncKeywordRulesFromSimpleText()
  } else {
    syncKeywordSimpleTextFromRules()
  }
  cleanupKeywordWhitelistTargets()
}

function syncKeywordRulesFromSimpleText() {
  const keywordRules = mergeKeywordRulesFromLines(parseLines(keywordForm.keywords_text), keywordForm.keyword_rules)
  keywordForm.keyword_rules = keywordRules
  keywordForm.whitelist_rules = pruneWhitelistTargets(
    mergeWhitelistRulesFromLines(parseLines(keywordForm.whitelist_text), keywordForm.whitelist_rules),
    keywordRules,
  )
}

function syncKeywordSimpleTextFromRules() {
  keywordForm.keyword_rules = sanitizeKeywordRules(keywordForm.keyword_rules)
  keywordForm.whitelist_rules = pruneWhitelistTargets(
    sanitizeWhitelistRules(keywordForm.whitelist_rules),
    keywordForm.keyword_rules,
  )
  keywordForm.keywords_text = keywordForm.keyword_rules.map((rule) => rule.pattern).join('\n')
  keywordForm.whitelist_text = keywordForm.whitelist_rules.map((rule) => rule.pattern).join('\n')
}

function cleanupKeywordWhitelistTargets() {
  keywordForm.whitelist_rules = pruneWhitelistTargets(keywordForm.whitelist_rules, keywordForm.keyword_rules)
}

function mergeKeywordRulesFromLines(lines: string[], existingRules: KeywordFilterRule[]): KeywordFilterRule[] {
  const existingByPattern = new Map<string, KeywordFilterRule>()
  existingRules.forEach((rule) => {
    const pattern = normalizeKeywordPattern(rule.pattern)
    if (pattern && !existingByPattern.has(pattern)) {
      existingByPattern.set(pattern, rule)
    }
  })

  return lines.map((pattern, index) => {
    const existing = existingByPattern.get(pattern)
    return existing
      ? normalizeKeywordRule({ ...existing, pattern }, index)
      : createKeywordRule(pattern, 'keyword', index)
  })
}

function mergeWhitelistRulesFromLines(lines: string[], existingRules: KeywordFilterWhitelistRule[]): KeywordFilterWhitelistRule[] {
  const existingByPattern = new Map<string, KeywordFilterWhitelistRule>()
  existingRules.forEach((rule) => {
    const pattern = normalizeKeywordPattern(rule.pattern)
    if (pattern && !existingByPattern.has(pattern)) {
      existingByPattern.set(pattern, rule)
    }
  })

  return lines.map((pattern, index) => {
    const existing = existingByPattern.get(pattern)
    return existing
      ? normalizeWhitelistRule({ ...existing, pattern }, index)
      : createWhitelistRule(pattern, index)
  })
}

function sanitizeKeywordRules(rules: KeywordFilterRule[]): KeywordFilterRule[] {
  const seenPatterns = new Set<string>()
  return rules.reduce<KeywordFilterRule[]>((items, rule) => {
    const pattern = normalizeKeywordPattern(rule.pattern)
    if (!pattern || seenPatterns.has(pattern)) return items
    seenPatterns.add(pattern)
    items.push(normalizeKeywordRule({ ...rule, pattern }, items.length))
    return items
  }, [])
}

function sanitizeWhitelistRules(rules: KeywordFilterWhitelistRule[]): KeywordFilterWhitelistRule[] {
  const seenPatterns = new Set<string>()
  return rules.reduce<KeywordFilterWhitelistRule[]>((items, rule) => {
    const pattern = normalizeKeywordPattern(rule.pattern)
    if (!pattern || seenPatterns.has(pattern)) return items
    seenPatterns.add(pattern)
    items.push(normalizeWhitelistRule({ ...rule, pattern }, items.length))
    return items
  }, [])
}

function normalizeKeywordRule(rule: KeywordFilterRule, index: number): KeywordFilterRule {
  const pattern = normalizeKeywordPattern(rule.pattern)
  return {
    id: rule.id?.trim() || createKeywordRuleID('keyword', pattern || String(index), index),
    pattern,
    match_mode: normalizeMatchMode(rule.match_mode),
    enabled: rule.enabled ?? true,
    action: rule.action || 'block',
    severity: rule.severity || '',
  }
}

function normalizeWhitelistRule(rule: KeywordFilterWhitelistRule, index: number): KeywordFilterWhitelistRule {
  const pattern = normalizeKeywordPattern(rule.pattern)
  return {
    id: rule.id?.trim() || createKeywordRuleID('whitelist', pattern || String(index), index),
    pattern,
    match_mode: normalizeMatchMode(rule.match_mode),
    target_rule_ids: Array.isArray(rule.target_rule_ids) ? [...rule.target_rule_ids] : [],
    enabled: rule.enabled ?? true,
  }
}

function pruneWhitelistTargets(
  whitelistRules: KeywordFilterWhitelistRule[],
  keywordRules: KeywordFilterRule[],
): KeywordFilterWhitelistRule[] {
  const validIDs = new Set(keywordRules.map((rule) => rule.id).filter(Boolean))
  return whitelistRules.map((rule) => ({
    ...rule,
    target_rule_ids: rule.target_rule_ids.filter((id) => validIDs.has(id)),
  }))
}

function normalizeKeywordPattern(pattern: string | undefined): string {
  return (pattern || '').trim()
}

function normalizeKeywordRules(rules: KeywordFilterRule[], fallbackKeywords: string[]): KeywordFilterRule[] {
  const source = Array.isArray(rules) && rules.length > 0
    ? rules
    : rulesFromLines(fallbackKeywords || [], 'legacy')
  return sanitizeKeywordRules(source)
}

function hasAdvancedKeywordConfig(config: KeywordFilterConfig): boolean {
  const keywordRules = Array.isArray(config.keyword_rules) ? config.keyword_rules : []
  const whitelistRules = Array.isArray(config.whitelist_rules) ? config.whitelist_rules : []
  return keywordRules.some((rule) => {
    return normalizeMatchMode(rule.match_mode) !== 'auto' ||
      rule.enabled === false ||
      Boolean(rule.severity) ||
      (rule.action && rule.action !== 'block')
  }) || whitelistRules.some((rule) => {
    return normalizeMatchMode(rule.match_mode) !== 'auto' ||
      rule.enabled === false ||
      (Array.isArray(rule.target_rule_ids) && rule.target_rule_ids.length > 0)
  })
}

function normalizeWhitelistRules(rules: KeywordFilterWhitelistRule[], fallbackWhitelist: string[]): KeywordFilterWhitelistRule[] {
  const source = Array.isArray(rules) && rules.length > 0
    ? rules
    : whitelistRulesFromLines(fallbackWhitelist || [])
  return sanitizeWhitelistRules(source)
}

function rulesFromLines(lines: string[], prefix: string): KeywordFilterRule[] {
  return lines.map((pattern, index) => createKeywordRule(pattern, prefix, index))
}

function whitelistRulesFromLines(lines: string[]): KeywordFilterWhitelistRule[] {
  return lines.map((pattern, index) => createWhitelistRule(pattern, index))
}

function createKeywordRule(pattern = '', prefix = 'keyword', index = Date.now()): KeywordFilterRule {
  return {
    id: createKeywordRuleID(prefix, pattern || String(index), index),
    pattern,
    match_mode: 'auto',
    enabled: true,
    action: 'block',
    severity: '',
  }
}

function createWhitelistRule(pattern = '', index = Date.now()): KeywordFilterWhitelistRule {
  return {
    id: createKeywordRuleID('whitelist', pattern || String(index), index),
    pattern,
    match_mode: 'auto',
    target_rule_ids: [],
    enabled: true,
  }
}

function createKeywordRuleID(prefix: string, pattern: string, index: number): string {
  const base = `${prefix}_${index}_${pattern}`
  let hash = 0
  for (let i = 0; i < base.length; i += 1) {
    hash = ((hash << 5) - hash + base.charCodeAt(i)) | 0
  }
  return `${prefix}_${Math.abs(hash).toString(36)}`
}

function normalizeMatchMode(mode: string | undefined): KeywordFilterMatchMode {
  const allowed: KeywordFilterMatchMode[] = ['auto', 'contains', 'fuzzy', 'token', 'exact_phrase', 'cjk_token']
  return allowed.includes(mode as KeywordFilterMatchMode) ? mode as KeywordFilterMatchMode : 'auto'
}

function matchModeLabel(mode: string): string {
  const found = keywordMatchModeOptions.value.find((option) => option.value === mode)
  return found?.label ?? mode
}

function kt(key: string): string {
  const fullKey = `admin.riskControl.keyword.${key}`
  const translated = t(fullKey)
  if (translated !== fullKey) return translated
  const fallback: Record<string, string> = {
    ruleEditor: 'Keyword Rules',
    ruleEditorHint: 'Use simple multiline input, or switch to advanced rules to choose match modes.',
    simpleRules: 'Simple',
    advancedRules: 'Advanced',
    keywordRules: 'Blocked Rules',
    whitelistRules: 'Whitelist Rules',
    addKeywordRule: 'Add Blocked Rule',
    addWhitelistRule: 'Add Whitelist Rule',
    rulePattern: 'Pattern',
    noKeywordRules: 'No keyword rules',
    noWhitelistRules: 'No whitelist rules',
    whitelistTargetsAll: 'Applies to all blocked rules',
    whitelistTargetsSome: 'Applies to selected blocked rules',
    matchModeAuto: 'Auto',
    matchModeContains: 'Contains',
    matchModeFuzzy: 'Fuzzy',
    matchModeToken: 'Token',
    matchModeExactPhrase: 'Exact phrase',
    matchModeCJKToken: 'CJK token',
    testRule: 'Rule',
    testSegment: 'Segment',
    testExcerpt: 'Excerpt',
  }
  return fallback[key] ?? key
}

function violationCountText(row: ContentModerationLog): string {
  if (!row.flagged) return '-'
  return t('admin.riskControl.violationCount', { count: row.violation_count || 1 })
}

function normalizeDateTimeLocal(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return undefined
  return date.toISOString()
}

function formatDateTime(value: string): string {
  return formatDateTimeValue(value) || '-'
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value)
}

onMounted(() => {
  void loadAll()
  statusTimer = window.setInterval(() => {
    void loadStatus(true)
  }, 15000)
})

onUnmounted(() => {
  if (statusTimer !== null) {
    window.clearInterval(statusTimer)
    statusTimer = null
  }
})
</script>
