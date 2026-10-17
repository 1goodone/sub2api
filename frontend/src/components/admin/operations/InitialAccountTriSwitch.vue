<template>
 <div class="flex shrink-0 items-center gap-2">
  <span v-if="value == null" class="text-xs text-gray-400">{{ t('autoConfig.account.unset') }}</span>
  <button v-else type="button" class="text-xs text-gray-500 underline" @click="$emit('change', null)">{{ t('autoConfig.account.unset') }}</button>
  <button type="button" role="switch" :aria-checked="value === true" :data-testid="testId" :class="['relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2', value === true ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600']" @click="$emit('change', value === true ? false : true)">
   <span :class="['pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow transition duration-200', value === true ? 'translate-x-5' : 'translate-x-0']" />
  </button>
 </div>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
defineProps<{ value: boolean | null | undefined; testId: string }>()
defineEmits<{ change: [value: boolean | null] }>()
const { t } = useI18n()
</script>
