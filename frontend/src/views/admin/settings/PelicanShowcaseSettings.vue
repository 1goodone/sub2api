<template>
  <div class="card" data-testid="pelican-showcase-settings">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 id="settings-section-features-pelican-showcase" tabindex="-1" class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.features.pelicanShowcase.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.features.pelicanShowcase.description') }}
      </p>
      <p class="mt-1.5 text-xs">
        <router-link
          to="/admin/pelican-tests"
          class="inline-flex items-center gap-1 text-primary-600 hover:underline dark:text-primary-400"
        >
          {{ t('admin.settings.features.pelicanShowcase.configureLink') }}
          <span aria-hidden="true">→</span>
        </router-link>
      </p>
    </div>
    <div class="space-y-5 p-6">
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.features.pelicanShowcase.enabled') }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.features.pelicanShowcase.enabledHint') }}
          </p>
        </div>
        <Toggle v-model="enabledModel" data-testid="pelican-showcase-enabled" />
      </div>

      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.features.pelicanShowcase.apiEnabled') }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.features.pelicanShowcase.apiEnabledHint') }}
          </p>
        </div>
        <Toggle v-model="apiEnabledModel" data-testid="pelican-showcase-api-enabled" />
      </div>

      <div v-if="enabled" class="space-y-5 border-t border-gray-100 pt-5 dark:border-dark-700">
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
            <label class="input-label" for="pelican-showcase-max-items">
              {{ t('admin.settings.features.pelicanShowcase.maxItems') }}
            </label>
            <div class="relative">
              <input
                id="pelican-showcase-max-items"
                v-model.number="maxItems"
                type="number"
                min="1"
                :max="PELICAN_SHOWCASE_MAX_ITEMS"
                class="input pr-12"
                data-testid="pelican-showcase-max-items"
              />
              <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-gray-400">
                {{ t('admin.settings.features.pelicanShowcase.maxItemsUnit') }}
              </span>
            </div>
            <p class="mt-1.5 text-xs text-gray-400">
              {{ t('admin.settings.features.pelicanShowcase.maxItemsHint') }}
            </p>
          </div>

          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.pelicanShowcase.autoCleanup') }}
                </p>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.pelicanShowcase.autoCleanupHint') }}
                </p>
              </div>
              <Toggle v-model="autoCleanup" data-testid="pelican-showcase-auto-cleanup" />
            </div>
            <div v-if="config.auto_cleanup" class="mt-3">
              <label class="input-label" for="pelican-showcase-retention-days">
                {{ t('admin.settings.features.pelicanShowcase.retentionDays') }}
              </label>
              <div class="relative">
                <input
                  id="pelican-showcase-retention-days"
                  v-model.number="retentionDays"
                  type="number"
                  min="1"
                  :max="PELICAN_SHOWCASE_MAX_RETENTION_DAYS"
                  class="input pr-12"
                  data-testid="pelican-showcase-retention-days"
                />
                <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-gray-400">
                  {{ t('admin.settings.features.pelicanShowcase.retentionDaysUnit') }}
                </span>
              </div>
              <p class="mt-1.5 text-xs text-gray-400">
                {{ t('admin.settings.features.pelicanShowcase.retentionDaysHint') }}
              </p>
            </div>
          </div>
        </div>

        <p class="rounded-lg bg-gray-50 px-3 py-2 text-xs leading-relaxed text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
          {{ t('admin.settings.features.pelicanShowcase.independentNote') }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import type { PelicanShowcaseConfig } from '@/api/admin/settings'
import { PELICAN_SHOWCASE_MAX_ITEMS, PELICAN_SHOWCASE_MAX_RETENTION_DAYS } from './pelicanShowcase'

const props = defineProps<{
  enabled: boolean
  apiEnabled: boolean
  config: PelicanShowcaseConfig
}>()

const emit = defineEmits<{
  (e: 'update:enabled', value: boolean): void
  (e: 'update:apiEnabled', value: boolean): void
  (e: 'update:config', value: PelicanShowcaseConfig): void
}>()

const { t } = useI18n()

function field<K extends keyof PelicanShowcaseConfig>(key: K) {
  return computed<PelicanShowcaseConfig[K]>({
    get: () => props.config[key],
    set: (value) => emit('update:config', { ...props.config, [key]: value }),
  })
}

const enabledModel = computed({
  get: () => props.enabled,
  set: (value: boolean) => emit('update:enabled', value),
})
const apiEnabledModel = computed({
  get: () => props.apiEnabled,
  set: (value: boolean) => emit('update:apiEnabled', value),
})
const maxItems = field('max_items')
const autoCleanup = field('auto_cleanup')
const retentionDays = field('retention_days')
</script>
