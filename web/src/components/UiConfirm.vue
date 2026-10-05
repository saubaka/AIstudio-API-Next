<script setup lang="ts">
import { confirmState, settleConfirm } from '@/confirm'
import { useI18n } from '@/i18n'
import UiDialog from './UiDialog.vue'
const { t } = useI18n()
</script>
<template>
  <UiDialog
    :open="confirmState.open"
    :title="confirmState.action"
    :width="400"
    alert
    @close="settleConfirm(false)"
  >
    <div>
      <h3 class="mb-3">{{ confirmState.action }}</h3>
      <p class="text-sm text-gray-400">{{ confirmState.message }}</p>
      <div class="mt-5 flex justify-end gap-2">
        <button
          class="rounded border border-line bg-surface px-4 py-2 text-sm"
          type="button"
          @click="settleConfirm(false)"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          class="rounded bg-danger-soft px-4 py-2 text-sm"
          type="button"
          @click="settleConfirm(true)"
        >
          {{ confirmState.action }}
        </button>
      </div>
    </div>
  </UiDialog>
</template>
