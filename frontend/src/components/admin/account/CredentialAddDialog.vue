<template>
  <div v-if="account" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="close">
    <form class="w-full max-w-md rounded-xl bg-white p-4 shadow-xl dark:bg-dark-800" data-testid="credential-add-dialog" @submit.prevent="submit">
      <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.accounts.loginColumns.addCredential') }}</h2>
      <p class="mt-1 truncate text-xs text-gray-500">{{ account.name }} · #{{ account.id }}</p>
      <label class="mt-3 block text-sm">{{ t('tokenGuardV2.loginEmail') }}<input v-model.trim="email" required type="email" class="input mt-1 w-full" /></label>
      <label class="mt-3 block text-sm">{{ t('tokenGuardV2.password') }}<input v-model="password" required type="password" autocomplete="new-password" class="input mt-1 w-full" /></label>
      <label class="mt-3 block text-sm">{{ t('tokenGuardV2.totpSecret') }}<input v-model.trim="totp" type="password" autocomplete="off" class="input mt-1 w-full" /></label>
      <p v-if="error" class="mt-2 text-sm text-red-600">{{ error }}</p>
      <div class="mt-4 flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" @click="close">{{ t('common.cancel') }}</button>
        <button type="submit" class="btn btn-primary" :disabled="saving">{{ t('common.save') }}</button>
      </div>
    </form>
  </div>
</template>
<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { createTokenGuardV2Account } from '@/api/admin/accountTokenGuardV2'
import type { AccountListItem } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{ account: AccountListItem | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const email = ref('')
const password = ref('')
const totp = ref('')
const saving = ref(false)
const error = ref('')

watch(() => props.account?.id, () => {
  const row = props.account
  email.value = row ? String(row.extra?.email_address || row.extra?.email || row.credentials?.email || row.parent_email || '') : ''
  password.value = ''
  totp.value = ''
  error.value = ''
}, { immediate: true })

function close() { if (!saving.value) emit('close') }
async function submit() {
  if (!props.account || saving.value) return
  saving.value = true
  error.value = ''
  try {
    await createTokenGuardV2Account({ account_id: props.account.id, login_email: email.value, credential_mode: 'password_totp', proxy_source: 'account', password: password.value, totp_secret: totp.value || undefined })
    emit('saved')
    emit('close')
  } catch (cause) {
    error.value = extractApiErrorMessage(cause, t('admin.accounts.loginColumns.failed'))
  } finally {
    saving.value = false
  }
}
</script>
