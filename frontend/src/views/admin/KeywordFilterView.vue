<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-16">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <template v-else>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.keywordFilter.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.keywordFilter.description') }}</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="loading || refreshing" @click="refreshAll">
              <Icon name="refresh" size="sm" :class="refreshing ? 'animate-spin' : ''" />
              {{ t('common.refresh') }}
            </button>
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" @click="testOpen = true">
              <Icon name="beaker" size="sm" />
              {{ kt('test') }}
            </button>
            <button type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving" @click="saveConfig">
              <Icon :name="saving ? 'refresh' : 'check'" size="sm" :class="saving ? 'animate-spin' : ''" />
              {{ saving ? t('common.saving') : kt('save') }}
            </button>
          </div>
        </div>

        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ kt('settingsTitle') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ kt('enabledHint') }}</p>
          </div>
          <div class="grid grid-cols-1 items-stretch gap-5 p-6 xl:grid-cols-[minmax(360px,0.75fr)_minmax(0,1.25fr)]">
            <div class="flex min-h-[320px] flex-col rounded-lg border border-gray-100 p-4 dark:border-dark-700">
              <div class="flex items-center justify-between gap-4 rounded-lg bg-gray-50 px-4 py-3 dark:bg-dark-900/30">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ kt('enabled') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ kt('enabledHint') }}</p>
                </div>
                <Toggle v-model="keywordForm.enabled" />
              </div>
              <div class="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label class="input-label">{{ t('admin.riskControl.blockStatus') }}</label>
                  <input v-model.number="keywordForm.block_status" type="number" min="400" max="599" class="input" />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.riskControl.hitRetentionDays') }}</label>
                  <input v-model.number="keywordForm.hit_retention_days" type="number" min="1" max="3650" class="input" />
                </div>
                <div class="sm:col-span-2">
                  <label class="input-label">{{ t('admin.riskControl.blockMessage') }}</label>
                  <textarea v-model.trim="keywordForm.block_message" rows="4" class="input min-h-28 resize-y"></textarea>
                </div>
              </div>
            </div>

            <div class="flex min-h-[320px] flex-col rounded-lg border border-gray-100 p-4 dark:border-dark-700">
              <div class="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ kt('groupScope') }}</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ kt('groupScopeHint') }}</p>
                </div>
                <div class="inline-flex shrink-0 rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
                  <button type="button" class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors" :class="keywordForm.all_groups ? selectedSegmentClass : mutedSegmentClass" @click="keywordForm.all_groups = true">
                    {{ t('admin.riskControl.allGroups') }}
                  </button>
                  <button type="button" class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors" :class="!keywordForm.all_groups ? selectedSegmentClass : mutedSegmentClass" @click="keywordForm.all_groups = false">
                    {{ t('admin.riskControl.selectedGroups') }}
                  </button>
                </div>
              </div>

              <div class="mt-4 flex min-h-0 flex-1">
                <div v-if="keywordForm.all_groups" class="flex w-full items-center justify-center rounded-lg border border-dashed border-gray-200 bg-gray-50 px-4 py-10 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-900/30 dark:text-gray-400">
                  {{ t('admin.riskControl.allGroups') }}
                </div>
                <div v-else class="w-full overflow-y-auto pr-1">
                  <div class="grid grid-cols-1 gap-2 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
                    <div v-if="keywordGroupScopeInvalid" class="md:col-span-2 xl:col-span-3 2xl:col-span-4 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300">
                      {{ kt('groupScopeEmptyHint') }}
                    </div>
                    <button
                      v-for="group in groups"
                      :key="group.id"
                      type="button"
                      class="flex min-w-0 items-center justify-between rounded-lg border px-3 py-2.5 text-left transition-colors"
                      :class="isKeywordGroupSelected(group.id) ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20' : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
                      @click="toggleKeywordGroup(group.id)"
                    >
                      <span class="min-w-0">
                        <span class="block truncate text-sm font-semibold text-gray-900 dark:text-white">{{ group.name }}</span>
                        <span class="text-xs text-gray-400">{{ group.platform }}</span>
                      </span>
                      <Icon v-if="isKeywordGroupSelected(group.id)" name="check" size="sm" class="ml-2 shrink-0 text-primary-500" />
                    </button>
                    <p v-if="groups.length === 0" class="text-sm text-gray-500 dark:text-gray-400">{{ groupStateText }}</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ kt('regexRules') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ kt('regexRulesHint') }}</p>
            </div>
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" @click="addKeywordRegexRule">
              <Icon name="plus" size="sm" />
              {{ kt('addRegex') }}
            </button>
          </div>
          <div class="space-y-2 p-6">
            <div v-if="keywordForm.regex_rules.length === 0" class="rounded-lg border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
              {{ kt('noRegexRules') }}
            </div>
            <div v-for="(rule, index) in keywordForm.regex_rules" :key="`${rule.name}-${index}`" class="grid grid-cols-1 gap-2 rounded-lg bg-gray-50 p-3 dark:bg-dark-900/30 lg:grid-cols-[180px_minmax(0,1fr)_auto_auto] lg:items-center">
              <input v-model.trim="rule.name" type="text" class="input h-9" :placeholder="kt('regexName')" />
              <input v-model.trim="rule.pattern" type="text" class="input h-9 font-mono text-sm" :placeholder="kt('regexPattern')" />
              <label class="inline-flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
                <Toggle v-model="rule.enabled" />
                {{ kt('enabledShort') }}
              </label>
              <button type="button" class="inline-flex h-9 w-9 items-center justify-center rounded-md text-gray-400 hover:bg-gray-100 hover:text-red-600 dark:hover:bg-dark-700" @click="removeKeywordRegexRule(index)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
        </div>

        <RuleTable
          kind="keyword"
          :title="kt('keywordRules')"
          :empty-text="kt('noKeywordRules')"
          :rows="keywordPageRows"
          :total="filteredKeywordRules.length"
          :page="keywordTable.page"
          :page-size="keywordTable.pageSize"
          :search="keywordTable.search"
          :mode="keywordTable.mode"
          :enabled-filter="keywordTable.enabled"
          :match-mode-options="keywordMatchModeOptions"
          :refreshing="rulesRefreshing"
          @add="addKeywordRule"
          @import="openImport('keyword')"
          @refresh="refreshRules"
          @search="(value) => updateRuleFilter(keywordTable, 'search', value)"
          @mode="(value) => updateRuleFilter(keywordTable, 'mode', value)"
          @enabled="(value) => updateRuleFilter(keywordTable, 'enabled', value)"
          @page="(value) => keywordTable.page = value"
          @page-size="(value) => updateRulePageSize(keywordTable, value)"
          @remove="removeKeywordRule"
        />

        <RuleTable
          kind="whitelist"
          :title="kt('whitelistRules')"
          :empty-text="kt('noWhitelistRules')"
          :rows="whitelistPageRows"
          :total="filteredWhitelistRules.length"
          :page="whitelistTable.page"
          :page-size="whitelistTable.pageSize"
          :search="whitelistTable.search"
          :mode="whitelistTable.mode"
          :enabled-filter="whitelistTable.enabled"
          :match-mode-options="keywordMatchModeOptions"
          :target-options="keywordTargetOptions"
          :refreshing="rulesRefreshing"
          @add="addWhitelistRule"
          @import="openImport('whitelist')"
          @refresh="refreshRules"
          @search="(value) => updateRuleFilter(whitelistTable, 'search', value)"
          @mode="(value) => updateRuleFilter(whitelistTable, 'mode', value)"
          @enabled="(value) => updateRuleFilter(whitelistTable, 'enabled', value)"
          @page="(value) => whitelistTable.page = value"
          @page-size="(value) => updateRulePageSize(whitelistTable, value)"
          @remove="removeWhitelistRule"
        />

        <div class="card">
          <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ kt('records') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ kt('recordsHint') }}</p>
              </div>
              <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="logsLoading" @click="loadLogs">
                <Icon name="refresh" size="sm" :class="logsLoading ? 'animate-spin' : ''" />
                {{ t('common.refresh') }}
              </button>
            </div>
            <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-6">
              <Select v-model="logFilters.match_type" :options="matchTypeOptions" @change="reloadLogsFromFirstPage" />
              <Select v-model="logFilters.group_id" :options="groupFilterOptions" @change="reloadLogsFromFirstPage" />
              <Select v-model="logFilters.endpoint" :options="endpointOptions" @change="reloadLogsFromFirstPage" />
              <input v-model.trim="logFilters.search" type="search" class="input" :placeholder="t('admin.riskControl.filters.search')" @keyup.enter="reloadLogsFromFirstPage" />
              <input v-model="logFilters.from" type="datetime-local" class="input" :title="t('admin.riskControl.filters.from')" @change="reloadLogsFromFirstPage" />
              <input v-model="logFilters.to" type="datetime-local" class="input" :title="t('admin.riskControl.filters.to')" @change="reloadLogsFromFirstPage" />
            </div>
          </div>
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.time') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.user') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.group') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.endpoint') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ kt('match') }}</th>
                  <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.table.input') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
                <tr v-if="logsLoading">
                  <td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</td>
                </tr>
                <tr v-else-if="keywordLogs.length === 0">
                  <td colspan="6" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ kt('emptyLogs') }}</td>
                </tr>
                <template v-else>
                  <tr v-for="row in keywordLogs" :key="row.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(row.created_at) }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.user_email || '-' }}</div>
                      <div class="text-xs text-gray-400">{{ row.api_key_name || '-' }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">{{ row.group_name || '-' }}</td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <div>{{ row.endpoint || '-' }}</div>
                      <div class="text-xs text-gray-400">{{ row.provider || '-' }} / {{ row.model || '-' }}</div>
                    </td>
                    <td class="whitespace-nowrap px-5 py-4 text-sm text-gray-700 dark:text-gray-300">
                      <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="row.match_type === 'regex' ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300' : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'">
                        {{ row.match_type === 'regex' ? kt('regex') : kt('keyword') }}
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
          <Pagination
            v-if="logPagination.total > 0"
            :page="logPagination.page"
            :total="logPagination.total"
            :page-size="logPagination.page_size"
            @update:page="onLogPageChange"
            @update:pageSize="onLogPageSizeChange"
          />
        </div>
      </template>

      <BaseDialog :show="testOpen" :title="kt('testInput')" width="wide" @close="testOpen = false">
        <div class="space-y-4">
          <textarea v-model="keywordTestText" class="input min-h-32 resize-y" :placeholder="kt('testPlaceholder')"></textarea>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="keywordTesting" @click="testKeywordFilter">
              <Icon name="beaker" size="sm" :class="keywordTesting ? 'animate-pulse' : ''" />
              {{ kt('test') }}
            </button>
            <span v-if="keywordTestResult" class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="keywordTestResult.blocked ? 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300' : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'">
              {{ keywordTestResult.blocked ? kt('testBlocked') : kt('testPassed') }}
            </span>
          </div>
          <div v-if="keywordTestResult?.blocked" class="grid grid-cols-1 gap-2 text-sm text-gray-500 dark:text-gray-400 sm:grid-cols-2">
            <div>{{ kt('testRule') }}: {{ keywordTestResult.rule_id || keywordTestResult.rule_name || '-' }}</div>
            <div>{{ kt('testSegment') }}: #{{ keywordTestResult.segment_index ?? 0 }}</div>
            <div>{{ kt('match') }}: {{ keywordTestResult.match_type }} / {{ matchModeLabel(keywordTestResult.resolved_match_mode) }} / {{ keywordTestResult.matched_text }}</div>
            <div class="sm:col-span-2">{{ kt('testExcerpt') }}: {{ keywordTestResult.segment_text || keywordTestResult.matched_text || '-' }}</div>
          </div>
        </div>
        <template #footer>
          <button type="button" class="btn btn-secondary" @click="testOpen = false">{{ t('common.close') }}</button>
        </template>
      </BaseDialog>

      <BaseDialog :show="importOpen" :title="t('admin.keywordFilter.importTitle')" width="wide" @close="importOpen = false">
        <div class="space-y-4">
          <div class="rounded-lg border border-dashed border-gray-200 bg-gray-50 p-5 dark:border-dark-700 dark:bg-dark-900/30">
            <label class="btn btn-secondary inline-flex cursor-pointer items-center gap-2">
              <Icon name="upload" size="sm" />
              {{ t('admin.keywordFilter.chooseFile') }}
              <input type="file" accept=".csv,.json,application/json,text/csv" class="sr-only" @change="handleImportFile" />
            </label>
            <p class="mt-3 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.keywordFilter.importHint') }}</p>
          </div>
          <div v-if="importSummary" class="rounded-lg border border-gray-100 p-4 text-sm dark:border-dark-700">
            <p class="font-semibold text-gray-900 dark:text-white">{{ importSummary.message }}</p>
            <ul v-if="importSummary.errors.length > 0" class="mt-2 max-h-48 list-disc space-y-1 overflow-y-auto pl-5 text-amber-700 dark:text-amber-300">
              <li v-for="(error, index) in importSummary.errors" :key="index">{{ error }}</li>
            </ul>
          </div>
        </div>
        <template #footer>
          <button type="button" class="btn btn-secondary" @click="importOpen = false">{{ t('common.close') }}</button>
        </template>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Pagination from '@/components/common/Pagination.vue'
import { adminAPI } from '@/api/admin'
import type {
  KeywordFilterConfig,
  KeywordFilterLog,
  KeywordFilterMatchMode,
  KeywordFilterRegexRule,
  KeywordFilterRule,
  KeywordFilterTestResponse,
  KeywordFilterWhitelistRule,
  UpdateKeywordFilterConfig,
} from '@/api/admin/riskControl'
import type { AdminGroup, SelectOption } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime as formatDateTimeValue } from '@/utils/format'

interface RuleTableProps {
  kind: RuleKind
  title: string
  emptyText: string
  rows: Array<KeywordFilterRule | KeywordFilterWhitelistRule>
  total: number
  page: number
  pageSize: number
  search: string
  mode: KeywordFilterMatchMode | 'all'
  enabledFilter: EnabledFilter
  matchModeOptions: SelectOption[]
  targetOptions?: SelectOption[]
  refreshing?: boolean
}

type RuleKind = 'keyword' | 'whitelist'
type EnabledFilter = 'all' | 'enabled' | 'disabled'
type RuleTableState = {
  search: string
  mode: KeywordFilterMatchMode | 'all'
  enabled: EnabledFilter
  page: number
  pageSize: number
}
type ImportSummary = {
  message: string
  errors: string[]
}

const defaultKeywordFilterBlockMessage = '输入内容命中关键词过滤规则，请调整后重试'
const maxRulePatternLength = 256
const maxRulesPerKind = 1000
const selectedSegmentClass = 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white'
const mutedSegmentClass = 'text-gray-500 dark:text-gray-400'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(true)
const refreshing = ref(false)
const rulesRefreshing = ref(false)
const saving = ref(false)
const logsLoading = ref(false)
const groupsLoading = ref(false)
const groupsLoadFailed = ref(false)
const testOpen = ref(false)
const importOpen = ref(false)
const importKind = ref<RuleKind>('keyword')
const keywordTesting = ref(false)
const keywordTestText = ref('')
const keywordTestResult = ref<KeywordFilterTestResponse | null>(null)
const importSummary = ref<ImportSummary | null>(null)
const groups = ref<AdminGroup[]>([])
const keywordLogs = ref<KeywordFilterLog[]>([])

const keywordForm = reactive({
  enabled: false,
  all_groups: true,
  group_ids: [] as number[],
  keyword_rules: [] as KeywordFilterRule[],
  whitelist_rules: [] as KeywordFilterWhitelistRule[],
  regex_rules: [] as KeywordFilterRegexRule[],
  block_status: 403,
  block_message: defaultKeywordFilterBlockMessage,
  hit_retention_days: 180,
})

const keywordTable = reactive<RuleTableState>({
  search: '',
  mode: 'all',
  enabled: 'all',
  page: 1,
  pageSize: 20,
})

const whitelistTable = reactive<RuleTableState>({
  search: '',
  mode: 'all',
  enabled: 'all',
  page: 1,
  pageSize: 20,
})

const logPagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 1,
})

const logFilters = reactive({
  match_type: '',
  group_id: 0,
  endpoint: '',
  search: '',
  from: '',
  to: '',
})

const keywordMatchModeOptions = computed<SelectOption[]>(() => [
  { value: 'auto', label: kt('matchModeAuto') },
  { value: 'contains', label: kt('matchModeContains') },
  { value: 'fuzzy', label: kt('matchModeFuzzy') },
  { value: 'token', label: kt('matchModeToken') },
  { value: 'exact_phrase', label: kt('matchModeExactPhrase') },
  { value: 'cjk_token', label: kt('matchModeCJKToken') },
])

const keywordTargetOptions = computed<SelectOption[]>(() => [
  { value: '', label: kt('whitelistTargetsAll') },
  ...keywordForm.keyword_rules.map((rule) => ({
    value: rule.id,
    label: rule.pattern || rule.id,
  })),
])

const matchTypeOptions = computed<SelectOption[]>(() => [
  { value: '', label: kt('resultAll') },
  { value: 'keyword', label: kt('keyword') },
  { value: 'regex', label: kt('regex') },
])

const groupFilterOptions = computed<SelectOption[]>(() => [
  { value: 0, label: t('admin.riskControl.filters.allGroups') },
  ...groups.value.map((group) => ({
    value: group.id,
    label: `${group.name} (${group.platform})`,
  })),
])

const endpointOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.riskControl.filters.allEndpoints') },
  { value: '/v1/messages', label: '/v1/messages' },
  { value: '/v1/responses', label: '/v1/responses' },
  { value: '/v1/chat/completions', label: '/v1/chat/completions' },
  { value: '/v1beta/models', label: '/v1beta/models' },
])

const keywordGroupScopeInvalid = computed(() => !keywordForm.all_groups && keywordForm.group_ids.length === 0)

const groupStateText = computed(() => {
  if (groupsLoading.value) return t('common.loading')
  if (groupsLoadFailed.value) return kt('groupsLoadFailed')
  return t('admin.riskControl.noGroups')
})

const filteredKeywordRules = computed(() => filterRules(keywordForm.keyword_rules, keywordTable))
const filteredWhitelistRules = computed(() => filterRules(keywordForm.whitelist_rules, whitelistTable))
const keywordPageRows = computed(() => paginateRows(filteredKeywordRules.value, keywordTable))
const whitelistPageRows = computed(() => paginateRows(filteredWhitelistRules.value, whitelistTable))

const RuleTable = defineComponent({
  name: 'KeywordRuleTable',
  props: ['kind', 'title', 'emptyText', 'rows', 'total', 'page', 'pageSize', 'search', 'mode', 'enabledFilter', 'matchModeOptions', 'targetOptions', 'refreshing'],
  emits: ['add', 'import', 'refresh', 'search', 'mode', 'enabled', 'page', 'page-size', 'remove'],
  setup(rawProps, { emit }) {
    const props = rawProps as unknown as RuleTableProps
    const { t } = useI18n()
    const allModeOptions = computed<SelectOption[]>(() => [
      { value: 'all', label: t('admin.keywordFilter.allMatchModes') },
      ...props.matchModeOptions,
    ])
    const enabledOptions = computed<SelectOption[]>(() => [
      { value: 'all', label: t('admin.keywordFilter.allStatus') },
      { value: 'enabled', label: t('admin.keywordFilter.enabledOnly') },
      { value: 'disabled', label: t('admin.keywordFilter.disabledOnly') },
    ])

    return () => h('div', { class: 'card' }, [
      h('div', { class: 'flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700' }, [
        h('div', { class: 'flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between' }, [
          h('div', [
            h('h2', { class: 'text-lg font-semibold text-gray-900 dark:text-white' }, props.title),
            h('p', { class: 'mt-1 text-sm text-gray-500 dark:text-gray-400' }, t('admin.keywordFilter.ruleCount', { count: props.total })),
          ]),
          h('div', { class: 'flex flex-wrap items-center gap-2' }, [
            h('button', { type: 'button', class: 'btn btn-secondary inline-flex items-center gap-2', onClick: () => emit('import') }, [
              h(Icon, { name: 'upload', size: 'sm' }),
              t('admin.keywordFilter.importRules'),
            ]),
            h('button', { type: 'button', class: 'btn btn-secondary inline-flex items-center gap-2', onClick: () => emit('add') }, [
              h(Icon, { name: 'plus', size: 'sm' }),
              props.kind === 'keyword' ? t('admin.keywordFilter.addKeywordRule') : t('admin.keywordFilter.addWhitelistRule'),
            ]),
            h('button', { type: 'button', disabled: props.refreshing, class: 'btn btn-secondary inline-flex items-center gap-2', onClick: () => emit('refresh') }, [
              h(Icon, { name: 'refresh', size: 'sm', class: props.refreshing ? 'animate-spin' : '' }),
              t('common.refresh'),
            ]),
          ]),
        ]),
        h('div', { class: 'grid grid-cols-1 gap-3 md:grid-cols-3' }, [
          h('input', {
            value: props.search,
            type: 'search',
            class: 'input',
            placeholder: t('admin.keywordFilter.searchRules'),
            onInput: (event: Event) => emit('search', (event.target as HTMLInputElement).value),
          }),
          h(Select, {
            modelValue: props.mode,
            options: allModeOptions.value,
            'onUpdate:modelValue': (value: string | number | boolean | null) => emit('mode', String(value || 'all')),
          }),
          h(Select, {
            modelValue: props.enabledFilter,
            options: enabledOptions.value,
            'onUpdate:modelValue': (value: string | number | boolean | null) => emit('enabled', String(value || 'all')),
          }),
        ]),
      ]),
      h('div', { class: 'overflow-x-auto' }, [
        h('table', { class: 'min-w-full divide-y divide-gray-200 dark:divide-dark-700' }, [
          h('thead', { class: 'bg-gray-50 dark:bg-dark-800' }, [
            h('tr', [
              h('th', { class: 'px-3 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400' }, t('admin.keywordFilter.index')),
              h('th', { class: 'px-3 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400' }, t('admin.keywordFilter.pattern')),
              h('th', { class: 'px-3 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400' }, t('admin.keywordFilter.matchMode')),
              props.kind === 'whitelist'
                ? h('th', { class: 'px-3 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400' }, t('admin.keywordFilter.targetRules'))
                : null,
              h('th', { class: 'px-3 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400' }, t('admin.keywordFilter.enabledShort')),
              h('th', { class: 'px-3 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400' }, t('common.actions')),
            ]),
          ]),
          h('tbody', { class: 'divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800' }, props.rows.length === 0
            ? [h('tr', [h('td', { colspan: props.kind === 'whitelist' ? 6 : 5, class: 'px-3 py-12 text-center text-sm text-gray-500 dark:text-gray-400' }, props.emptyText)])]
            : props.rows.map((rule, index) => h('tr', { key: rule.id || index, class: 'hover:bg-gray-50 dark:hover:bg-dark-700/60' }, [
              h('td', { class: 'whitespace-nowrap px-3 py-3 text-sm text-gray-500 dark:text-gray-400' }, String((props.page - 1) * props.pageSize + index + 1)),
              h('td', { class: 'min-w-[260px] px-3 py-3' }, [
                h('input', {
                  value: rule.pattern,
                  type: 'text',
                  class: 'input h-9',
                  placeholder: t('admin.keywordFilter.pattern'),
                  onInput: (event: Event) => { rule.pattern = (event.target as HTMLInputElement).value },
                }),
              ]),
              h('td', { class: 'min-w-[180px] px-3 py-3' }, [
                h(Select, {
                  modelValue: rule.match_mode,
                  options: props.matchModeOptions,
                  'onUpdate:modelValue': (value: string | number | boolean | null) => { rule.match_mode = normalizeMatchMode(String(value || 'auto')) },
                }),
              ]),
              props.kind === 'whitelist'
                ? h('td', { class: 'min-w-[240px] px-3 py-3' }, [
                  h(Select, {
                    modelValue: ((rule as KeywordFilterWhitelistRule).target_rule_ids[0] || ''),
                    options: props.targetOptions || [],
                    searchable: true,
                    'onUpdate:modelValue': (value: string | number | boolean | null) => {
                      const target = String(value || '')
                      ;(rule as KeywordFilterWhitelistRule).target_rule_ids = target ? [target] : []
                    },
                  }),
                ])
                : null,
              h('td', { class: 'whitespace-nowrap px-3 py-3' }, [
                h(Toggle, {
                  modelValue: rule.enabled,
                  'onUpdate:modelValue': (value: boolean) => { rule.enabled = value },
                }),
              ]),
              h('td', { class: 'whitespace-nowrap px-3 py-3' }, [
                h('button', {
                  type: 'button',
                  class: 'inline-flex h-9 w-9 items-center justify-center rounded-md text-gray-400 hover:bg-gray-100 hover:text-red-600 dark:hover:bg-dark-700',
                  onClick: () => emit('remove', rule.id),
                }, [h(Icon, { name: 'trash', size: 'sm' })]),
              ]),
            ]))),
        ]),
      ]),
      props.total > 0
        ? h(Pagination, {
          page: props.page,
          total: props.total,
          pageSize: props.pageSize,
          'onUpdate:page': (value: number) => emit('page', value),
          'onUpdate:pageSize': (value: number) => emit('page-size', value),
        })
        : null,
    ])
  },
})

function applyKeywordConfig(config: KeywordFilterConfig) {
  keywordForm.enabled = config.enabled
  keywordForm.all_groups = config.all_groups
  keywordForm.group_ids = Array.isArray(config.group_ids) ? [...config.group_ids] : []
  keywordForm.keyword_rules = normalizeKeywordRules(config.keyword_rules || [], config.keywords || [])
  keywordForm.whitelist_rules = normalizeWhitelistRules(config.whitelist_rules || [], config.whitelist || [])
  keywordForm.regex_rules = Array.isArray(config.regex_rules) ? config.regex_rules.map((rule) => ({ ...rule })) : []
  keywordForm.block_status = config.block_status || 403
  keywordForm.block_message = config.block_message || defaultKeywordFilterBlockMessage
  keywordForm.hit_retention_days = config.hit_retention_days || 180
  cleanupKeywordWhitelistTargets()
}

async function loadAll(silent = true) {
  loading.value = true
  try {
    const [config] = await Promise.all([
      adminAPI.riskControl.getKeywordFilterConfig(),
      loadGroups({ silent: true }),
    ])
    applyKeywordConfig(config)
    await loadLogs()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.keywordFilter.loadFailed')))
  } finally {
    loading.value = false
    if (!silent) appStore.showSuccess(t('common.success'))
  }
}

async function refreshAll() {
  refreshing.value = true
  try {
    const [config] = await Promise.all([
      adminAPI.riskControl.getKeywordFilterConfig(),
      loadGroups({ silent: true }),
    ])
    applyKeywordConfig(config)
    await loadLogs()
    appStore.showSuccess(t('common.success'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.keywordFilter.loadFailed')))
  } finally {
    refreshing.value = false
  }
}

async function refreshRules() {
  rulesRefreshing.value = true
  try {
    const [config] = await Promise.all([
      adminAPI.riskControl.getKeywordFilterConfig(),
      loadGroups({ silent: true }),
    ])
    applyKeywordConfig(config)
    appStore.showSuccess(t('common.success'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.keywordFilter.loadFailed')))
  } finally {
    rulesRefreshing.value = false
  }
}

async function loadGroups(options: { silent?: boolean } = {}) {
  groupsLoading.value = true
  groupsLoadFailed.value = false
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch (err: unknown) {
    groupsLoadFailed.value = true
    if (!options.silent) {
      appStore.showError(extractApiErrorMessage(err, kt('groupsLoadFailed')))
    }
  } finally {
    groupsLoading.value = false
  }
}

async function loadLogs() {
  logsLoading.value = true
  try {
    const result = await adminAPI.riskControl.listKeywordFilterLogs({
      page: logPagination.page,
      page_size: logPagination.page_size,
      match_type: logFilters.match_type || undefined,
      group_id: logFilters.group_id || undefined,
      endpoint: logFilters.endpoint || undefined,
      search: logFilters.search || undefined,
      from: normalizeDateTimeLocal(logFilters.from),
      to: normalizeDateTimeLocal(logFilters.to),
    })
    keywordLogs.value = result.items
    logPagination.total = result.total
    logPagination.page = result.page
    logPagination.page_size = result.page_size
    logPagination.pages = result.pages
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, kt('logsFailed')))
  } finally {
    logsLoading.value = false
  }
}

function buildKeywordPayload(): UpdateKeywordFilterConfig {
  const keywordRules = sanitizeKeywordRules(keywordForm.keyword_rules)
  const whitelistRules = pruneWhitelistTargets(
    sanitizeWhitelistRules(keywordForm.whitelist_rules),
    keywordRules,
  )
  keywordForm.keyword_rules = keywordRules
  keywordForm.whitelist_rules = whitelistRules
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

async function saveConfig() {
  if (keywordGroupScopeInvalid.value) {
    appStore.showError(kt('groupScopeEmptyHint'))
    return
  }
  saving.value = true
  try {
    const updated = await adminAPI.riskControl.updateKeywordFilterConfig(buildKeywordPayload())
    applyKeywordConfig(updated)
    appStore.showSuccess(kt('saved'))
    await loadLogs()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, kt('saveFailed')))
  } finally {
    saving.value = false
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
    appStore.showError(extractApiErrorMessage(err, kt('testFailed')))
  } finally {
    keywordTesting.value = false
  }
}

function addKeywordRegexRule() {
  keywordForm.regex_rules.push({ name: '', pattern: '', enabled: false })
}

function removeKeywordRegexRule(index: number) {
  keywordForm.regex_rules.splice(index, 1)
}

function addKeywordRule() {
  keywordForm.keyword_rules.unshift(createKeywordRule('', 'keyword'))
  keywordTable.page = 1
}

function addWhitelistRule() {
  keywordForm.whitelist_rules.unshift(createWhitelistRule(''))
  whitelistTable.page = 1
}

function removeKeywordRule(ruleID: string) {
  keywordForm.keyword_rules = keywordForm.keyword_rules.filter((rule) => rule.id !== ruleID)
  cleanupKeywordWhitelistTargets()
}

function removeWhitelistRule(ruleID: string) {
  keywordForm.whitelist_rules = keywordForm.whitelist_rules.filter((rule) => rule.id !== ruleID)
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

function updateRuleFilter<T extends keyof RuleTableState>(state: RuleTableState, key: T, value: RuleTableState[T]) {
  state[key] = value
  state.page = 1
}

function updateRulePageSize(state: RuleTableState, value: number) {
  state.page = 1
  state.pageSize = value
}

function filterRules<T extends KeywordFilterRule | KeywordFilterWhitelistRule>(rules: T[], state: RuleTableState): T[] {
  const search = state.search.trim().toLowerCase()
  return rules.filter((rule) => {
    if (search && !rule.pattern.toLowerCase().includes(search)) return false
    if (state.mode !== 'all' && rule.match_mode !== state.mode) return false
    if (state.enabled === 'enabled' && !rule.enabled) return false
    if (state.enabled === 'disabled' && rule.enabled) return false
    return true
  })
}

function paginateRows<T>(rows: T[], state: RuleTableState): T[] {
  const start = (state.page - 1) * state.pageSize
  return rows.slice(start, start + state.pageSize)
}

function reloadLogsFromFirstPage() {
  logPagination.page = 1
  void loadLogs()
}

function onLogPageChange(page: number) {
  logPagination.page = page
  void loadLogs()
}

function onLogPageSizeChange(pageSize: number) {
  logPagination.page = 1
  logPagination.page_size = pageSize
  void loadLogs()
}

function openImport(kind: RuleKind) {
  importKind.value = kind
  importSummary.value = null
  importOpen.value = true
}

async function handleImportFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  try {
    const text = await file.text()
    const result = file.name.toLowerCase().endsWith('.json')
      ? importJSON(text)
      : importCSV(text)
    importSummary.value = result
    appStore.showSuccess(result.message)
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : t('admin.keywordFilter.importFailed')
    importSummary.value = { message, errors: [message] }
    appStore.showError(message)
  }
}

function importCSV(text: string): ImportSummary {
  const rows = parseCSV(text)
  if (rows.length === 0) return { message: t('admin.keywordFilter.importEmpty'), errors: [] }
  const headers = rows[0].map((item) => item.trim().toLowerCase())
  const hasHeader = headers.includes('pattern') || headers.includes('type') || headers.includes('match_mode')
  const dataRows = hasHeader ? rows.slice(1) : rows
  const errors: string[] = []
  const imported: ImportedRule[] = []

  dataRows.forEach((row, index) => {
    const line = hasHeader ? index + 2 : index + 1
    if (hasHeader) {
      const value = (name: string) => row[headers.indexOf(name)] || ''
      imported.push({
        type: normalizeImportType(value('type'), 'keyword'),
        pattern: value('pattern'),
        match_mode: normalizeMatchMode(value('match_mode')),
        raw_match_mode: value('match_mode'),
        enabled: parseImportEnabled(value('enabled')),
        target_patterns: splitTargetPatterns(value('target_patterns')),
        line,
      })
    } else {
      imported.push({
        type: 'keyword',
        pattern: row[0] || '',
        match_mode: 'auto',
        enabled: true,
        target_patterns: [],
        line,
      })
    }
  })

  return mergeImportedRules(imported, errors)
}

function importJSON(text: string): ImportSummary {
  const parsed = JSON.parse(text) as unknown
  const imported: ImportedRule[] = []
  const errors: string[] = []
  if (Array.isArray(parsed)) {
    parsed.forEach((value, index) => {
      imported.push({ type: 'keyword', pattern: String(value || ''), match_mode: 'auto', enabled: true, target_patterns: [], line: index + 1 })
    })
  } else if (parsed && typeof parsed === 'object') {
    const obj = parsed as {
      keywords?: unknown[]
      whitelist?: unknown[]
      keyword_rules?: Partial<KeywordFilterRule>[]
      whitelist_rules?: Partial<KeywordFilterWhitelistRule>[]
    }
    if (Array.isArray(obj.keyword_rules) || Array.isArray(obj.whitelist_rules)) {
      ;(obj.keyword_rules || []).forEach((rule, index) => {
        imported.push({
          type: 'keyword',
          pattern: String(rule.pattern || ''),
          match_mode: normalizeMatchMode(rule.match_mode),
          raw_match_mode: rule.match_mode,
          enabled: rule.enabled ?? true,
          target_patterns: [],
          id: rule.id,
          line: index + 1,
        })
      })
      ;(obj.whitelist_rules || []).forEach((rule, index) => {
        imported.push({
          type: 'whitelist',
          pattern: String(rule.pattern || ''),
          match_mode: normalizeMatchMode(rule.match_mode),
          raw_match_mode: rule.match_mode,
          enabled: rule.enabled ?? true,
          target_rule_ids: Array.isArray(rule.target_rule_ids) ? [...rule.target_rule_ids] : [],
          target_patterns: [],
          id: rule.id,
          line: index + 1,
        })
      })
    } else {
      ;(obj.keywords || []).forEach((value, index) => {
        imported.push({ type: 'keyword', pattern: String(value || ''), match_mode: 'auto', enabled: true, target_patterns: [], line: index + 1 })
      })
      ;(obj.whitelist || []).forEach((value, index) => {
        imported.push({ type: 'whitelist', pattern: String(value || ''), match_mode: 'auto', enabled: true, target_patterns: [], line: index + 1 })
      })
    }
  } else {
    throw new Error(t('admin.keywordFilter.importInvalidJSON'))
  }
  return mergeImportedRules(imported, errors)
}

type ImportedRule = {
  type: RuleKind
  pattern: string
  match_mode: KeywordFilterMatchMode
  raw_match_mode?: string
  enabled: boolean
  target_patterns: string[]
  target_rule_ids?: string[]
  id?: string
  line: number
}

function mergeImportedRules(items: ImportedRule[], errors: string[]): ImportSummary {
  let addedKeyword = 0
  let addedWhitelist = 0
  let skipped = 0
  const keywordByPattern = new Map(keywordForm.keyword_rules.map((rule) => [normalizeKeywordPattern(rule.pattern), rule]))
  const whitelistByPattern = new Map(keywordForm.whitelist_rules.map((rule) => [normalizeKeywordPattern(rule.pattern), rule]))

  for (const item of items) {
    const pattern = normalizeKeywordPattern(item.pattern)
    if (!pattern) {
      skipped += 1
      errors.push(t('admin.keywordFilter.importErrorEmpty', { line: item.line }))
      continue
    }
    if (pattern.length > maxRulePatternLength) {
      skipped += 1
      errors.push(t('admin.keywordFilter.importErrorTooLong', { line: item.line }))
      continue
    }
    if (item.raw_match_mode && !isValidMatchMode(item.raw_match_mode)) {
      skipped += 1
      errors.push(t('admin.keywordFilter.importErrorMode', { line: item.line }))
      continue
    }
    if (item.type === 'keyword') {
      if (keywordByPattern.has(pattern) || keywordForm.keyword_rules.length >= maxRulesPerKind) {
        skipped += 1
        continue
      }
      const rule = normalizeKeywordRule({
        id: item.id || '',
        pattern,
        match_mode: item.match_mode,
        enabled: item.enabled,
        action: 'block',
      }, keywordForm.keyword_rules.length)
      keywordForm.keyword_rules.push(rule)
      keywordByPattern.set(pattern, rule)
      addedKeyword += 1
    } else {
      if (whitelistByPattern.has(pattern) || keywordForm.whitelist_rules.length >= maxRulesPerKind) {
        skipped += 1
        continue
      }
      const targetIDs = item.target_rule_ids?.length
        ? item.target_rule_ids
        : item.target_patterns
          .map((target) => keywordByPattern.get(normalizeKeywordPattern(target))?.id || '')
          .filter(Boolean)
      const rule = normalizeWhitelistRule({
        id: item.id || '',
        pattern,
        match_mode: item.match_mode,
        target_rule_ids: targetIDs,
        enabled: item.enabled,
      }, keywordForm.whitelist_rules.length)
      keywordForm.whitelist_rules.push(rule)
      whitelistByPattern.set(pattern, rule)
      addedWhitelist += 1
    }
  }
  cleanupKeywordWhitelistTargets()
  return {
    message: t('admin.keywordFilter.importSummary', { keyword: addedKeyword, whitelist: addedWhitelist, skipped }),
    errors,
  }
}

function parseCSV(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let field = ''
  let inQuotes = false
  for (let i = 0; i < text.length; i += 1) {
    const char = text[i]
    const next = text[i + 1]
    if (char === '"') {
      if (inQuotes && next === '"') {
        field += '"'
        i += 1
      } else {
        inQuotes = !inQuotes
      }
    } else if (char === ',' && !inQuotes) {
      row.push(field.trim())
      field = ''
    } else if ((char === '\n' || char === '\r') && !inQuotes) {
      if (char === '\r' && next === '\n') i += 1
      row.push(field.trim())
      if (row.some((item) => item !== '')) rows.push(row)
      row = []
      field = ''
    } else {
      field += char
    }
  }
  row.push(field.trim())
  if (row.some((item) => item !== '')) rows.push(row)
  return rows
}

function normalizeImportType(value: string, fallback: RuleKind): RuleKind {
  const normalized = value.trim().toLowerCase()
  if (normalized === 'whitelist') return 'whitelist'
  if (normalized === 'keyword') return 'keyword'
  return fallback
}

function parseImportEnabled(value: string): boolean {
  const normalized = value.trim().toLowerCase()
  if (['false', '0', 'disabled', 'off', 'no'].includes(normalized)) return false
  return true
}

function splitTargetPatterns(value: string): string[] {
  return value.split(';').map((item) => item.trim()).filter(Boolean)
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

function normalizeKeywordRule(rule: Partial<KeywordFilterRule>, index: number): KeywordFilterRule {
  const pattern = normalizeKeywordPattern(rule.pattern)
  return {
    id: rule.id?.trim() || createKeywordRuleID('keyword', pattern || String(index), index),
    pattern,
    match_mode: normalizeMatchMode(rule.match_mode),
    enabled: rule.enabled ?? true,
    action: rule.action || 'block',
  }
}

function normalizeWhitelistRule(rule: Partial<KeywordFilterWhitelistRule>, index: number): KeywordFilterWhitelistRule {
  const pattern = normalizeKeywordPattern(rule.pattern)
  return {
    id: rule.id?.trim() || createKeywordRuleID('whitelist', pattern || String(index), index),
    pattern,
    match_mode: normalizeMatchMode(rule.match_mode),
    target_rule_ids: Array.isArray(rule.target_rule_ids) ? [...rule.target_rule_ids] : [],
    enabled: rule.enabled ?? true,
  }
}

function normalizeKeywordRules(rules: KeywordFilterRule[], fallbackKeywords: string[]): KeywordFilterRule[] {
  const source = Array.isArray(rules) && rules.length > 0
    ? rules
    : fallbackKeywords.map((pattern, index) => createKeywordRule(pattern, 'legacy', index))
  return sanitizeKeywordRules(source)
}

function normalizeWhitelistRules(rules: KeywordFilterWhitelistRule[], fallbackWhitelist: string[]): KeywordFilterWhitelistRule[] {
  const source = Array.isArray(rules) && rules.length > 0
    ? rules
    : fallbackWhitelist.map((pattern, index) => createWhitelistRule(pattern, index))
  return sanitizeWhitelistRules(source)
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

function cleanupKeywordWhitelistTargets() {
  keywordForm.whitelist_rules = pruneWhitelistTargets(keywordForm.whitelist_rules, keywordForm.keyword_rules)
}

function normalizeKeywordPattern(pattern: string | undefined): string {
  return (pattern || '').trim()
}

function createKeywordRule(pattern = '', prefix = 'keyword', index = Date.now()): KeywordFilterRule {
  return {
    id: createKeywordRuleID(prefix, pattern || String(index), index),
    pattern,
    match_mode: 'auto',
    enabled: true,
    action: 'block',
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

function isValidMatchMode(mode: string): boolean {
  return ['auto', 'contains', 'fuzzy', 'token', 'exact_phrase', 'cjk_token'].includes(mode)
}

function matchModeLabel(mode: string): string {
  const found = keywordMatchModeOptions.value.find((option) => option.value === mode)
  return found?.label ?? mode
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

function kt(key: string): string {
  return t(`admin.keywordFilter.${key}`)
}

onMounted(() => {
  void loadAll()
})
</script>
