<script setup lang="ts">
import { computed, nextTick, onUnmounted, reactive, ref, watch } from 'vue'
import { api } from '@/api'
import { useI18n } from '@/i18n'
import type { Account, GoogleLoginOptions, GoogleLoginSession, GooglePasteCheck } from '@/types'
import UiDialog from './UiDialog.vue'
import GoogleCookieGuide from './GoogleCookieGuide.vue'

const props = defineProps<{ open: boolean; account: Account | null }>()
const emit = defineEmits<{ close: []; complete: [] }>()
const { locale } = useI18n()
const text = (cn: string, en: string) => (locale.value === 'zh-CN' ? cn : en)
const options = ref<GoogleLoginOptions | null>(null)
const session = ref<GoogleLoginSession | null>(null)
const mode = ref<'open' | 'link'>('open')
const browserID = ref('')
const busy = ref(false)
const error = ref('')
const data = ref('')
const manual = ref(false)
const copied = ref(false)
const pasteCheck = ref<GooglePasteCheck | null>(null)
const closeButton = ref<HTMLButtonElement>()
const stepHeading = ref<HTMLElement>()
const environment = reactive({ proxy: '', locale: '', timezone: '' })
const selectedBrowser = computed(() =>
  options.value?.browsers.find((browser) => browser.id === browserID.value),
)
const message = (cause: unknown) =>
  cause instanceof Error ? cause.message : text('操作失败，请重试', 'Request failed. Please retry.')

watch(data, () => {
  pasteCheck.value = null
  error.value = ''
})

async function inspectPaste(fromClipboard = false): Promise<void> {
  busy.value = true
  error.value = ''
  pasteCheck.value = null
  try {
    if (fromClipboard) {
      try {
        data.value = await navigator.clipboard.readText()
      } catch {
        throw new Error(
          text(
            '无法读取剪贴板，请点击下方输入框按 ⌘V 粘贴，再检查内容。',
            'Paste with ⌘V in the text box, then check the pasted text.',
          ),
        )
      }
      await nextTick()
    }
    pasteCheck.value = await api.inspectGooglePaste(data.value)
  } catch (cause) {
    error.value = message(cause)
  } finally {
    busy.value = false
  }
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    error.value = ''
    data.value = ''
    session.value = null
    copied.value = false
    manual.value = false
    environment.proxy = props.account?.proxy ?? ''
    environment.locale = props.account?.locale ?? (navigator.language || 'en-US')
    environment.timezone =
      props.account?.timezone ?? (Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')
    busy.value = true
    try {
      options.value = await api.googleLoginOptions()
      browserID.value = options.value.browsers[0]?.id ?? ''
      mode.value = browserID.value ? 'open' : 'link'
    } catch (cause) {
      error.value = message(cause)
    } finally {
      busy.value = false
      await nextTick()
      if (props.open) closeButton.value?.focus()
    }
  },
  { immediate: true },
)

async function start(): Promise<void> {
  busy.value = true
  error.value = ''
  try {
    session.value = await api.startGoogleLogin({
      browser_id: browserID.value,
      mode: mode.value,
      account_id: props.account?.id ?? '',
      ...environment,
    })
    manual.value = session.value.capture === 'manual'
    await nextTick()
    stepHeading.value?.focus()
  } catch (cause) {
    error.value = message(cause)
  } finally {
    busy.value = false
  }
}

async function complete(): Promise<void> {
  if (!session.value) return
  busy.value = true
  error.value = ''
  try {
    await api.completeGoogleLogin(session.value.id, manual.value ? data.value : '')
    data.value = ''
    session.value = null
    emit('complete')
    emit('close')
  } catch (cause) {
    error.value = message(cause)
  } finally {
    busy.value = false
  }
}

async function close(): Promise<void> {
  if (busy.value) return
  busy.value = true
  const id = session.value?.id
  session.value = null
  data.value = ''
  try {
    if (id) await api.cancelGoogleLogin(id)
  } catch {
    /* The server also expires pending sessions automatically. */
  } finally {
    busy.value = false
    emit('close')
  }
}

async function copyLink(): Promise<void> {
  if (!session.value) return
  try {
    await navigator.clipboard.writeText(session.value.url)
    copied.value = true
  } catch {
    error.value = text('复制失败，请选择链接手动复制', 'Select the link and copy it manually.')
  }
}

async function readFile(event: Event): Promise<void> {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  if (file.size > 512 * 1024) {
    error.value = text('会话文件不能超过 512 KB', 'Session files must be under 512 KB')
    input.value = ''
    return
  }
  try {
    data.value = await file.text()
    error.value = ''
  } catch {
    error.value = text('读取文件失败', 'Could not read the file')
  }
  input.value = ''
}

onUnmounted(() => {
  data.value = ''
  if (session.value) void api.cancelGoogleLogin(session.value.id).catch(() => {})
})
</script>

<template>
  <UiDialog
    :open="open"
    :title="text('Google 账户登录', 'Google account sign-in')"
    :width="560"
    @close="close"
  >
    <div class="google-login-dialog">
      <div class="google-login-heading">
        <div>
          <h3>{{ text('Google 账户登录', 'Google account sign-in') }}</h3>
          <p v-if="account" class="google-login-account">{{ account.label }}</p>
        </div>
        <button
          ref="closeButton"
          type="button"
          class="icon-button"
          :aria-label="text('关闭', 'Close')"
          :disabled="busy"
          @click="close"
        >
          ×
        </button>
      </div>
      <p v-if="error" role="alert" class="google-login-error">{{ error }}</p>
      <template v-if="!session">
        <p class="google-login-hint">
          {{
            text(
              '在本机浏览器中登录 Google，完成后将会话接入本服务。',
              'Sign in to Google in a local browser, then connect that session to this service.',
            )
          }}
        </p>
        <fieldset class="google-login-modes">
          <legend class="sr-only">{{ text('登录方式', 'Sign-in method') }}</legend>
          <label :class="{ 'is-selected': mode === 'open' }"
            ><input
              v-model="mode"
              type="radio"
              value="open"
              :disabled="busy || !options?.browsers.length"
            /><span>{{ text('打开本机浏览器', 'Open local browser') }}</span></label
          >
          <label :class="{ 'is-selected': mode === 'link' }"
            ><input v-model="mode" type="radio" value="link" :disabled="busy" /><span>{{
              text('仅获取登录链接', 'Get sign-in link')
            }}</span></label
          >
        </fieldset>
        <label v-if="mode === 'open'" class="google-login-field"
          ><span>{{ text('选择浏览器', 'Choose browser') }}</span>
          <select
            v-model="browserID"
            :aria-label="text('选择浏览器', 'Choose browser')"
            :disabled="busy"
            class="w-full rounded border border-line bg-surface px-3 py-2"
          >
            <option v-for="browser in options?.browsers" :key="browser.id" :value="browser.id">
              {{ browser.name }}
            </option>
          </select>
        </label>
        <p v-if="mode === 'open' && selectedBrowser?.automatic" class="google-login-hint">
          {{
            text(
              '推荐：无需复制 Cookie。在打开的浏览器专用窗口登录并进入 AI Studio，返回这里点击“完成登录并接入”。日常浏览器资料保持独立。',
              'Uses a dedicated sign-in window in your installed browser. Return here after entering AI Studio to connect the account.',
            )
          }}
        </p>
        <p v-else class="google-login-hint">
          {{
            text(
              'Safari 或自行打开链接：登录后，在浏览器中复制 AI Studio 请求为 cURL，整段粘贴即可提取 Cookie。下一步有详细教程和检查工具。',
              'For Safari or manually opened links, copy an AI Studio request as cURL and paste the whole text. The next step includes a browser guide and a paste checker.',
            )
          }}
        </p>
        <details class="google-login-environment">
          <summary>{{ text('账户环境', 'Account environment') }}</summary>
          <label class="google-login-field"
            ><span>{{
              text('账户代理（留空使用服务默认代理）', 'Account proxy (blank uses service default)')
            }}</span
            ><input
              v-model.trim="environment.proxy"
              type="url"
              placeholder="http://127.0.0.1:10808"
          /></label>
          <div class="google-login-grid">
            <label class="google-login-field"
              ><span>{{ text('语言区域', 'Locale') }}</span
              ><input v-model.trim="environment.locale" /></label
            ><label class="google-login-field"
              ><span>{{ text('时区', 'Timezone') }}</span
              ><input v-model.trim="environment.timezone"
            /></label>
          </div>
          <p class="google-login-hint">
            {{
              text(
                'Safari 使用自身的网络与代理设置。',
                'Safari uses its own network and proxy settings.',
              )
            }}
          </p>
        </details>
        <div class="google-login-actions">
          <button type="button" :disabled="busy" @click="close">{{ text('取消', 'Cancel') }}</button
          ><button
            type="button"
            :disabled="busy || !options || (mode === 'open' && !browserID)"
            :aria-busy="busy"
            @click="start"
          >
            {{
              busy
                ? text('正在准备…', 'Preparing…')
                : mode === 'link'
                  ? text('获取登录链接', 'Get sign-in link')
                  : text('打开登录页面', 'Open sign-in page')
            }}
          </button>
        </div>
      </template>
      <template v-else>
        <div ref="stepHeading" class="google-login-step" tabindex="-1">
          <strong>{{
            session.opened
              ? text('登录页面已打开', 'Sign-in page opened')
              : text('登录链接已准备好', 'Sign-in link ready')
          }}</strong>
          <p>
            {{ session.browser_name }} · {{ text('会话有效至', 'Session expires at') }}
            {{
              new Date(session.expires_at).toLocaleTimeString(locale, {
                hour: '2-digit',
                minute: '2-digit',
              })
            }}
          </p>
        </div>
        <label class="google-login-field"
          ><span>{{ text('Google 官方登录链接', 'Google sign-in link') }}</span>
          <div class="google-login-link">
            <input :value="session.url" readonly type="url" /><button
              type="button"
              @click="copyLink"
            >
              {{ copied ? text('已复制', 'Copied') : text('复制链接', 'Copy link') }}
            </button>
          </div></label
        >
        <p v-if="!manual" class="google-login-hint">
          {{
            text(
              '请在打开的浏览器中完成 Google 登录，进入可使用的 AI Studio 页面，再点击“完成登录并接入”。未登录或未验证成功时，不会创建账户。',
              'Sign in in the opened browser and enter AI Studio, then click Finish sign-in. No account is created until the session is verified.',
            )
          }}
        </p>
        <label v-if="session.capture === 'automatic'" class="google-login-manual-toggle"
          ><input v-model="manual" type="checkbox" :disabled="busy" />{{
            text('改用手动会话导入', 'Import session manually instead')
          }}</label
        >
        <template v-if="manual">
          <GoogleCookieGuide :browser="session.browser_id" />
          <label class="google-login-field"
            ><span>{{
              text(
                '粘贴浏览器复制的 cURL、Cookie 或会话 JSON',
                'Paste cURL, Cookie, or session JSON',
              )
            }}</span
            ><textarea
              v-model="data"
              rows="4"
              autocomplete="off"
              spellcheck="false"
              :disabled="busy"
              :placeholder="
                text('整段粘贴“复制为 cURL”的内容即可…', 'Paste the entire Copy as cURL text…')
              "
            ></textarea>
          </label>
          <div class="google-paste-tools">
            <button type="button" :disabled="busy" @click="inspectPaste(true)">
              {{ text('从剪贴板粘贴并检查', 'Paste from clipboard and check') }}
            </button>
            <button type="button" :disabled="busy || !data.trim()" @click="inspectPaste()">
              {{ text('检查粘贴内容', 'Check pasted text') }}
            </button>
          </div>
          <div v-if="pasteCheck" class="google-paste-result" role="status">
            <p v-if="pasteCheck.ready">
              {{
                text(
                  `已识别 ${pasteCheck.cookie_count} 个 Cookie，登录所需字段齐全。可以点击“完成登录并接入”。`,
                  `Found ${pasteCheck.cookie_count} cookies and all required fields. You can now finish sign-in.`,
                )
              }}
            </p>
            <p v-else>
              {{
                text(
                  '已识别 Cookie，但缺少以下有效字段：',
                  'Found cookies, but these required fields are missing or expired:',
                )
              }}
              {{ pasteCheck.missing.join('、') }}
            </p>
            <p>
              {{
                text(
                  '此检查只识别格式和字段；真实登录状态会在完成接入时验证。',
                  'This checks format and fields only. Sign-in is verified when connecting the account.',
                )
              }}
            </p>
          </div>
          <label class="google-login-file"
            ><span>{{ text('选择 JSON 文件', 'Choose JSON file') }}</span
            ><input type="file" accept=".json,application/json" :disabled="busy" @change="readFile"
          /></label>
        </template>
        <div class="google-login-actions">
          <button type="button" :disabled="busy" @click="close">
            {{ text('取消登录', 'Cancel sign-in') }}</button
          ><button
            type="button"
            :disabled="
              busy || (manual && (!data.trim() || (pasteCheck !== null && !pasteCheck.ready)))
            "
            :aria-busy="busy"
            @click="complete"
          >
            {{
              busy
                ? text('正在验证会话…', 'Verifying session…')
                : text('完成登录并接入', 'Finish sign-in')
            }}
          </button>
        </div>
      </template>
    </div>
  </UiDialog>
</template>
