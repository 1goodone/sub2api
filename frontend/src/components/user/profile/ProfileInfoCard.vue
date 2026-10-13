<template>
  <div class="profile-info">
    <section
      data-testid="profile-overview-hero"
      class="profile-overview"
    >
      <div class="profile-overview-main">
        <div class="profile-identity">
          <div class="profile-avatar">
            <img
              v-if="avatarUrl"
              :src="avatarUrl"
              :alt="displayName"
              class="h-full w-full object-cover"
            >
            <span v-else>{{ avatarInitial }}</span>
          </div>

          <div class="profile-identity-copy">
            <div class="profile-name-row">
              <h2 :title="displayName">{{ displayName }}</h2>
              <span class="badge badge-gray">
                {{ user?.role === 'admin' ? t('profile.administrator') : t('profile.user') }}
              </span>
            </div>
            <p class="profile-account-status" :class="{ 'is-disabled': user?.status !== 'active' }">
              <span aria-hidden="true" />
              {{ user?.status === 'active' ? t('common.active') : t('common.disabled') }}
            </p>
            <div v-if="sourceHints.length" class="profile-source-hints">
              <span v-for="hint in sourceHints" :key="hint.key" class="inline-flex items-center gap-1">
                <Icon name="link" size="sm" />
                {{ hint.text }}
              </span>
            </div>
          </div>
        </div>
        <dl class="profile-account-metrics">
          <div data-testid="profile-overview-metric-balance">
            <dt>{{ t('profile.accountBalance') }}</dt>
            <dd>{{ formatCurrency(user?.balance || 0) }}</dd>
          </div>
          <div data-testid="profile-overview-metric-concurrency">
            <dt>{{ t('profile.concurrencyLimit') }}</dt>
            <dd>{{ user?.concurrency || 0 }}</dd>
          </div>
        </dl>
        <button type="button" class="btn btn-primary btn-sm profile-edit-action" data-testid="profile-edit-action" @click="editing = true">
          <Icon name="edit" size="sm" />
          {{ t('common.edit') }}
        </button>
      </div>
      <div class="profile-overview-footer">
        <span v-if="primaryEmailDisplay" class="profile-email" :title="primaryEmailDisplay">
          <Icon name="mail" size="sm" />
          <strong>{{ primaryEmailDisplay }}</strong>
        </span>
        <span data-testid="profile-overview-metric-member-since" class="profile-member-since">
          <Icon name="calendar" size="sm" />
          {{ t('profile.memberSince') }}
          <strong>{{ memberSinceLabel }}</strong>
        </span>
      </div>
    </section>

    <section data-testid="profile-auth-bindings-panel" class="profile-bindings">
      <ProfileIdentityBindingsSection
        :user="user"
        :linuxdo-enabled="linuxdoEnabled"
        :dingtalk-enabled="dingtalkEnabled"
        :oidc-enabled="oidcEnabled"
        :oidc-provider-name="oidcProviderName"
        :wechat-enabled="wechatEnabled"
        :wechat-open-enabled="wechatOpenEnabled"
        :wechat-mp-enabled="wechatMpEnabled"
        compact
      />
    </section>

    <BaseDialog :show="editing" :title="t('profile.editProfile')" width="wide" @close="editing = false">
      <div data-testid="profile-basics-panel" class="profile-edit-grid">
        <ProfileAvatarCard :user="user" embedded />
        <ProfileEditForm :initial-username="user?.username || ''" embedded />
      </div>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ProfileAvatarCard from '@/components/user/profile/ProfileAvatarCard.vue'
import ProfileEditForm from '@/components/user/profile/ProfileEditForm.vue'
import ProfileIdentityBindingsSection from '@/components/user/profile/ProfileIdentityBindingsSection.vue'
import type { User, UserAuthBindingStatus, UserAuthProvider, UserProfileSourceContext } from '@/types'

const props = withDefaults(defineProps<{
  user: User | null
  linuxdoEnabled?: boolean
  dingtalkEnabled?: boolean
  oidcEnabled?: boolean
  oidcProviderName?: string
  wechatEnabled?: boolean
  wechatOpenEnabled?: boolean
  wechatMpEnabled?: boolean
}>(), {
  linuxdoEnabled: false,
  dingtalkEnabled: false,
  oidcEnabled: false,
  oidcProviderName: 'OIDC',
  wechatEnabled: false,
  wechatOpenEnabled: undefined,
  wechatMpEnabled: undefined,
})

const { t } = useI18n()
const editing = ref(false)

function normalizeBindingStatus(binding: boolean | UserAuthBindingStatus | undefined): boolean | null {
  if (typeof binding === 'boolean') {
    return binding
  }
  if (!binding) {
    return null
  }
  if (typeof binding.bound === 'boolean') {
    return binding.bound
  }
  return Boolean(binding.provider_subject || binding.issuer || binding.provider_key)
}

function isEmailBound(user: User | null | undefined): boolean {
  if (typeof user?.email_bound === 'boolean') {
    return user.email_bound
  }

  const nested = user?.auth_bindings?.email ?? user?.identity_bindings?.email
  const normalized = normalizeBindingStatus(nested)
  return normalized ?? false
}

const avatarUrl = computed(() => props.user?.avatar_url?.trim() || '')
const displayName = computed(() => props.user?.username?.trim() || props.user?.email?.trim() || t('profile.user'))
const primaryEmailDisplay = computed(() => {
  const email = props.user?.email?.trim() || ''
  if (!email) {
    return ''
  }
  if (email.endsWith('.invalid') && !isEmailBound(props.user)) {
    return ''
  }
  return email
})
const avatarInitial = computed(() => displayName.value.charAt(0).toUpperCase() || 'U')
const memberSinceLabel = computed(() => {
  const raw = props.user?.created_at?.trim()
  if (!raw) {
    return '-'
  }

  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) {
    return '-'
  }

  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(date)
})

const providerLabels = computed<Record<UserAuthProvider, string>>(() => ({
  email: t('profile.authBindings.providers.email'),
  linuxdo: t('profile.authBindings.providers.linuxdo'),
  dingtalk: t('profile.authBindings.providers.dingtalk'),
  oidc: t('profile.authBindings.providers.oidc', { providerName: props.oidcProviderName }),
  wechat: t('profile.authBindings.providers.wechat'),
  github: 'GitHub',
  google: 'Google'
}))

function formatCurrency(value: number): string {
  return `$${value.toFixed(2)}`
}

function normalizeProvider(value: string): UserAuthProvider | null {
  const normalized = value.trim().toLowerCase()
  if (
    normalized === 'email' ||
    normalized === 'linuxdo' ||
    normalized === 'wechat' ||
    normalized === 'github' ||
    normalized === 'google'
  ) {
    return normalized
  }
  if (normalized === 'oidc' || normalized.startsWith('oidc:') || normalized.startsWith('oidc/')) {
    return 'oidc'
  }
  return null
}

function readObjectString(source: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
  }
  return ''
}

function resolveThirdPartySource(
  rawSource: string | UserProfileSourceContext | null | undefined
): { provider: UserAuthProvider; label: string } | null {
  if (!rawSource) {
    return null
  }

  if (typeof rawSource === 'string') {
    const provider = normalizeProvider(rawSource)
    if (!provider || provider === 'email') {
      return null
    }
    return {
      provider,
      label: providerLabels.value[provider]
    }
  }

  const sourceRecord = rawSource as Record<string, unknown>
  const provider = normalizeProvider(
    readObjectString(sourceRecord, 'provider', 'source', 'provider_type', 'auth_provider')
  )
  if (!provider || provider === 'email') {
    return null
  }

  const explicitLabel = readObjectString(
    sourceRecord,
    'provider_label',
    'label',
    'provider_name',
    'providerName'
  )

  return {
    provider,
    label: explicitLabel || providerLabels.value[provider]
  }
}

const sourceHints = computed(() => {
  const currentUser = props.user
  if (!currentUser) {
    return []
  }

  const hints: Array<{ key: string; text: string }> = []
  const avatarSource = resolveThirdPartySource(
    currentUser.profile_sources?.avatar ?? currentUser.avatar_source
  )
  const usernameSource = resolveThirdPartySource(
    currentUser.profile_sources?.username ??
      currentUser.profile_sources?.display_name ??
      currentUser.profile_sources?.nickname ??
      currentUser.display_name_source ??
      currentUser.username_source ??
      currentUser.nickname_source
  )

  if (avatarSource) {
    hints.push({
      key: 'avatar',
      text: t('profile.authBindings.source.avatar', { providerName: avatarSource.label })
    })
  }

  if (usernameSource) {
    hints.push({
      key: 'username',
      text: t('profile.authBindings.source.username', { providerName: usernameSource.label })
    })
  }

  return hints
})
</script>

<style scoped>
.profile-info { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.profile-overview { overflow: hidden; border: 1px solid var(--console-line); border-radius: var(--console-radius); background: var(--console-surface); box-shadow: var(--console-shadow); }
.profile-overview-main { display: flex; align-items: center; gap: 32px; min-height: 180px; padding: 28px; }
.profile-identity { display: flex; align-items: center; gap: 22px; flex: 1; min-width: 0; }
.profile-avatar { display: grid; place-items: center; flex-shrink: 0; width: 84px; height: 84px; overflow: hidden; border: 1px solid var(--console-line); border-radius: 50%; background: var(--console-accent-soft); color: var(--console-accent); font-size: 28px; font-weight: 600; }
.profile-identity-copy { min-width: 0; }
.profile-name-row { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
.profile-name-row h2 { min-width: 0; overflow-wrap: anywhere; font-size: 22px; line-height: 30px; font-weight: 650; }
.profile-name-row .badge { padding: 2px 7px; border: 1px solid var(--console-line); border-radius: 4px; background: var(--console-raised); font-size: 10px; font-weight: 500; color: var(--console-muted); }
.profile-account-status { display: flex; align-items: center; gap: 6px; margin-top: 8px; color: var(--console-muted); font-size: 12px; }
.profile-account-status > span { width: 5px; height: 5px; border-radius: 50%; background: #269763; }
.profile-account-status.is-disabled > span { background: #dc5252; }
.profile-source-hints { display: flex; flex-wrap: wrap; gap: 6px 14px; margin-top: 10px; font-size: 11px; color: var(--console-muted); }
.profile-account-metrics { display: flex; gap: 32px; margin-left: auto; }
.profile-account-metrics > div + div { padding-left: 32px; border-left: 1px solid var(--console-line); }
.profile-account-metrics dt { font-size: 12px; color: var(--console-muted); }
.profile-account-metrics dd { margin-top: 6px; font-size: 24px; line-height: 32px; font-weight: 600; font-variant-numeric: tabular-nums; }
.profile-edit-action { align-self: flex-start; flex-shrink: 0; }
.profile-overview-footer { display: flex; align-items: center; flex-wrap: wrap; gap: 10px 24px; min-height: 44px; padding: 12px 24px; border-top: 1px solid var(--console-divider); background: var(--console-raised); color: var(--console-muted); font-size: 11px; }
.profile-overview-footer > span { display: flex; align-items: center; gap: 8px; min-width: 0; }
.profile-overview-footer strong { min-width: 0; overflow-wrap: anywhere; color: var(--console-text); font-weight: 550; }
.profile-overview-footer svg { flex-shrink: 0; }
.profile-member-since { margin-left: auto; }
.profile-bindings :deep(.card > :first-child) { padding: 18px 24px; }
.profile-bindings :deep(h2) { font-size: 15px; }
.profile-bindings :deep(h2 + p) { font-size: 12px; }
.profile-edit-grid { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.25fr); gap: 32px; }
.profile-edit-grid > :last-child { padding-left: 32px; border-left: 1px solid var(--console-line); }
@media (max-width: 900px) {
  .profile-overview-main { flex-wrap: wrap; gap: 22px; padding: 24px; }
  .profile-account-metrics { order: 3; width: 100%; margin-left: 106px; }
}
@media (max-width: 600px) {
  .profile-overview-main { gap: 18px 12px; padding: 20px 16px; }
  .profile-identity { gap: 12px; }
  .profile-avatar { width: 56px; height: 56px; font-size: 22px; }
  .profile-name-row h2 { font-size: 18px; line-height: 26px; }
  .profile-name-row { gap: 5px 8px; }
  .profile-account-metrics { margin-left: 0; padding-top: 18px; border-top: 1px solid var(--console-divider); gap: 24px; }
  .profile-account-metrics > div { flex: 1; min-width: 0; }
  .profile-account-metrics > div + div { padding-left: 24px; }
  .profile-account-metrics dd { font-size: 22px; overflow-wrap: anywhere; }
  .profile-overview-footer { padding: 12px 16px; gap: 8px; }
  .profile-overview-footer > span { width: 100%; }
  .profile-member-since { margin-left: 0; }
  .profile-bindings :deep(.card > :first-child) { padding: 16px; }
  .profile-edit-grid { grid-template-columns: minmax(0, 1fr); gap: 24px; }
  .profile-edit-grid > :last-child { padding: 24px 0 0; border-left: 0; border-top: 1px solid var(--console-line); }
}
</style>
