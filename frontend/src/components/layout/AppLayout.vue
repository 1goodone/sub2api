<template>
  <div class="app-shell min-h-screen" :class="{ 'sidebar-is-collapsed': sidebarCollapsed }">
    <a class="console-skip" href="#app-main">{{ t('home.orbit.skip') }}</a>
    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div class="app-frame relative min-h-screen min-w-0">
      <!-- Header -->
      <AppHeader />

      <!-- Main Content -->
      <main id="app-main" class="app-main min-w-0" tabindex="-1">
        <div class="w-full">
          <slot />
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const appStore = useAppStore()
const { t } = useI18n()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
