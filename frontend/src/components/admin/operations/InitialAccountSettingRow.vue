<template>
 <div :class="bare ? 'flex items-center justify-between gap-4' : 'flex items-center justify-between gap-4 border-t border-gray-200 pt-4 dark:border-dark-600'">
  <div class="min-w-0">
   <label class="input-label mb-0">{{ title }}</label>
   <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ description }}</p>
  </div>
  <slot />
 </div>
</template>
<script setup lang="ts">
defineProps<{ title: string; description: string; bare?: boolean }>()
</script>
