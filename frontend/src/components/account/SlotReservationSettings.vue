<template>
  <div class="space-y-2 rounded-lg border border-dashed border-gray-300 p-3 dark:border-gray-600">
    <label class="flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-200">
      <input v-model="enabled" data-testid="slot-reservation-enabled" type="checkbox" class="h-4 w-4 rounded" />
      {{ t('admin.accounts.slotReservation.title') }}
    </label>
    <p class="input-hint">{{ t('admin.accounts.slotReservation.hint') }}</p>
    <label v-if="enabled" class="flex items-center gap-3 text-sm text-gray-700 dark:text-gray-200">
      {{ t('admin.accounts.slotReservation.seconds') }}
      <input v-model.number="seconds" data-testid="slot-reservation-seconds" type="number" min="1" max="3600" step="1" required class="input w-28" />
    </label>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const enabled = defineModel<boolean>('enabled', { default: false })
const seconds = defineModel<number>('seconds', { default: 60 })
const { t } = useI18n()
</script>
