<template>
  <component :is="usageView" v-bind="authStore.isObserver ? { observerMode: true } : {}" />
</template>

<script lang="ts">
import { defineAsyncComponent, type Component } from 'vue'

const loaders = {
  user: () => import('@/views/user/UsageView.vue'),
  observer: () => import('@/views/admin/UsageView.vue'),
}
const views: Record<keyof typeof loaders, Component> = {
  user: defineAsyncComponent(loaders.user),
  observer: defineAsyncComponent(loaders.observer),
}

export async function preloadUsageView(observer: boolean) {
  const role = observer ? 'observer' : 'user'
  views[role] = (await loaders[role]()).default
}
</script>

<script setup lang="ts">
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'

const authStore = useAuthStore()
const usageView = computed(() => views[authStore.isObserver ? 'observer' : 'user'])
</script>
