<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { OpsFeishuDelivery } from '@/api/admin/ops'
defineProps<{ deliveries?: OpsFeishuDelivery[]; showErrors?: boolean }>()
const { t } = useI18n()
</script>

<template>
  <div v-if="deliveries?.length" class="mt-1 space-y-1 text-xs">
    <div v-for="delivery in deliveries" :key="delivery.id" :title="delivery.last_error" class="break-words" :class="delivery.status === 'failed' ? 'text-red-700 dark:text-red-300' : 'text-gray-600 dark:text-gray-300'">
      {{ t('admin.ops.feishu.deliveryTitle') }} · {{ t(`admin.ops.feishu.${delivery.kind}`) }}:
      {{ t(`admin.ops.feishu.${delivery.status}`) }}
      <p v-if="showErrors && delivery.last_error" class="mt-1 whitespace-normal break-words font-normal">{{ delivery.last_error }}</p>
    </div>
  </div>
</template>
