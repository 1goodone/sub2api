<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <span
          class="mx-auto grid h-14 w-14 place-items-center rounded-full bg-primary-50 text-primary-700 dark:bg-primary-950/50 dark:text-primary-300"
          aria-hidden="true"
        >
          <Icon :name="status === 'success' ? 'checkCircle' : 'shield'" size="xl" />
        </span>
        <h2 class="mt-4 text-2xl font-bold text-gray-900 dark:text-white">
          {{ status === 'success' ? '桌面端已授权' : '授权小羊咩咩' }}
        </h2>
        <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-dark-300">
          {{ status === 'success' ? '可以关闭此页面，回到小羊咩咩继续选择分组。' : '确认是你刚刚在桌面端发起的连接。' }}
        </p>
      </div>

      <template v-if="status !== 'success'">
        <div class="border-y border-gray-200 py-4 dark:border-dark-700">
          <div class="flex items-center justify-between gap-4">
            <span class="text-sm text-gray-500 dark:text-dark-300">配对校验码</span>
            <strong class="font-mono text-base tracking-normal text-gray-900 dark:text-white">
              {{ userCode || '无效请求' }}
            </strong>
          </div>
        </div>

        <div class="space-y-3 text-sm text-gray-600 dark:text-dark-300">
          <p class="font-medium text-gray-900 dark:text-white">授权后，小羊咩咩将：</p>
          <div class="flex gap-3">
            <Icon name="check" size="sm" class="mt-0.5 shrink-0 text-primary-600" />
            <span>读取你可用的站点分组</span>
          </div>
          <div class="flex gap-3">
            <Icon name="check" size="sm" class="mt-0.5 shrink-0 text-primary-600" />
            <span>创建一个名为“小羊咩咩桌面端”的专用 API Key</span>
          </div>
          <div class="flex gap-3">
            <Icon name="check" size="sm" class="mt-0.5 shrink-0 text-primary-600" />
            <span>把 Key 保存到本机系统凭据库，用于余额提醒和 CC Switch 配置</span>
          </div>
        </div>

        <div class="flex gap-3 border-l-4 border-sky-500 bg-sky-50 px-4 py-3 text-sm leading-6 text-sky-900 dark:bg-sky-950/40 dark:text-sky-200">
          <Icon name="lock" size="sm" class="mt-1 shrink-0" />
          <span>账号密码、验证码和 2FA 只在本站登录页处理，不会发送给桌面端。</span>
        </div>

        <p v-if="error" class="text-sm text-red-600 dark:text-red-400" role="alert">
          {{ error }}
        </p>

        <div class="space-y-3">
          <button
            type="button"
            class="btn btn-primary min-h-11 w-full"
            :disabled="status === 'submitting' || !requestID"
            @click="approve"
          >
            <Icon v-if="status !== 'submitting'" name="shield" size="md" class="mr-2" />
            <span v-else class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white" aria-hidden="true"></span>
            {{ status === 'submitting' ? '正在授权' : '授权并继续' }}
          </button>
          <router-link class="btn btn-secondary min-h-11 w-full" to="/dashboard">
            取消
          </router-link>
        </div>
      </template>

      <div v-else class="space-y-3" aria-live="polite">
        <div class="flex gap-3 border-l-4 border-primary-600 bg-primary-50 px-4 py-3 text-sm leading-6 text-primary-900 dark:bg-primary-950/40 dark:text-primary-200">
          <Icon name="checkCircle" size="sm" class="mt-1 shrink-0" />
          <span>这次授权只能交换一次，并会在桌面端完成配置后从内存中清除。</span>
        </div>
        <router-link class="btn btn-secondary min-h-11 w-full" to="/dashboard">
          返回控制台
        </router-link>
      </div>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { apiClient } from '@/api/client'
import { AuthLayout } from '@/components/layout'
import Icon from '@/components/icons/Icon.vue'

type AuthorizationStatus = 'ready' | 'submitting' | 'success' | 'error'

const route = useRoute()
const status = ref<AuthorizationStatus>('ready')
const error = ref('')
const requestID = computed(() => {
  const value = Array.isArray(route.query.request) ? route.query.request[0] : route.query.request
  return typeof value === 'string' && /^[A-Za-z0-9_-]{43}$/.test(value) ? value : ''
})
const userCode = computed(() => {
  const value = requestID.value
  return value ? `${value.slice(0, 4)}-${value.slice(-4)}`.toUpperCase() : ''
})

async function approve(): Promise<void> {
  if (!requestID.value || status.value === 'submitting') return
  status.value = 'submitting'
  error.value = ''
  try {
    await apiClient.post('/auth/desktop/approve', { request_id: requestID.value })
    status.value = 'success'
  } catch (reason) {
    const failure = reason as { status?: number; message?: string }
    status.value = 'error'
    error.value = failure.status === 404
      ? '配对请求已过期，请回到桌面端重新发起。'
      : failure.status === 409
        ? '这个请求已经由其他账号授权，请回到桌面端重新发起。'
        : failure.message || '暂时无法完成授权，请稍后重试。'
  }
}
</script>
