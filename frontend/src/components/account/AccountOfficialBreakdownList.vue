<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ title }}</h3>
    <div v-if="items.length" class="space-y-3">
      <div v-for="item in items" :key="item.label">
        <div class="mb-1 flex items-start justify-between gap-3 text-xs">
          <div class="min-w-0">
            <p class="truncate font-medium text-gray-700 dark:text-gray-200">{{ item.label }}</p>
            <p class="text-gray-400">{{ item.detail }}</p>
          </div>
          <span class="shrink-0 font-semibold text-gray-900 dark:text-white">${{ formatCost(item.value) }}</span>
        </div>
        <div class="h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-600">
          <div class="h-full rounded-full bg-blue-500" :style="{ width: `${Math.min(100, item.share * 100)}%` }"></div>
        </div>
      </div>
    </div>
    <p v-else class="py-6 text-center text-xs text-gray-400">-</p>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  items: Array<{ label: string; value: number; share: number; detail: string }>
}>()

function formatCost(value: number) {
  return value >= 1 ? value.toFixed(2) : value.toFixed(4)
}
</script>
