<script setup lang="ts">
import { nextTick, onUnmounted, ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{ open: boolean; title: string; width?: number; alert?: boolean }>(),
  { width: 448 },
)
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement>()
let returnFocus: HTMLElement | null = null

watch(
  () => props.open,
  async (open) => {
    await nextTick()
    if (open && dialog.value && !dialog.value.open) {
      returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
      dialog.value.showModal()
      dialog.value
        .querySelector<HTMLElement>('button:not(:disabled), input:not(:disabled)')
        ?.focus()
    } else if (!open && dialog.value?.open) {
      dialog.value.close()
      if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true })
    }
  },
  { immediate: true },
)

function backdropClick(event: MouseEvent): void {
  if (event.target !== dialog.value) return
  const rect = dialog.value.getBoundingClientRect()
  if (
    event.clientX < rect.left ||
    event.clientX > rect.right ||
    event.clientY < rect.top ||
    event.clientY > rect.bottom
  )
    emit('close')
}
function trapFocus(event: KeyboardEvent): void {
  if (event.key !== 'Tab') return
  const controls = Array.from(
    dialog.value?.querySelectorAll<HTMLElement>(
      'button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href], [tabindex="0"]',
    ) ?? [],
  ).filter((element) => element.getClientRects().length > 0)
  const first = controls[0]
  const last = controls[controls.length - 1]
  if (!first) {
    event.preventDefault()
    dialog.value?.focus()
  } else if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last?.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}
onUnmounted(() => {
  dialog.value?.close()
  if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true })
})
</script>

<template>
  <Teleport to="body">
    <dialog
      ref="dialog"
      class="studio-dialog"
      :style="{ width: `min(${width}px, calc(100vw - 32px))` }"
      :role="alert ? 'alertdialog' : 'dialog'"
      :aria-label="title"
      @cancel.prevent="emit('close')"
      @click="backdropClick"
      @keydown="trapFocus"
    >
      <slot />
    </dialog>
  </Teleport>
</template>
