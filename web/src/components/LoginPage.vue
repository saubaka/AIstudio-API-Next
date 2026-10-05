<script setup lang="ts">
import { ref } from 'vue'
import { api, type AdminSession } from '@/api'
import { useI18n } from '@/i18n'
import UiIcon from './UiIcon.vue'

const emit = defineEmits<{ authenticated: [session: AdminSession] }>()
const { t } = useI18n()
const username = ref('')
const password = ref('')
const busy = ref(false)
const error = ref('')

// login 提交管理员凭据并清理密码输入
async function login(): Promise<void> {
  busy.value = true
  error.value = ''
  try {
    emit('authenticated', await api.login(username.value, password.value))
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : t('common.error')
  } finally {
    password.value = ''
    busy.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <main class="login-main">
      <section class="login-card">
        <div class="login-heading">
          <div class="login-logo"><UiIcon name="key" :size="32" /></div>
          <span class="login-product-name">aistudio2api</span>
          <h1>{{ t('auth.login') }}</h1>
        </div>
        <form class="login-form" @submit.prevent="login">
          <label
            ><span>{{ t('auth.username') }}</span>
            <input
              v-model.trim="username"
              name="username"
              autocomplete="username"
              autofocus
              required
            />
          </label>
          <label
            ><span>{{ t('auth.password') }}</span>
            <input
              v-model="password"
              name="password"
              type="password"
              autocomplete="current-password"
              required
            />
          </label>
          <p v-if="error" role="alert" class="text-sm text-red-300">{{ error }}</p>
          <button :disabled="busy" :aria-busy="busy" type="submit" class="login-submit">
            {{ t('auth.login') }}
          </button>
        </form>
      </section>
    </main>
    <footer class="login-footer">AIStudio2API</footer>
  </div>
</template>
