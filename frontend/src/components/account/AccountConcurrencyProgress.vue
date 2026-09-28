<template>
  <div v-if="progress" class="mt-1 max-w-64 text-xs text-gray-500" :title="t('autoConfig.history')">
    <div>{{ t('autoConfig.tier', { current: account.concurrency, max: progress.maximum }) }}</div>
    <progress class="h-1 w-full accent-emerald-500" :value="progress.successes" :max="progress.required" />
    <div>{{ t('autoConfig.progress', { count: progress.successes, required: progress.required, next: Math.min(account.concurrency + progress.step, progress.maximum) }) }}</div>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const progress = computed(() => {
 const state = props.account.extra?.auto_config_concurrency as Record<string, unknown> | undefined
 if (!state || state.concurrency !== props.account.concurrency) return null
 const { successes, required, maximum, step } = state
 if (typeof successes !== 'number' || typeof required !== 'number' || typeof maximum !== 'number' || typeof step !== 'number' || required <= 0 || maximum <= 0 || step <= 0) return null
 return { successes, required, maximum, step }
})
</script>
