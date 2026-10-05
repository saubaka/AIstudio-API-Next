<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Model, UpstreamChannel } from '@/types'
import { useI18n } from '@/i18n'
import ModelsTable from './ModelsTable.vue'
import PlaygroundPanel from './PlaygroundPanel.vue'
import UiIcon from './UiIcon.vue'

const props = defineProps<{
  channel: UpstreamChannel
  models: Model[]
  loading: boolean
  error: string
  apiKey: string
  ready: boolean
  enabled: boolean
}>()
const { locale } = useI18n()
const cn = computed(() => locale.value === 'zh-CN')
const title = computed(() => (props.channel === 'build' ? 'App Build' : 'Playground'))
const view = ref<'models' | 'try' | 'connect'>('models')
const selectedModel = ref('')
const copied = ref('')
const channelModels = computed(() =>
  props.models
    .filter((model) => model.channels?.includes(props.channel))
    .map((model) => ({
      ...model,
      channels: [props.channel],
      methods: props.channel === 'build' ? ['generateContent'] : model.methods,
    })),
)
const base = computed(() => `${location.origin}/${props.channel}`)
const endpoints = computed(() => [
  {
    label: 'OpenAI Chat / Responses',
    url: `${base.value}/v1`,
    path: '/v1/chat/completions · /v1/responses',
  },
  { label: 'Anthropic Messages', url: base.value, path: '/v1/messages' },
  { label: 'Gemini', url: base.value, path: '/v1beta/models/{model}:generateContent' },
])
const example = computed(() =>
  JSON.stringify(
    {
      model:
        channelModels.value.find((model) => model.id === 'gemini-flash-latest')?.id ??
        channelModels.value.find((model) => model.methods.includes('generateContent'))?.id ??
        '{model}',
      messages: [{ role: 'user', content: 'Hello' }],
      stream: true,
    },
    null,
    2,
  ),
)
function tryModel(model: string): void {
  selectedModel.value = model
  view.value = 'try'
}
async function copy(value: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(value)
    copied.value = value
  } catch {
    copied.value = 'failed'
  }
}
</script>

<template>
  <section class="channel-workspace" :data-channel="channel" :aria-label="title">
    <header class="channel-heading">
      <div>
        <span class="channel-eyebrow">{{ cn ? '独立 API 渠道' : 'Dedicated API channel' }}</span>
        <h2>
          {{ title }}
          <span class="channel-model-count"
            >{{ channelModels.length }} {{ cn ? '个模型' : 'models' }}</span
          >
        </h2>
        <p>
          {{
            channel === 'build'
              ? cn
                ? '通过 App Build 代理调用 Gemini，使用 Build 渠道额度。'
                : 'Call Gemini through App Build using its separate quota.'
              : cn
                ? '调用 AI Studio Playground，使用 Playground 渠道额度。'
                : 'Call AI Studio Playground using its separate quota.'
          }}
        </p>
      </div>
      <span class="channel-route-badge">/{{ channel }}</span>
    </header>
    <nav class="channel-tabs" :aria-label="cn ? '渠道功能' : 'Channel views'">
      <button
        type="button"
        :aria-current="view === 'models' ? 'page' : undefined"
        @click="view = 'models'"
      >
        {{ cn ? '模型目录' : 'Models' }}
      </button>
      <button
        type="button"
        :aria-current="view === 'try' ? 'page' : undefined"
        @click="view = 'try'"
      >
        {{ cn ? 'API 试用' : 'API tester' }}
      </button>
      <button
        type="button"
        :aria-current="view === 'connect' ? 'page' : undefined"
        @click="view = 'connect'"
      >
        {{ cn ? '接入说明' : 'Connect' }}
      </button>
    </nav>
    <p v-if="!enabled || !ready" class="channel-state-note" role="status">
      {{
        !enabled
          ? cn
            ? '此渠道未启用，请在服务配置中启用并重启生成服务。'
            : 'Enable this channel in settings and restart the service.'
          : cn
            ? '生成服务尚未运行，请先启动服务。'
            : 'Start the generation service to send requests.'
      }}
    </p>
    <div class="channel-content" :class="{ 'is-tester': view === 'try' }">
      <KeepAlive>
        <ModelsTable
          v-if="view === 'models'"
          :models="channelModels"
          :loading="loading"
          :error="error"
          @try-model="tryModel"
        />
        <PlaygroundPanel
          v-else-if="view === 'try'"
          :channel="channel"
          :models="channelModels"
          :api-key="apiKey"
          :initial-model="selectedModel"
          :ready="ready && enabled"
        />
      </KeepAlive>
      <div v-if="view === 'connect'" class="channel-connect">
        <p class="channel-connect-intro">
          {{
            cn
              ? '在客户端填写对应的 Base URL，API 密钥使用服务配置中的密钥。此入口只使用当前渠道，不会切换到另一渠道。'
              : 'Use the matching Base URL and your service API key. Requests stay on this channel without switching upstreams.'
          }}
        </p>
        <article v-for="endpoint in endpoints" :key="endpoint.label" class="channel-endpoint">
          <h3>{{ endpoint.label }}</h3>
          <div>
            <code>{{ endpoint.url }}</code
            ><button
              type="button"
              :aria-label="`${cn ? '复制' : 'Copy'} ${endpoint.label} Base URL`"
              @click="copy(endpoint.url)"
            >
              <UiIcon :name="copied === endpoint.url ? 'check' : 'copy'" :size="16" />
            </button>
          </div>
          <small>{{ endpoint.path }}</small>
        </article>
        <p v-if="copied" role="status" class="channel-copy-status">
          {{
            copied === 'failed'
              ? cn
                ? '复制失败，请手动选择地址。'
                : 'Select and copy the address manually.'
              : cn
                ? '接入地址已复制'
                : 'Base URL copied'
          }}
        </p>
        <article class="channel-endpoint">
          <h3>{{ cn ? '模型目录接口' : 'Model catalog endpoint' }}</h3>
          <code>{{ base }}/v1/models</code>
          <small>{{
            cn
              ? '仅返回此渠道可调用的模型，使用相同 API 密钥鉴权。'
              : 'Returns only this channel’s models, authenticated with the same API key.'
          }}</small>
        </article>
        <article class="channel-endpoint">
          <h3>{{ cn ? '文件与媒体上传' : 'File and media uploads' }}</h3>
          <code>POST {{ base }}/v1/files</code>
          <small
            >/v1/uploads/images · /v1/uploads/audio · /v1/uploads/videos · /v1/uploads/files</small
          >
          <small>{{
            cn
              ? '使用 multipart/form-data，字段 file 和 purpose=user_data。单文件上限 32 MiB；把返回的 id 放入 Responses 的 input_file.file_id。支持图片、音频、视频、PDF 和 UTF-8 文本。'
              : 'Send multipart/form-data with file and purpose=user_data. Maximum 32 MiB per file. Reference the returned id through Responses input_file.file_id. Images, audio, video, PDF and UTF-8 text are supported.'
          }}</small>
          <small>{{
            cn
              ? '文件保存在本机 runtime/uploads，生成请求引用时才发送给当前渠道。GET /v1/files 列出文件；DELETE /v1/files/{id} 删除文件。'
              : 'Files persist locally in runtime/uploads and are sent to this channel only when referenced. GET /v1/files lists uploads; DELETE /v1/files/{id} removes them.'
          }}</small>
        </article>
        <article class="channel-endpoint">
          <h3>{{ cn ? '请求示例' : 'Request example' }}</h3>
          <code>POST /{{ channel }}/v1/chat/completions</code>
          <small>Authorization: Bearer &lt;API_KEY&gt; · Content-Type: application/json</small>
          <pre>{{ example }}</pre>
        </article>
        <p class="channel-connect-intro">
          {{
            channel === 'build'
              ? cn
                ? '支持 Chat、Responses、Messages、Gemini 生成，以及目录允许的图片、语音和音乐生成。两套渠道均支持本地文件、图片、音频和视频上传。CountTokens、Live、Veo、转录和 Interactions 请使用 Playground。'
                : 'Supports Chat, Responses, Messages, Gemini generation and supported image/speech/music models. Both channels accept local file, image, audio and video uploads. CountTokens, Live, Veo, transcription and Interactions use Playground.'
              : cn
                ? '支持各生成协议，以及模型能力允许的 Files、CountTokens、Live、Veo、转录和 Interactions。'
                : 'Supports generation protocols and model-specific Files, CountTokens, Live, Veo, transcription and Interactions.'
          }}
        </p>
      </div>
    </div>
  </section>
</template>
