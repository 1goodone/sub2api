<template>
  <div class="auth-layout min-h-screen lg:grid lg:grid-cols-2">
    <section class="auth-brand-panel hidden min-h-screen flex-col overflow-hidden border-r px-12 py-10 lg:flex xl:px-20">
      <router-link to="/home" class="flex min-w-0 items-center gap-3">
        <img :src="siteLogo || '/logo.svg'" alt="" class="h-9 w-9 shrink-0 object-contain" />
        <span class="truncate text-xl font-bold">{{ siteName }}</span>
      </router-link>
      <div class="auth-brand-copy my-auto">
        <p class="mb-6 font-mono text-xs text-[#93a6b3]">CONNECTED INTELLIGENCE</p>
        <h1>{{ siteName }}</h1>
        <p class="auth-tagline">{{ t('home.orbit.heroLead') }}<br />{{ t('home.orbit.heroAccent') }}</p>
        <p class="auth-subtitle">{{ siteSubtitle }}</p>
      </div>
      <footer class="text-xs">&copy; {{ currentYear }} {{ siteName }}</footer>
    </section>
    <main class="auth-main relative flex min-h-screen items-center justify-center px-6 pb-12 pt-24 sm:px-10">
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
