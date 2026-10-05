// Isolated visual fixture. All HTTP and SSE are replaced before the app mounts.
// No credentials, Google sessions, or real service writes are used here.
import { createApp } from 'vue'
import ConsoleRoot from '../src/ConsoleRoot.vue'
import LoginPage from '../src/components/LoginPage.vue'
import { vTooltip } from '../src/tooltip'
import type { Account, Model, ServiceConfig, ServiceStatus } from '../src/types'
import '../src/style.css'

const params = new URLSearchParams(location.search)
if (params.has('reduce')) document.documentElement.dataset.motion = 'reduce'
const models: Model[] = [
  {
    id: 'fixture-flash',
    name: 'Flash 示例模型',
    description: '隔离验收中的文本与工具调用模型',
    methods: ['generateContent', 'countTokens'],
    input_token_limit: 1048576,
    output_token_limit: 65536,
    channels: ['playground', 'build'],
    capabilities: { text: true, thinking: true, tools: true },
    capability_options: { thinking_level: ['low', 'high'] },
  },
  {
    id: 'fixture-image',
    name: 'Image 示例模型',
    methods: ['generateContent'],
    input_token_limit: 32768,
    output_token_limit: 32768,
    paid: true,
    channels: ['playground'],
    capabilities: { image: true },
    capability_options: { resolution: ['1K', '2K'] },
  },
]
const accounts: Account[] = [
  {
    id: 'ready@example.test',
    label: 'ready@example.test',
    enabled: true,
    state: 'ready',
    proxy: '',
    locale: 'zh-CN',
    timezone: 'Asia/Shanghai',
    models: ['fixture-flash'],
    benefit_tier: 'Free',
    message: '',
  },
  {
    id: 'login@example.test',
    label: 'login@example.test',
    enabled: true,
    state: 'auth_required',
    proxy: '',
    locale: 'zh-CN',
    timezone: 'Asia/Shanghai',
    models: [],
    benefit_tier: 'Free',
    message: '隔离示例：登录状态需要更新，请重新登录。',
  },
]
const config: ServiceConfig = {
  admin_auth_enabled: false,
  admin_username: 'fixture-admin',
  admin_password_set: false,
  auth_states: 'fixture-auth',
  listen_addr: '127.0.0.1:2048',
  proxy_api_key: 'fixture-key-not-a-secret',
  active_listen_addr: '127.0.0.1:2048',
  active_proxy_api_key: 'fixture-key-not-a-secret',
  management_restart_required: false,
  service_restart_required: false,
  proxy: '',
  init_timeout: '2m',
  request_timeout: '5m',
  warm_worker_limit: 5,
  max_active_workers: 10,
  warm_startup_concurrency: 2,
  per_account_concurrency: 2,
  routing_strategy: 'round-robin',
  upstream_channels: ['playground', 'build'],
  waa_backend: 'camoufox',
  temporary_chat: false,
  build_native_nonstream: true,
}
const status: ServiceStatus = {
  state: 'STOPPED',
  running: false,
  ready: false,
  version: 'UI fixture',
  active_requests: 0,
  accounts: { total: 2, ready: 1, busy: 0, cooldown: 0, auth_required: 1 },
}
class FixtureEvents {
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  timer: number
  constructor() {
    this.timer = window.setTimeout(() => {
      this.onopen?.()
      const data = {
        type: 'log',
        data: {
          time: new Date().toISOString(),
          level: 'INFO',
          source: '隔离验收',
          message: 'UI 示例已就绪，所有请求均在浏览器内模拟。',
          event: 'fixture',
        },
      }
      this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(data) }))
    }, 100)
  }
  close() {
    window.clearTimeout(this.timer)
  }
}
window.EventSource = FixtureEvents as unknown as typeof EventSource
window.fetch = async (input, init) => {
  const path = new URL(input instanceof Request ? input.url : String(input), location.href).pathname
  const method = init?.method ?? 'GET'
  const response = (body: unknown, code = 200) =>
    new Response(JSON.stringify(body), {
      status: code,
      headers: { 'Content-Type': 'application/json' },
    })
  if (path === '/api/auth/google/options')
    return response({
      browsers: [
        { id: 'chrome', name: 'Google Chrome', automatic: true },
        { id: 'safari', name: 'Safari', automatic: false },
      ],
      url: 'https://accounts.google.com/AccountChooser?continue=https%3A%2F%2Faistudio.google.com%2Fprompts%2Fnew_chat',
      chrome_import_supported: false,
    })
  if (path === '/api/auth/google/start') {
    const input = JSON.parse(String(init?.body ?? '{}'))
    return response(
      {
        id: 'fixture-login-session',
        url: 'https://accounts.google.com/AccountChooser?continue=https%3A%2F%2Faistudio.google.com%2Fprompts%2Fnew_chat',
        browser_id: input.mode === 'link' ? 'link' : input.browser_id,
        browser_name:
          input.mode === 'link'
            ? '登录链接'
            : input.browser_id === 'safari'
              ? 'Safari'
              : 'Google Chrome',
        capture: input.mode === 'open' && input.browser_id === 'chrome' ? 'automatic' : 'manual',
        expires_at: new Date(Date.now() + 900000).toISOString(),
        opened: input.mode === 'open',
      },
      201,
    )
  }
  if (path === '/api/auth/google/fixture-login-session' && method === 'DELETE')
    return new Response(null, { status: 204 })
  if (path === '/api/auth/google/fixture-login-session/complete') {
    const input = JSON.parse(String(init?.body ?? '{}'))
    if (input.data === 'invalid')
      return response(
        { error: { message: '隔离示例：会话验证失败，请重新导入完整 Cookie。' } },
        400,
      )
    return response({ account: accounts[0] })
  }
  if (path === '/api/auth/session')
    return response({ enabled: false, authenticated: true, username: '' })
  if (path === '/api/status') return response(status)
  if (path === '/api/accounts' && method === 'GET') return response({ accounts })
  if (path === '/api/models') return response({ models })
  if (path === '/api/config') return response(config)
  if (path === '/api/cooldowns')
    return response({
      cooldowns: [
        {
          account_id: accounts[0]!.id,
          account_label: accounts[0]!.label,
          channel: 'build',
          model_id: 'fixture-flash',
          until: new Date(Date.now() + 120000).toISOString(),
          reason: '隔离示例：分钟额度冷却',
        },
      ],
    })
  if (path === '/api/requests')
    return response({
      requests: [
        {
          id: 'fixture-request',
          model: 'fixture-flash',
          account_id: accounts[0]!.id,
          account_label: accounts[0]!.label,
          channel: 'playground',
          state: 'completed',
          started_at: new Date().toISOString(),
        },
      ],
    })
  if (path === '/api/accounts/import/chrome')
    return response({
      profiles: [
        {
          id: 'fixture-profile',
          profile: 'Default',
          display_name: '示例账户',
          email: 'import@example.test',
          locale: 'zh-CN',
        },
      ],
    })
  if (path.startsWith('/api/control/')) {
    status.state = path.endsWith('start') ? 'RUNNING' : 'STOPPED'
    status.running = status.state === 'RUNNING'
    status.ready = status.running
    return response(status)
  }
  return response({ error: { message: '隔离验收：此操作没有访问真实服务。' } }, 400)
}
createApp(params.has('login') ? LoginPage : ConsoleRoot)
  .directive('tooltip', vTooltip)
  .mount('#app')
const badge = document.createElement('div')
badge.textContent = '隔离 UI 示例 · 无真实账户操作'
badge.style.cssText =
  'position:fixed;left:12px;bottom:4px;z-index:20;font-size:10px;color:#778397;pointer-events:none'
document.body.append(badge)
