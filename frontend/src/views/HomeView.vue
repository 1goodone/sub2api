<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="hasHomeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- homeContent is an administrator-managed setting. -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Compact Home Page -->
  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            alt="Logo"
            class="h-9 w-9 shrink-0 rounded-lg object-contain"
          />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="flex h-10 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <router-link
            to="/desktop"
            class="flex h-10 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('home.desktopDownload')"
          >
            <Icon name="download" size="md" />
            <span class="hidden sm:inline">{{ t('home.desktopDownload') }}</span>
          </router-link>
          <button
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <img
          :src="siteLogo || '/logo.svg'"
          alt="Logo"
          class="mx-auto mb-6 h-20 w-20 rounded-2xl object-contain"
        />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-bold md:text-4xl">{{ siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap [overflow-wrap:anywhere] text-base text-gray-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700"
        >
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>

    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 [overflow-wrap:anywhere] sm:px-6 dark:border-dark-800 dark:text-dark-400">
      &copy; {{ currentYear }} {{ siteName }}
    </footer>
  </div>

  <!-- Default Home Page -->
  <div
    v-else
    class="terminal-container relative min-h-screen overflow-hidden bg-[#f7f9fc] text-slate-950 dark:bg-dark-950 dark:text-white"
  >
    <header class="relative z-20 border-b border-slate-200/80 bg-white/95 dark:border-dark-800 dark:bg-dark-950/95">
      <nav
        class="mx-auto flex h-16 max-w-[1320px] items-center justify-between px-4 sm:h-[72px] sm:px-6"
      >
        <router-link to="/" class="flex min-w-0 items-center gap-3">
          <span class="h-9 w-9 shrink-0 overflow-hidden rounded-lg shadow-sm">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </span>
          <span class="truncate text-base font-bold text-slate-900 dark:text-white sm:text-lg">
            {{ siteName }}
          </span>
        </router-link>

        <div class="flex items-center gap-1 sm:gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="hidden rounded-lg p-2 text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-800 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white sm:inline-flex"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <!-- Model Plaza Link -->
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="inline-flex items-center gap-1.5 rounded-lg p-2 text-sm text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <router-link
            to="/desktop"
            class="inline-flex items-center gap-1.5 rounded-lg p-2 text-sm text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('home.desktopDownload')"
          >
            <Icon name="download" size="md" />
            <span class="hidden sm:inline">{{ t('home.desktopDownload') }}</span>
          </router-link>
          <button
            type="button"
            class="rounded-lg p-2 text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-800 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="ml-1 inline-flex h-9 shrink-0 items-center whitespace-nowrap rounded-lg bg-slate-950 px-3 text-sm font-semibold text-white transition-colors hover:bg-primary-600 dark:bg-white dark:text-slate-950 dark:hover:bg-primary-100 sm:px-4"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main>
      <section
        class="home-hero mx-auto grid max-w-[1320px] items-center gap-12 px-6 py-14 lg:grid-cols-12 lg:gap-10 lg:py-12"
      >
        <div class="home-enter lg:col-span-5">
          <div
            class="mb-6 inline-flex items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-semibold text-slate-600 shadow-sm dark:border-dark-700 dark:bg-dark-900 dark:text-dark-300"
          >
            <Icon name="sparkles" size="xs" class="text-primary-500" />
            AI API Gateway
          </div>
          <h1 class="max-w-2xl text-5xl font-bold leading-[1.02] text-slate-950 dark:text-white sm:text-6xl xl:text-[72px]">
            {{ siteName }}
          </h1>
          <p class="mt-5 max-w-xl text-xl font-medium leading-8 text-slate-600 dark:text-dark-300 xl:text-2xl">
            {{ siteSubtitle }}
          </p>
          <p class="mt-3 max-w-xl text-base leading-7 text-slate-500 dark:text-dark-400">
            {{ t('home.heroDescription') }}
          </p>

          <div class="mt-8 flex flex-wrap items-center gap-3">
            <router-link
              :to="isAuthenticated ? dashboardPath : '/login'"
              class="btn btn-primary h-11 px-6"
            >
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
              <Icon name="arrowRight" size="sm" />
            </router-link>
            <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="btn btn-secondary h-11 px-6">
              {{ t('home.viewDocs') }}
            </a>
          </div>

          <div class="mt-9">
            <p class="mb-3 text-xs font-medium text-slate-400">{{ t('home.providers.title') }}</p>
            <div class="flex flex-wrap gap-2">
              <span
                v-for="provider in providers"
                :key="provider.model"
                class="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-600 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-300"
              >
                <span
                  class="inline-flex h-4 w-4 shrink-0 items-center justify-center"
                  :class="{ 'rounded-full bg-white ring-1 ring-slate-200': provider.model === 'gpt-5' }"
                >
                  <Icon
                    v-if="provider.model === 'antigravity'"
                    name="sparkles"
                    size="xs"
                    class="text-violet-500"
                  />
                  <ModelIcon v-else :model="provider.model" size="16px" />
                </span>
                {{ provider.label }}
              </span>
            </div>
          </div>
        </div>

        <div class="home-enter lg:col-span-7 lg:pl-4" style="--home-enter-delay: 90ms">
          <div
            class="home-console overflow-hidden rounded-lg border border-slate-800 bg-[#111318]"
            :aria-label="t('home.heroDescription')"
          >
            <div class="flex h-12 items-center gap-2 border-b border-white/10 px-5 text-xs text-slate-400">
              <Icon name="terminal" size="sm" />
              <span class="font-mono">API request</span>
              <span class="ml-auto inline-flex items-center gap-2 font-mono text-[10px] text-slate-400">
                <span class="home-live-dot h-1.5 w-1.5 rounded-full bg-emerald-400"></span>
                live
              </span>
            </div>
            <div class="min-h-[286px] p-5 font-mono text-[13px] leading-7 sm:p-7 sm:text-sm">
              <p><span class="text-emerald-400">$</span> <span class="text-sky-400">curl</span> <span class="text-violet-300">-X POST</span> <span class="text-primary-400">/v1/messages</span></p>
              <p class="text-slate-500"># Routing to the best available upstream</p>
              <p class="mt-4 text-slate-300">{</p>
              <p class="pl-5 text-slate-300">"model": <span class="text-amber-300">"gpt-5"</span>,</p>
              <p class="pl-5 text-slate-300">"status": <span class="text-emerald-400">"ready"</span>,</p>
              <p class="pl-5 text-slate-300">"latency_ms": <span class="text-sky-300">842</span></p>
              <p class="text-slate-300">}</p>
              <p class="mt-4 inline-flex rounded bg-emerald-400/10 px-2 text-emerald-400">200 OK</p>
            </div>
            <div class="grid grid-cols-3 border-t border-white/10 px-5 py-4 font-mono text-[11px] text-slate-400 sm:px-7">
              <span>{{ t('dashboard.requests') }} <strong class="ml-1 font-medium text-slate-200">1</strong></span>
              <span class="border-x border-white/10 px-4">{{ t('dashboard.model') }} <strong class="ml-1 font-medium text-slate-200">gpt-5</strong></span>
              <span class="pl-4">{{ t('dashboard.averageTime') }} <strong class="ml-1 font-medium text-slate-200">842ms</strong></span>
            </div>
          </div>
        </div>
      </section>

      <section class="border-y border-slate-200/80 bg-white dark:border-dark-800 dark:bg-dark-950">
        <div class="mx-auto max-w-[1320px] px-6 py-16 lg:py-20">
          <div class="mb-10 max-w-2xl">
            <h2 class="text-3xl font-bold text-slate-950 dark:text-white">{{ t('home.solutions.title') }}</h2>
            <p class="mt-3 text-slate-500 dark:text-dark-400">{{ t('home.solutions.subtitle') }}</p>
          </div>

          <div class="home-bento grid gap-3 lg:grid-cols-12">
            <article
              class="home-bento-cell home-bento-gateway overflow-hidden rounded-lg border border-slate-800 bg-slate-950 p-6 text-white lg:col-span-7 lg:row-span-2 lg:p-8"
            >
              <div class="flex items-start justify-between gap-6">
                <div>
                  <div class="mb-5 flex h-10 w-10 items-center justify-center rounded-lg bg-primary-500 text-white">
                    <Icon :name="features[0].icon" size="md" />
                  </div>
                  <h3 class="text-xl font-semibold">{{ features[0].title }}</h3>
                  <p class="mt-2 max-w-xl text-sm leading-6 text-slate-400">{{ features[0].description }}</p>
                </div>
                <span class="hidden font-mono text-xs text-slate-500 sm:block">POST /v1/messages</span>
              </div>

              <div class="mt-9 divide-y divide-white/10 border-y border-white/10 font-mono text-xs">
                <div v-for="provider in providers.slice(0, 3)" :key="provider.model" class="flex items-center gap-3 py-3">
                  <ModelIcon :model="provider.model === 'antigravity' ? 'gemini-2.5-pro' : provider.model" size="18px" />
                  <span class="text-slate-300">{{ provider.label }}</span>
                  <span class="ml-auto text-emerald-400">ready</span>
                </div>
              </div>
            </article>

            <article class="home-bento-cell rounded-lg border border-primary-100 bg-primary-50 p-6 dark:border-primary-900/50 dark:bg-primary-950/30 lg:col-span-5">
              <div class="mb-5 flex h-10 w-10 items-center justify-center rounded-lg bg-white text-primary-600 shadow-sm dark:bg-dark-900 dark:text-primary-300">
                <Icon name="sparkles" size="md" />
              </div>
              <h3 class="text-lg font-semibold text-slate-900 dark:text-white">{{ t('home.providers.title') }}</h3>
              <div class="mt-5 flex flex-wrap gap-3">
                <span v-for="provider in providers" :key="provider.model" class="inline-flex items-center gap-2 text-sm font-medium text-slate-600 dark:text-dark-300">
                  <ModelIcon v-if="provider.model !== 'antigravity'" :model="provider.model" size="18px" />
                  <Icon v-else name="sparkles" size="sm" class="text-primary-500" />
                  {{ provider.label }}
                </span>
              </div>
            </article>

            <article class="home-bento-cell rounded-lg border border-slate-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-900 lg:col-span-5">
              <div class="mb-5 flex h-10 w-10 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
                <Icon :name="features[1].icon" size="md" />
              </div>
              <h3 class="text-lg font-semibold text-slate-900 dark:text-white">{{ features[1].title }}</h3>
              <p class="mt-2 text-sm leading-6 text-slate-500 dark:text-dark-400">{{ features[1].description }}</p>
            </article>

            <article class="home-bento-cell rounded-lg border border-slate-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-900 lg:col-span-7">
              <div class="flex flex-col gap-6 sm:flex-row sm:items-end sm:justify-between">
                <div>
                  <div class="mb-5 flex h-10 w-10 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
                    <Icon :name="features[2].icon" size="md" />
                  </div>
                  <h3 class="text-lg font-semibold text-slate-900 dark:text-white">{{ features[2].title }}</h3>
                  <p class="mt-2 max-w-xl text-sm leading-6 text-slate-500 dark:text-dark-400">{{ features[2].description }}</p>
                </div>
                <span class="shrink-0 font-mono text-sm font-semibold text-primary-600 dark:text-primary-300">{{ t('dashboard.tokens') }}</span>
              </div>
            </article>

            <article class="home-bento-cell rounded-lg border border-slate-200 bg-slate-50 p-6 dark:border-dark-700 dark:bg-dark-900/60 lg:col-span-5">
              <Icon name="book" size="md" class="text-primary-600 dark:text-primary-300" />
              <h3 class="mt-5 text-lg font-semibold text-slate-900 dark:text-white">
                {{ docUrl ? t('home.docs') : 'GitHub' }}
              </h3>
              <a
                :href="docUrl || githubUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="mt-6 inline-flex items-center gap-2 text-sm font-semibold text-primary-600 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200"
              >
                {{ docUrl ? t('home.viewDocs') : 'GitHub' }}
                <Icon name="arrowRight" size="sm" />
              </a>
            </article>
          </div>
        </div>
      </section>

    </main>

    <footer class="border-t border-slate-200/80 bg-white dark:border-dark-800 dark:bg-dark-950">
      <div class="mx-auto flex max-w-[1320px] flex-col gap-4 px-6 py-7 text-sm text-slate-400 sm:flex-row sm:items-center sm:justify-between">
        <p>&copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}</p>
        <div class="flex items-center gap-4">
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="hover:text-slate-700 dark:hover:text-white">{{ t('home.docs') }}</a>
          <router-link to="/desktop" class="hover:text-slate-700 dark:hover:text-white">{{ t('home.desktopDownload') }}</router-link>
          <a :href="githubUrl" target="_blank" rel="noopener noreferrer" class="hover:text-slate-700 dark:hover:text-white">GitHub</a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || t('home.heroSubtitle'))
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isDark = ref(document.documentElement.classList.contains('dark'))
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'
const isAuthenticated = computed(() => authStore.isAuthenticated)
const modelPlazaRequiresAuth = computed(
  () => appStore.cachedPublicSettings?.model_plaza_require_auth === true,
)
const showModelPlazaEntry = computed(
  () => modelPlazaEnabled.value && (isAuthenticated.value || !modelPlazaRequiresAuth.value),
)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const currentYear = computed(() => new Date().getFullYear())

const providers = computed(() => [
  { model: 'claude-sonnet-4', label: t('home.providers.claude') },
  { model: 'gpt-5', label: 'GPT' },
  { model: 'gemini-2.5-pro', label: t('home.providers.gemini') },
  { model: 'antigravity', label: t('home.providers.antigravity') }
])

const features = computed(() => [
  {
    icon: 'server' as const,
    title: t('home.features.unifiedGateway'),
    description: t('home.features.unifiedGatewayDesc')
  },
  {
    icon: 'shield' as const,
    title: t('home.features.multiAccount'),
    description: t('home.features.multiAccountDesc')
  },
  {
    icon: 'chartBar' as const,
    title: t('home.features.balanceQuota'),
    description: t('home.features.balanceQuotaDesc')
  }
])

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()
})
</script>

<style scoped>
.home-hero {
  min-height: min(580px, calc(100dvh - 72px));
}

.home-console {
  box-shadow:
    0 2px 4px rgb(15 23 42 / 0.16),
    0 32px 72px -34px rgb(15 23 42 / 0.62);
}

.home-bento-cell {
  min-height: 210px;
  box-shadow:
    0 1px 2px rgb(15 23 42 / 0.035),
    0 12px 30px -24px rgb(15 23 42 / 0.22);
  transition:
    transform 220ms cubic-bezier(0.16, 1, 0.3, 1),
    border-color 220ms ease,
    box-shadow 220ms ease;
}

.home-bento-cell:hover {
  transform: translateY(-2px);
  box-shadow:
    0 2px 4px rgb(15 23 42 / 0.04),
    0 22px 48px -26px rgb(15 23 42 / 0.34);
}

.home-bento-gateway {
  min-height: 433px;
}

@media (prefers-reduced-motion: no-preference) {
  .home-enter {
    animation: home-enter 520ms cubic-bezier(0.16, 1, 0.3, 1) both;
    animation-delay: var(--home-enter-delay, 0ms);
  }

  .home-live-dot {
    animation: home-status 2.2s ease-in-out infinite;
  }
}

@media (max-width: 1023px) {
  .home-hero {
    min-height: auto;
  }

  .home-bento-gateway {
    min-height: 380px;
  }
}

@media (max-width: 639px) {
  .home-bento-cell,
  .home-bento-gateway {
    min-height: auto;
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-bento-cell {
    transition: none;
  }

  .home-bento-cell:hover {
    transform: none;
  }
}

@keyframes home-enter {
  from {
    opacity: 0;
    transform: translateY(14px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes home-status {
  0%,
  100% {
    opacity: 0.55;
  }
  50% {
    opacity: 1;
  }
}
</style>
