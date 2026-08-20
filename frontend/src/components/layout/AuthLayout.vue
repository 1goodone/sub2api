<template>
  <div class="grid min-h-screen bg-[#f3f6fb] dark:bg-dark-950 lg:grid-cols-[1.08fr_0.92fr]">
    <section
      class="relative hidden min-h-screen overflow-hidden border-r border-slate-200/70 bg-[#f8fbff] lg:flex lg:flex-col dark:border-dark-800 dark:bg-dark-900"
    >
      <div
        class="pointer-events-none absolute inset-0 bg-mesh-gradient bg-[size:48px_48px] opacity-80 dark:opacity-20"
      ></div>
      <div class="relative z-10 flex items-center gap-3 px-14 py-10">
        <div class="h-10 w-10 overflow-hidden rounded-lg shadow-glow">
          <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
        </div>
        <span class="text-base font-bold text-slate-900 dark:text-white">{{ siteName }}</span>
      </div>

      <div class="relative z-10 my-auto max-w-2xl px-14 pb-20">
        <p class="mb-5 text-sm font-semibold text-primary-600">AI API Gateway</p>
        <h1 class="max-w-xl text-5xl font-bold leading-[1.08] text-slate-950 dark:text-white">
          {{ siteName }}
        </h1>
        <p class="mt-5 max-w-lg text-lg text-slate-500 dark:text-dark-300">
          {{ siteSubtitle }}
        </p>
        <div class="mt-10 grid max-w-md gap-4 text-sm text-slate-600 dark:text-dark-300">
          <div class="flex items-center gap-3">
            <Icon name="swap" size="sm" class="text-primary-500" />
            <span>{{ t('home.tags.subscriptionToApi') }}</span>
          </div>
          <div class="flex items-center gap-3">
            <Icon name="shield" size="sm" class="text-primary-500" />
            <span>{{ t('home.tags.stickySession') }}</span>
          </div>
          <div class="flex items-center gap-3">
            <Icon name="chart" size="sm" class="text-primary-500" />
            <span>{{ t('home.tags.realtimeBilling') }}</span>
          </div>
        </div>
      </div>

      <div class="relative z-10 px-14 py-8 text-xs text-slate-400 dark:text-dark-500">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </section>

    <main class="relative flex min-h-screen items-center justify-center px-4 py-10 sm:px-8">
      <div class="w-full max-w-[500px]">
        <div v-if="settingsLoaded" class="mb-7 flex items-center justify-center gap-3 lg:hidden">
          <div class="h-11 w-11 overflow-hidden rounded-lg shadow-glow">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <span class="text-lg font-bold text-slate-900 dark:text-white">{{ siteName }}</span>
        </div>

        <div
          class="rounded-xl border border-slate-200/80 bg-white p-6 shadow-glass sm:p-8 dark:border-dark-700 dark:bg-dark-900"
        >
          <slot />
        </div>

        <div class="mt-6 text-center text-sm">
          <slot name="footer" />
        </div>

        <div class="mt-8 text-center text-xs text-slate-400 lg:hidden dark:text-dark-500">
          &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'

const appStore = useAppStore()
const { t } = useI18n()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>
