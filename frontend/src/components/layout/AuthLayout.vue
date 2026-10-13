<template>
  <div class="auth-layout min-h-screen bg-[#f5f9fa] text-slate-900 dark:bg-dark-950 dark:text-white lg:grid lg:grid-cols-2">
    <section class="auth-brand-panel relative hidden min-h-screen flex-col overflow-hidden border-r border-primary-200 bg-primary-50 px-12 py-10 dark:border-dark-700 dark:bg-dark-900 lg:flex xl:px-20">
      <router-link to="/home" class="flex min-w-0 items-center gap-3">
        <img :src="siteLogo || '/logo.svg'" alt="" class="h-9 w-9 shrink-0 object-contain" />
        <span class="truncate text-xl font-bold">{{ siteName }}</span>
      </router-link>
      <div class="my-auto py-16">
        <p class="mb-7 font-mono text-xs tracking-[.2em] text-primary-700 dark:text-primary-300">CONNECTED INTELLIGENCE</p>
        <h1 class="text-5xl font-bold leading-tight tracking-tight xl:text-6xl">{{ t('home.orbit.heroLead') }}<br /><span class="text-primary-700 dark:text-primary-300">{{ t('home.orbit.heroAccent') }}</span></h1>
        <p class="mt-7 max-w-md whitespace-pre-line text-base leading-8 text-slate-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <div class="mt-14 space-y-4 border-t border-primary-200 pt-7 text-sm text-slate-600 dark:border-dark-700 dark:text-dark-300">
          <div class="flex items-center gap-3"><Icon name="server" size="sm" /><span>{{ t('home.features.unifiedGateway') }}</span></div>
          <div class="flex items-center gap-3"><Icon name="key" size="sm" /><span>{{ t('home.orbit.step2Title') }}</span></div>
          <div class="flex items-center gap-3"><Icon name="chart" size="sm" /><span>{{ t('home.tags.realtimeBilling') }}</span></div>
        </div>
      </div>
      <p class="text-xs text-slate-500 dark:text-dark-400">&copy; {{ currentYear }} {{ siteName }}</p>
    </section>
    <main class="relative flex min-h-screen items-center justify-center px-6 pb-12 pt-24 sm:px-10">
      <div class="absolute right-6 top-6 flex items-center gap-3">
        <LocaleSwitcher />
        <button type="button" class="rounded-md p-2 text-slate-600 hover:bg-primary-50 dark:text-dark-300 dark:hover:bg-dark-800" :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')" @click="toggleTheme()"><Icon :name="isDark ? 'sun' : 'moon'" size="md" /></button>
      </div>
      <div class="w-full max-w-[420px]">
        <router-link to="/home" class="mb-9 flex min-w-0 items-center justify-center gap-3 lg:hidden"><img :src="siteLogo || '/logo.svg'" alt="" class="h-9 w-9 object-contain" /><span class="truncate text-lg font-bold">{{ siteName }}</span></router-link>
        <slot />
        <div class="mt-7 text-center text-sm"><slot name="footer" /></div>
        <p class="mt-10 text-center text-xs text-slate-500 dark:text-dark-400">&copy; {{ currentYear }} {{ siteName }}</p>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useDark, useToggle } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import { sanitizeUrl } from '@/utils/url'
const appStore = useAppStore()
const { t } = useI18n()
const isDark = useDark({ storageKey: 'theme' })
const toggleTheme = useToggle(isDark)
const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || t('home.heroSubtitle'))
const currentYear = new Date().getFullYear()
onMounted(() => { if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings() })
</script>

<style scoped>
.auth-layout :deep(.input) { box-shadow: none; border-radius: 6px; min-height: 46px; transition: none; }
.auth-layout :deep(.btn) { box-shadow: none; border-radius: 6px; transition: none; transform: none; }
.auth-layout :deep(.btn-primary) { background: #076d79; color: white; }
.dark .auth-layout :deep(.btn-primary) { background: #75d3d9; color: #07141e; }
.auth-layout :deep(:is(button,a,input):focus-visible) { outline: 2px solid #087d89; outline-offset: 3px; }
.dark .auth-layout :deep(:is(button,a,input):focus-visible) { outline-color: #75d3d9; }
</style>
