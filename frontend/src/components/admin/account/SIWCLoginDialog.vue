<template>
  <div v-if="account" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="emit('close')">
    <div class="w-full max-w-lg rounded-xl bg-white p-4 shadow-xl dark:bg-dark-800" data-testid="siwc-login-dialog">
      <div class="mb-3 flex items-center justify-between">
        <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">SIWC · {{ account.name }}</h2>
        <button type="button" class="text-gray-400" @click="emit('close')">×</button>
      </div>
      <SIWCAccountSettings :account-id="account.id" :enabled="enabled" :active="true" @update:enabled="saveEnabled" />
      <p v-if="error" role="alert" class="mt-2 text-sm text-red-600">{{ error }}</p>
    </div>
  </div>
</template>
<script setup lang="ts">
import { ref, watch } from 'vue'
import SIWCAccountSettings from '@/components/account/SIWCAccountSettings.vue'
import { setSIWCEnabled } from '@/api/admin/siwc'
import type { AccountListItem } from '@/types'

const props = defineProps<{ account: AccountListItem | null }>()
const emit = defineEmits<{ close: [] }>()
const enabled = ref(false)
const saving = ref(false)
const error = ref('')
watch(() => props.account, (account) => {
  enabled.value = account?.extra?.openai_siwc === true
  error.value = ''
}, { immediate: true })

async function saveEnabled(value: boolean) {
  const account = props.account
  if (!account || saving.value || value === enabled.value) return
  const previous = enabled.value
  enabled.value = value
  saving.value = true
  error.value = ''
  try {
    await setSIWCEnabled(account.id, value)
    account.extra = { ...(account.extra ?? {}), openai_siwc: value }
  } catch (cause) {
    enabled.value = previous
    const failure = cause as { message?: string; response?: { data?: { message?: string } } }
    error.value = failure?.response?.data?.message || failure?.message || '保存协议失败'
  } finally {
    saving.value = false
  }
}
</script>
