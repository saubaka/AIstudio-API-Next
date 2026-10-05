<script setup lang="ts">
import { computed } from 'vue'
import { channelLabelKey, useI18n } from '@/i18n'
import type { LogRow } from '@/logs'
import UiIcon from './UiIcon.vue'

const props = defineProps<{ row: LogRow }>()
const { t, locale } = useI18n()
const request = computed(() => props.row.entry.request!)

// number 按当前语言格式化统计数值
function number(value: number, digits = 0): string {
  return value.toLocaleString(locale.value, { maximumFractionDigits: digits })
}
</script>

<template>
  <div class="request-log">
    <div class="request-heading">
      <span class="request-state" :class="`state-${request.state}`">{{
        request.status ?? t('logs.running')
      }}</span>
      <strong>{{ request.model || `${request.method} ${request.path}` }}</strong>
      <span v-if="request.duration_ms !== undefined" class="request-duration"
        >{{ number(request.duration_ms / 1000, 2) }} s</span
      >
      <span v-if="request.tool_calls" class="tool-count"
        >{{ t('logs.tools') }} {{ number(request.tool_calls) }}</span
      >
    </div>
    <div v-if="request.usage" class="request-metrics">
      <span
        >{{ t('logs.inputTokens') }} <b>{{ number(request.usage.input_tokens) }}</b></span
      >
      <span
        >{{ t('logs.cachedTokens') }}
        <b>{{
          request.usage.cache_tokens_known
            ? number(request.usage.cached_tokens ?? 0)
            : t('logs.cacheUnknown')
        }}</b></span
      >
      <span v-if="request.usage.estimated">{{ t('logs.estimatedUsage') }}</span>
      <span
        >{{ t('logs.reasoning') }} <b>{{ number(request.usage.reasoning_tokens) }}</b></span
      >
      <span
        >{{ t('logs.reply') }} <b>{{ number(request.usage.reply_tokens) }}</b></span
      >
      <span
        >{{ t('logs.outputTotal') }} <b>{{ number(request.usage.output_tokens) }}</b> tokens</span
      >
      <span
        >{{ t('logs.averageSpeed') }}
        <b>{{ number(request.usage.average_tokens_per_second, 1) }}</b> tokens/s</span
      >
    </div>
    <p v-if="request.error" class="request-error">{{ request.error }}</p>
    <p v-else-if="request.state === 'blocked' || request.state === 'limited'" class="request-error">
      {{ request.finish_reason }}
    </p>
    <details class="request-details">
      <summary>
        <UiIcon class="request-disclosure-icon" name="chevronRight" :size="12" />
        {{ t('logs.details') }}
        <span class="request-summary-meta"
          >{{ request.method }} · HTTP {{ request.status || '…'
          }}<template v-if="request.channel">
            · {{ t(channelLabelKey(request.channel)) }}</template
          ></span
        >
      </summary>
      <dl>
        <dt>ID</dt>
        <dd>{{ request.id }}</dd>
        <dt>{{ t('logs.endpoint') }}</dt>
        <dd>{{ request.method }} {{ request.path }}</dd>
        <template v-if="request.channel">
          <dt>{{ t('logs.channel') }}</dt>
          <dd>{{ t(channelLabelKey(request.channel)) }}</dd>
        </template>
        <template v-if="request.usage">
          <dt>{{ t('logs.inputTokens') }}</dt>
          <dd>{{ number(request.usage.input_tokens) }} tokens</dd>
          <dt>{{ t('logs.cachedTokens') }}</dt>
          <dd>
            {{
              request.usage.cache_tokens_known
                ? number(request.usage.cached_tokens ?? 0) + ' tokens'
                : t('logs.cacheUnknown')
            }}
          </dd>
          <template v-if="request.usage.cache_tokens_known">
            <dt>{{ t('logs.uncachedTokens') }}</dt>
            <dd>
              {{ number(request.usage.uncached_tokens ?? request.usage.input_tokens) }} tokens
            </dd>
          </template>
          <dt>{{ t('logs.totalTokens') }}</dt>
          <dd>{{ number(request.usage.total_tokens) }} tokens</dd>
        </template>
        <template v-if="request.input_messages">
          <dt>{{ t('logs.inputMessages') }}</dt>
          <dd>{{ number(request.input_messages) }}</dd>
        </template>
        <template v-if="request.finish_reason">
          <dt>{{ t('logs.finishReason') }}</dt>
          <dd>{{ request.finish_reason }}</dd>
        </template>
      </dl>
      <ol class="request-timeline">
        <li v-for="(event, index) in row.events" :key="index">
          <time>{{ new Date(event.time).toLocaleTimeString(locale, { hour12: false }) }}</time>
          <span>{{ event.message }}</span>
        </li>
      </ol>
      <details class="request-json">
        <summary>
          <UiIcon class="request-disclosure-icon" name="chevronRight" :size="12" />
          JSON
        </summary>
        <pre>{{ JSON.stringify(row.entry, null, 2) }}</pre>
      </details>
    </details>
  </div>
</template>

<style scoped>
.request-log {
  padding: 4px 0 6px;
}
.request-heading {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px 12px;
}
.request-heading strong {
  color: var(--text);
  font-weight: 600;
  overflow-wrap: anywhere;
}
.request-state {
  border: 1px solid currentColor;
  border-radius: 4px;
  padding: 0 6px;
  font-size: 11px;
  line-height: 19px;
}
.state-completed,
.state-tool_calls {
  color: var(--success);
  background: var(--success-soft);
}
.state-running {
  color: var(--sunny-blue);
}
.state-limited,
.state-blocked,
.state-cancelled {
  color: var(--warning);
}
.state-failed,
.request-error {
  color: var(--danger);
}
.request-duration {
  color: var(--text);
  margin-left: auto;
  font-variant-numeric: tabular-nums;
}
.tool-count {
  color: var(--sunny-blue);
  font-size: 12px;
}
.request-metrics {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 18px;
  margin-top: 6px;
  color: var(--text-soft);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.request-metrics b {
  color: var(--text);
  font-weight: 500;
}
.request-error {
  margin: 6px 0 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.request-details {
  margin-top: 5px;
  color: var(--text-soft);
  font-size: 11px;
}
.request-details summary {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 34px;
  padding: 6px 8px;
  border-radius: 4px;
  cursor: pointer;
  list-style: none;
  user-select: none;
  transition:
    background-color 120ms ease,
    color 120ms ease;
}
.request-details summary::-webkit-details-marker {
  display: none;
}
.request-details summary:hover {
  color: var(--text);
  background: var(--line-blue);
}
.request-details summary:focus-visible {
  outline: 1px solid var(--sunny-blue);
  outline-offset: 2px;
}
.request-disclosure-icon {
  color: var(--text-faint);
  transition: transform 120ms ease;
}
details[open] > summary > .request-disclosure-icon {
  transform: rotate(90deg);
}
.request-summary-meta {
  min-width: 0;
  margin-left: 6px;
  color: var(--text-faint);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
@media (pointer: coarse) {
  .request-details summary {
    min-height: 40px;
  }
}
@media (prefers-reduced-motion: reduce) {
  .request-details summary,
  .request-disclosure-icon {
    transition: none;
  }
}
.request-details dl {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  gap: 4px 16px;
  margin: 10px 0;
}
.request-details dd {
  color: var(--text);
  overflow-wrap: anywhere;
}
.request-timeline {
  border-left: 1px solid var(--line-blue);
  padding-left: 12px;
  margin: 10px 0;
}
.request-timeline li {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 4px;
}
.request-timeline time {
  flex-shrink: 0;
  color: var(--text-faint);
}
.request-json pre {
  overflow: auto;
  max-height: 320px;
  padding: 12px;
  background: var(--surface);
  border: 1px solid var(--line-blue);
  border-radius: 4px;
  margin-top: 6px;
  white-space: pre;
}
@media (max-width: 767px) {
  .request-duration {
    margin-left: 0;
  }
  .request-metrics {
    gap: 4px 12px;
  }
}
</style>
