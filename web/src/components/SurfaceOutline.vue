<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'

// Geometry and four-part contour follow codedx/8's surfaceLines.ts.
const host = ref<SVGSVGElement>()
const width = ref(1)
const height = ref(1)
const radius = ref(12)
let observer: ResizeObserver | undefined
onMounted(() => {
  const parent = host.value?.parentElement
  if (!parent) return
  const measure = () => {
    width.value = Math.max(1, parent.clientWidth)
    height.value = Math.max(1, parent.clientHeight)
    radius.value = Math.min(
      parseFloat(getComputedStyle(parent).borderTopLeftRadius) || 12,
      width.value / 2,
      height.value / 2,
    )
  }
  observer = new ResizeObserver(measure)
  observer.observe(parent)
  measure()
})
onUnmounted(() => observer?.disconnect())
</script>

<template>
  <svg
    ref="host"
    class="surface-outline"
    :viewBox="`0 0 ${width} ${height}`"
    aria-hidden="true"
    focusable="false"
  >
    <rect
      v-for="part in 4"
      :key="part"
      :class="`surface-outline__piece part-${part - 1}`"
      x="1.5"
      y="1.5"
      :width="Math.max(0, width - 3)"
      :height="Math.max(0, height - 3)"
      :rx="radius"
      vector-effect="non-scaling-stroke"
    />
  </svg>
</template>
