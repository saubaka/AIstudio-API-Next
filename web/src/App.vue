<script setup lang="ts">
defineProps<{ adminUsername?: string }>()
const emit = defineEmits<{ logout: [] }>()
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { api, openAdminEvents, type EventConnection } from '@/api'
import { useI18n, type TranslationKey } from '@/i18n'
import type {
  Account,
  AdminLog,
  AdminEvent,
  Model,
  Cooldown,
  RequestSummary,
  ServiceConfig,
  ServiceStatus,
  TabID,
} from '@/types'
import AccountsPanel from '@/components/AccountsPanel.vue'
import LogsPanel from '@/components/LogsPanel.vue'
import ChannelWorkspace from '@/components/ChannelWorkspace.vue'
import RequestsPanel from '@/components/RequestsPanel.vue'
import SettingsPanel from '@/components/SettingsPanel.vue'
import UiConfirm from '@/components/UiConfirm.vue'
import WorkspaceShell from '@/components/WorkspaceShell.vue'
import UiIcon, { type IconName } from '@/components/UiIcon.vue'

const { locale, t } = useI18n()
const TAB_STORAGE_KEY = 'aistudio2api_active_tab'
const validTabs: TabID[] = ['logs', 'accounts', 'requests', 'settings', 'playground', 'build']
const savedTab =
  typeof window !== 'undefined'
    ? (window.localStorage.getItem(TAB_STORAGE_KEY) as TabID | null)
    : null
const currentTab = ref<TabID>(
  location.hash === '#google-login'
    ? 'accounts'
    : savedTab === 'models'
      ? 'playground'
      : savedTab && validTabs.includes(savedTab)
        ? savedTab
        : 'logs',
)
watch(currentTab, (tab) => {
  window.localStorage.setItem(TAB_STORAGE_KEY, tab)
})
const status = ref<ServiceStatus | null>(null)
const logs = ref<AdminLog[]>([])
const accounts = ref<Account[]>([])
const models = ref<Model[]>([])
const cooldowns = ref<Cooldown[]>([])
const requests = ref<RequestSummary[]>([])
const config = ref<ServiceConfig | null>(null)
const startPending = ref(false)
const stopPending = ref(false)
const launchCancellationRequested = ref(false)
const notice = reactive({ message: '', tone: 'success' as 'success' | 'error' })
const loading = reactive({
  accounts: true,
  models: true,
  requests: true,
  cooldowns: true,
  config: true,
})
const errors = reactive({ accounts: '', models: '', requests: '', cooldowns: '', config: '' })
let eventConnection: EventConnection | undefined
let mounted = false
let noticeTimer: number | undefined

const navigation: { id: TabID; label: TranslationKey; icon: IconName }[] = [
  { id: 'logs', label: 'nav.logs', icon: 'dashboard' },
  { id: 'accounts', label: 'nav.accounts', icon: 'key' },
  { id: 'requests', label: 'nav.requests', icon: 'info' },
  { id: 'settings', label: 'nav.settings', icon: 'settings' },
  { id: 'playground', label: 'nav.playground', icon: 'chat' },
  { id: 'build', label: 'nav.build', icon: 'dashboard' },
]

const serviceState = computed(() => {
  if (status.value === null) return 'unavailable'
  if (status.value.state === 'RUNNING') return 'running'
  if (status.value.state === 'LAUNCHING') return 'launching'
  if (startPending.value && !launchCancellationRequested.value) return 'launching'
  return 'stopped'
})
// messageOf 统一呈现服务端错误内容
function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : t('common.error')
}

// showNotice 显示一次短暂操作结果
function showNotice(message: string, tone: 'success' | 'error'): void {
  notice.message = message
  notice.tone = tone
  if (noticeTimer !== undefined) window.clearTimeout(noticeTimer)
  noticeTimer = window.setTimeout(() => {
    notice.message = ''
  }, 3200)
}

async function loadStatus(): Promise<void> {
  try {
    status.value = await api.status()
  } catch {
    status.value = null
  }
}

async function loadAccounts(): Promise<void> {
  loading.accounts = accounts.value.length === 0
  errors.accounts = ''
  try {
    accounts.value = await api.accounts()
  } catch (error) {
    errors.accounts = messageOf(error)
  } finally {
    loading.accounts = false
  }
}

async function loadAccountData(): Promise<void> {
  await Promise.all([loadAccounts(), loadModels(), loadStatus()])
}

async function loadModels(): Promise<void> {
  loading.models = models.value.length === 0
  errors.models = ''
  try {
    models.value = await api.models()
  } catch (error) {
    errors.models = messageOf(error)
  } finally {
    loading.models = false
  }
}

async function loadCooldowns(): Promise<void> {
  loading.cooldowns = cooldowns.value.length === 0
  errors.cooldowns = ''
  try {
    cooldowns.value = await api.cooldowns()
  } catch (error) {
    errors.cooldowns = messageOf(error)
  } finally {
    loading.cooldowns = false
  }
}

async function loadRequests(): Promise<void> {
  loading.requests = requests.value.length === 0
  errors.requests = ''
  try {
    requests.value = await api.requests()
  } catch (error) {
    errors.requests = messageOf(error)
  } finally {
    loading.requests = false
  }
}

async function loadRequestData(): Promise<void> {
  await Promise.all([loadRequests(), loadCooldowns()])
}

async function loadConfig(): Promise<void> {
  loading.config = config.value === null
  errors.config = ''
  try {
    config.value = await api.config()
  } catch (error) {
    errors.config = messageOf(error)
  } finally {
    loading.config = false
  }
}

async function refreshAll(): Promise<void> {
  await Promise.all([
    loadStatus(),
    loadAccounts(),
    loadModels(),
    loadCooldowns(),
    loadRequests(),
    loadConfig(),
  ])
}

async function startService(): Promise<void> {
  startPending.value = true
  launchCancellationRequested.value = false
  try {
    status.value = await api.startService()
    if (launchCancellationRequested.value || status.value.state !== 'RUNNING') return
    showNotice(t('app.start'), 'success')
    await Promise.all([loadAccounts(), loadModels(), loadCooldowns()])
  } catch (error) {
    if (!launchCancellationRequested.value) showNotice(messageOf(error), 'error')
    await loadStatus()
  } finally {
    startPending.value = false
  }
}

async function stopService(): Promise<void> {
  launchCancellationRequested.value = serviceState.value === 'launching'
  stopPending.value = true
  try {
    status.value = await api.stopService()
    showNotice(t('app.stop'), 'success')
    await Promise.all([loadAccounts(), loadModels(), loadCooldowns(), loadRequests()])
  } catch (error) {
    showNotice(messageOf(error), 'error')
    await loadStatus()
  } finally {
    stopPending.value = false
  }
}

let pendingLogs: AdminLog[] = []
let logFlushTimer: number | undefined

function flushLogs(): void {
  logFlushTimer = undefined
  if (pendingLogs.length === 0) return
  logs.value.push(...pendingLogs)
  pendingLogs = []
  if (logs.value.length > 2000) logs.value.splice(0, logs.value.length - 2000)
}

async function clearLogs(): Promise<void> {
  try {
    await api.clearLogs()
    pendingLogs = []
    if (logFlushTimer !== undefined) {
      window.clearTimeout(logFlushTimer)
      logFlushTimer = undefined
    }
    logs.value = []
  } catch (error) {
    showNotice(messageOf(error), 'error')
  }
}

function replaceByID<T extends { id: string }>(items: T[], incoming: T): void {
  const index = items.findIndex((item) => item.id === incoming.id)
  if (index === -1) items.unshift(incoming)
  else items[index] = incoming
}

function handleAdminEvent(event: AdminEvent): void {
  if (event.type === 'status') {
    status.value = event.data
    return
  }
  if (event.type === 'log') {
    pendingLogs.push(event.data)
    if (logFlushTimer === undefined) {
      logFlushTimer = window.setTimeout(flushLogs, 40)
    }
    return
  }
  if (event.type === 'accounts') {
    accounts.value = event.data.accounts
    return
  }
  if (event.type === 'models') {
    models.value = event.data.models
    return
  }
  if (event.type === 'cooldowns') {
    cooldowns.value = event.data
    return
  }
  replaceByID(requests.value, event.data)
}

onMounted(async () => {
  mounted = true
  document.title = t('app.title')
  await refreshAll()
  if (!mounted) return
  eventConnection = openAdminEvents(handleAdminEvent, () => {
    pendingLogs = []
    if (logFlushTimer !== undefined) {
      window.clearTimeout(logFlushTimer)
      logFlushTimer = undefined
    }
    logs.value = []
    requests.value = []
  })
})

watch(locale, () => {
  document.title = t('app.title')
})

onUnmounted(() => {
  mounted = false
  eventConnection?.close()
  if (logFlushTimer !== undefined) window.clearTimeout(logFlushTimer)
  if (noticeTimer !== undefined) window.clearTimeout(noticeTimer)
})
</script>

<template>
  <WorkspaceShell
    :current-tab="currentTab"
    :navigation="navigation"
    :service-state="serviceState"
    :busy="stopPending || (serviceState === 'stopped' && startPending)"
    :version="status?.version || '—'"
    :account-count="accounts.length"
    :model-count="models.length"
    :admin-username="adminUsername ?? ''"
    @navigate="currentTab = $event"
    @start="startService"
    @stop="stopService"
    @logout="emit('logout')"
  >
    <Transition name="workspace-panel" mode="out-in">
      <KeepAlive>
        <LogsPanel v-if="currentTab === 'logs'" :logs="logs" @clear="clearLogs" />
        <AccountsPanel
          v-else-if="currentTab === 'accounts'"
          :accounts="accounts"
          :loading="loading.accounts"
          :error="errors.accounts"
          @refresh="loadAccountData"
          @notice="showNotice"
        />
        <RequestsPanel
          v-else-if="currentTab === 'requests'"
          :accounts="accounts"
          :cooldowns="cooldowns"
          :requests="requests"
          :loading="loading.requests || loading.cooldowns"
          :cooldown-error="errors.cooldowns"
          :request-error="errors.requests"
          @refresh="loadRequestData"
          @notice="showNotice"
        />
        <SettingsPanel
          v-else-if="currentTab === 'settings'"
          :config="config"
          :loading="loading.config"
          :error="errors.config"
          @saved="config = $event"
          @notice="showNotice"
        />
        <ChannelWorkspace
          v-else
          :key="currentTab === 'build' ? 'build' : 'playground'"
          :channel="currentTab === 'build' ? 'build' : 'playground'"
          :models="models"
          :loading="loading.models"
          :error="errors.models"
          :api-key="config?.proxy_api_key ?? ''"
          :ready="serviceState === 'running'"
          :enabled="
            config?.upstream_channels.includes(currentTab === 'build' ? 'build' : 'playground') ??
            true
          "
        />
      </KeepAlive>
    </Transition>
  </WorkspaceShell>
  <Transition name="notice">
    <div
      v-if="notice.message"
      class="notification-capsule"
      :data-tone="notice.tone"
      :role="notice.tone === 'error' ? 'alert' : 'status'"
    >
      <span class="notification-dot" aria-hidden="true"></span>
      <span>{{ notice.message }}</span>
      <button type="button" :aria-label="t('common.close')" @click="notice.message = ''">
        <UiIcon name="close" :size="16" />
      </button>
    </div>
  </Transition>
  <UiConfirm />
</template>
