<template>
  <component
    :is="isAuthenticated ? AppLayout : 'div'"
    :class="isAuthenticated ? undefined : 'min-h-screen bg-[#f7f9fc] text-slate-950 dark:bg-dark-950 dark:text-white'"
  >
    <header v-if="!isAuthenticated" class="border-b border-slate-200/80 bg-white/95 dark:border-dark-800 dark:bg-dark-950/95">
      <nav class="mx-auto flex h-16 max-w-[1180px] items-center justify-between gap-4 px-4 sm:h-[72px] sm:px-6">
        <router-link to="/" class="flex min-w-0 items-center gap-3">
          <span class="h-9 w-9 shrink-0 overflow-hidden rounded-lg shadow-sm">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </span>
          <span class="truncate text-base font-bold text-slate-900 dark:text-white sm:text-lg">{{ siteName }}</span>
        </router-link>

        <div class="flex shrink-0 items-center gap-2">
          <LocaleSwitcher />
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex h-9 items-center whitespace-nowrap rounded-lg bg-slate-950 px-3 text-sm font-semibold text-white transition-colors hover:bg-primary-600 dark:bg-white dark:text-slate-950 dark:hover:bg-primary-100 sm:px-4"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <component
      :is="isAuthenticated ? 'div' : 'main'"
      class="mx-auto max-w-[1180px] text-slate-950 dark:text-white"
      :class="isAuthenticated ? 'py-2 sm:py-4' : 'px-4 py-10 sm:px-6 sm:py-14'"
    >
      <section class="grid items-center gap-10 lg:grid-cols-[1fr_360px] lg:gap-16">
        <div>
          <div class="mb-5 inline-flex items-center gap-2 rounded-lg border border-primary-200 bg-primary-50 px-3 py-1.5 text-xs font-semibold text-primary-700 dark:border-primary-900/60 dark:bg-primary-950/40 dark:text-primary-300">
            <Icon name="download" size="sm" />
            sub2api desktop
          </div>
          <h1
            class="max-w-2xl font-bold leading-tight text-slate-950 dark:text-white"
            :class="isAuthenticated ? 'text-3xl sm:text-4xl' : 'text-4xl sm:text-5xl'"
          >
            {{ t('desktop.title') }}
          </h1>
          <p class="mt-5 max-w-2xl text-xl font-medium leading-8 text-slate-600 dark:text-dark-300">
            {{ t('desktop.subtitle') }}
          </p>
          <p class="mt-3 max-w-2xl text-base leading-7 text-slate-500 dark:text-dark-400">
            {{ t('desktop.description') }}
          </p>

          <div class="mt-8 flex flex-wrap items-center gap-3">
            <a
              href="#desktop-downloads"
              class="btn btn-primary h-11 px-5"
              data-testid="desktop-latest-release"
            >
              <Icon name="download" size="sm" />
              {{ t('desktop.latestRelease') }}
            </a>
          </div>

          <p class="mt-5 flex max-w-xl items-start gap-2 text-xs leading-5 text-slate-500 dark:text-dark-400">
            <Icon name="infoCircle" size="sm" class="mt-0.5 shrink-0 text-slate-400" />
            <span>{{ t('desktop.releaseNote') }}</span>
          </p>
        </div>

        <div class="relative flex min-h-[300px] items-end justify-center overflow-hidden rounded-2xl border border-primary-100 bg-primary-50/70 px-8 pt-8 dark:border-primary-900/50 dark:bg-primary-950/30">
          <div class="absolute left-6 top-6 h-2 w-2 rounded-full bg-primary-300 dark:bg-primary-700" aria-hidden="true"></div>
          <div class="absolute right-10 top-14 h-3 w-3 rounded-full bg-amber-300 dark:bg-amber-700" aria-hidden="true"></div>
          <img src="/illustrations/xiaoyang-desktop.png" alt="小羊咩咩" class="relative z-10 w-64 max-w-full object-contain drop-shadow-[0_18px_20px_rgba(15,23,42,0.12)]" />
        </div>
      </section>

      <section id="desktop-downloads" class="mt-14 scroll-mt-6 border-t border-slate-200/80 pt-10 dark:border-dark-800">
        <div>
          <p class="text-xs font-semibold text-primary-600 dark:text-primary-300">{{ t('nav.desktop') }}</p>
          <h2 class="mt-2 text-2xl font-bold text-slate-950 dark:text-white">{{ t('desktop.latestRelease') }}</h2>
        </div>

        <div class="mt-6 grid gap-3 md:grid-cols-3">
          <article
            v-for="option in downloadOptions"
            :key="option.key"
            class="flex min-h-[170px] flex-col rounded-lg border border-slate-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-900"
          >
            <div class="flex items-start justify-between gap-3">
              <span class="flex h-10 w-10 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-950/50 dark:text-primary-300">
                <Icon :name="option.icon" size="md" />
              </span>
              <span class="rounded-full bg-emerald-50 px-2 py-1 text-[11px] font-semibold text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">{{ t(option.archKey) }}</span>
            </div>
            <h3 class="mt-5 text-base font-semibold text-slate-900 dark:text-white">{{ t(option.titleKey) }}</h3>
            <p class="mt-1 text-sm text-slate-500 dark:text-dark-400">{{ t(option.detailKey) }}</p>
            <a :href="option.url" class="mt-auto inline-flex h-11 items-center gap-1.5 text-sm font-semibold text-primary-600 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200" :data-testid="`desktop-download-${option.key}`">
              <Icon name="download" size="sm" />
              {{ t('desktop.latestRelease') }}
            </a>
          </article>
        </div>
      </section>

      <section class="mt-14 grid gap-3 border-t border-slate-200/80 pt-10 sm:grid-cols-2 lg:grid-cols-4 dark:border-dark-800">
        <article v-for="feature in features" :key="feature.titleKey" class="border-l-2 border-primary-200 pl-4 dark:border-primary-800">
          <Icon :name="feature.icon" size="md" class="text-primary-600 dark:text-primary-300" />
          <h3 class="mt-4 text-base font-semibold text-slate-900 dark:text-white">{{ t(feature.titleKey) }}</h3>
          <p class="mt-2 text-sm leading-6 text-slate-500 dark:text-dark-400">{{ t(feature.detailKey) }}</p>
        </article>
      </section>
    </component>

    <footer v-if="!isAuthenticated" class="border-t border-slate-200/80 bg-white dark:border-dark-800 dark:bg-dark-950">
      <div class="mx-auto flex max-w-[1180px] flex-col gap-2 px-4 py-6 text-sm text-slate-400 sm:flex-row sm:items-center sm:justify-between sm:px-6">
        <p>&copy; {{ currentYear }} {{ siteName }}</p>
        <router-link to="/" class="font-medium hover:text-slate-700 dark:hover:text-white">{{ siteName }}</router-link>
      </div>
    </footer>
  </component>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'

const AppLayout = defineAsyncComponent(() => import('@/components/layout/AppLayout.vue').then((module) => module.default))

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => (authStore.isAdmin ? '/admin/dashboard' : '/dashboard'))
const currentYear = new Date().getFullYear()

function envUrl(name: string): string {
  const value = import.meta.env[name]
  return typeof value === 'string' ? sanitizeUrl(value) : ''
}

const downloadOptions = computed(() =>
  [
    {
      key: 'windows',
      titleKey: 'desktop.windows',
      detailKey: 'desktop.windowsDetail',
      archKey: 'desktop.windowsArch',
      icon: 'terminal' as const,
      url: envUrl('VITE_DESKTOP_WINDOWS_URL')
    },
    {
      key: 'mac-arm',
      titleKey: 'desktop.macArm',
      detailKey: 'desktop.macArmDetail',
      archKey: 'desktop.macArmArch',
      icon: 'cpu' as const,
      url: envUrl('VITE_DESKTOP_MAC_ARM_URL')
    },
    {
      key: 'mac-intel',
      titleKey: 'desktop.macIntel',
      detailKey: 'desktop.macIntelDetail',
      archKey: 'desktop.macIntelArch',
      icon: 'server' as const,
      url: envUrl('VITE_DESKTOP_MAC_INTEL_URL')
    }
  ].filter((option) => option.url !== '')
)

const features = [
  { icon: 'bell' as const, titleKey: 'desktop.features.balance', detailKey: 'desktop.features.balanceDetail' },
  { icon: 'sync' as const, titleKey: 'desktop.features.setup', detailKey: 'desktop.features.setupDetail' },
  { icon: 'shield' as const, titleKey: 'desktop.features.secure', detailKey: 'desktop.features.secureDetail' },
  { icon: 'cloud' as const, titleKey: 'desktop.features.update', detailKey: 'desktop.features.updateDetail' }
]

onMounted(() => {
  if (!appStore.publicSettingsLoaded) {
    void appStore.fetchPublicSettings()
  }
})
</script>
