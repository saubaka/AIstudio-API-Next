<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n, type TranslationKey } from '@/i18n'
import type { Locale, TabID } from '@/types'
import UiIcon, { type IconName } from './UiIcon.vue'
import UiSelect from './UiSelect.vue'
import SurfaceOutline from './SurfaceOutline.vue'

const props = defineProps<{
  currentTab: TabID
  navigation: { id: TabID; label: TranslationKey; icon: IconName }[]
  serviceState: string
  busy: boolean
  version: string
  accountCount: number
  modelCount: number
  adminUsername?: string
}>()
const emit = defineEmits<{ navigate: [tab: TabID]; start: []; stop: []; logout: [] }>()
const { t, locale, availableLocales, setLocale } = useI18n()
const sidebarExpanded = ref(true)
const mobile = ref(false)
const menuOpen = ref(false)
const menu = ref<HTMLElement>()
const menuTrigger = ref<HTMLButtonElement>()
const content = ref<HTMLElement>()
const navigationElement = ref<HTMLElement>()
const selectionGeometry = ref({ x: 0, y: 0, width: 0, height: 0 })
const panelDirection = ref('forward')
let navigationObserver: ResizeObserver | undefined
const desktopPreference = ref<boolean | null>(null)
let breakpoint: MediaQueryList | undefined
let widthBreakpoint: MediaQueryList | undefined
const currentTitle = computed(() =>
  t(props.navigation.find((item) => item.id === props.currentTab)!.label),
)
const stateText = computed(() => {
  const cn = locale.value === 'zh-CN'
  return (
    {
      running: cn ? '运行中' : 'Running',
      launching: cn ? '正在启动' : 'Starting',
      stopped: cn ? '已停止' : 'Stopped',
      unavailable: cn ? '连接中断' : 'Disconnected',
    }[props.serviceState] ?? props.serviceState
  )
})
const menuLabel = computed(() => (locale.value === 'zh-CN' ? '菜单' : 'Menu'))

function syncLayout(): void {
  mobile.value = breakpoint?.matches ?? false
  sidebarExpanded.value = desktopPreference.value ?? widthBreakpoint?.matches ?? true
  if (!mobile.value) menuOpen.value = false
}
function measureSelection(): void {
  const selected = navigationElement.value?.querySelector<HTMLElement>('[aria-current="page"]')
  if (!selected) return
  selectionGeometry.value = {
    x: selected.offsetLeft,
    y: selected.offsetTop,
    width: selected.offsetWidth,
    height: selected.offsetHeight,
  }
}
watch(navigationElement, (element) => {
  navigationObserver?.disconnect()
  if (!element) return
  navigationObserver = new ResizeObserver(measureSelection)
  navigationObserver.observe(element)
  measureSelection()
})
watch(
  () => props.currentTab,
  async (next, previous) => {
    panelDirection.value =
      props.navigation.findIndex((item) => item.id === next) <
      props.navigation.findIndex((item) => item.id === previous)
        ? 'backward'
        : 'forward'
    await nextTick()
    measureSelection()
  },
)
function toggleSidebar(): void {
  desktopPreference.value = !sidebarExpanded.value
  sidebarExpanded.value = desktopPreference.value
  localStorage.setItem('aistudio2api_sidebar_expanded', String(sidebarExpanded.value))
}
async function navigate(tab: TabID): Promise<void> {
  menuOpen.value = false
  emit('navigate', tab)
  await nextTick()
  content.value?.focus({ preventScroll: true })
}
async function closeMenu(): Promise<void> {
  menuOpen.value = false
  await nextTick()
  menuTrigger.value?.focus()
}
function trapMenu(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    event.preventDefault()
    void closeMenu()
    return
  }
  if (event.key !== 'Tab') return
  const items = Array.from(
    menu.value?.querySelectorAll<HTMLElement>('button:not(:disabled), [tabindex="0"]') ?? [],
  )
  const first = items[0],
    last = items[items.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last?.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first?.focus()
  }
}
watch(menuOpen, async (open) => {
  await nextTick()
  if (open) menu.value?.querySelector<HTMLButtonElement>('button')?.focus()
})
onMounted(() => {
  const saved = localStorage.getItem('aistudio2api_sidebar_expanded')
  if (saved === 'true' || saved === 'false') desktopPreference.value = saved === 'true'
  breakpoint = matchMedia('(max-width: 900px)')
  widthBreakpoint = matchMedia('(min-width: 1180px)')
  breakpoint.addEventListener('change', syncLayout)
  widthBreakpoint.addEventListener('change', syncLayout)
  syncLayout()
})
onUnmounted(() => {
  navigationObserver?.disconnect()
  breakpoint?.removeEventListener('change', syncLayout)
  widthBreakpoint?.removeEventListener('change', syncLayout)
})
</script>

<template>
  <div
    class="studio-workspace"
    :data-panel-direction="panelDirection"
    :class="{ 'sidebar-expanded': sidebarExpanded, 'is-mobile': mobile }"
  >
    <a class="skip-link" href="#main-content">{{
      locale === 'zh-CN' ? '跳到主内容' : 'Skip to content'
    }}</a>
    <header class="workspace-header" :inert="menuOpen">
      <button
        class="sidebar-toggle"
        type="button"
        :aria-expanded="sidebarExpanded"
        aria-controls="studio-navigation"
        :aria-label="
          locale === 'zh-CN' ? (sidebarExpanded ? '折叠菜单' : '展开菜单') : 'Toggle navigation'
        "
        @click="toggleSidebar"
      >
        <UiIcon name="chevronRight" :size="18" :class="{ 'rotate-180': sidebarExpanded }" />
      </button>
      <h1 class="workspace-brand">aistudio2api</h1>
      <UiSelect
        :model-value="locale"
        :aria-label="locale === 'zh-CN' ? '界面语言' : 'Language'"
        class="workspace-language"
        @update:model-value="setLocale($event as Locale)"
      >
        <option v-for="item in availableLocales" :key="item.code" :value="item.code">
          {{ item.label }}
        </option>
      </UiSelect>
    </header>
    <div class="workspace-body">
      <Transition name="dock-menu">
        <aside
          v-if="!mobile || menuOpen"
          id="studio-navigation"
          ref="menu"
          class="workspace-sidebar"
          :class="{ 'mobile-menu': mobile }"
          :role="mobile ? 'dialog' : undefined"
          :aria-modal="mobile ? true : undefined"
          :aria-label="menuLabel"
          @keydown="mobile && trapMenu($event)"
        >
          <div v-if="mobile" class="mobile-menu-heading">
            <span>{{ menuLabel }}</span
            ><button
              type="button"
              class="icon-button"
              :aria-label="t('common.close')"
              @click="closeMenu"
            >
              <UiIcon name="close" :size="16" />
            </button>
          </div>
          <div class="sidebar-scroll">
            <button
              type="button"
              class="service-control"
              :class="{ 'is-stop': serviceState === 'running' || serviceState === 'launching' }"
              :disabled="busy || serviceState === 'unavailable'"
              :aria-busy="busy"
              :title="serviceState === 'stopped' ? t('app.start') : t('app.stop')"
              @click="serviceState === 'stopped' ? emit('start') : emit('stop')"
            >
              <UiIcon :name="serviceState === 'stopped' ? 'play' : 'stop'" :size="20" />
              <span class="sidebar-label">{{
                serviceState === 'stopped' ? t('app.start') : t('app.stop')
              }}</span>
            </button>
            <nav
              ref="navigationElement"
              :aria-label="t('app.console')"
              class="workspace-navigation"
            >
              <div
                class="navigation-highlight"
                aria-hidden="true"
                :style="{
                  width: `${selectionGeometry.width}px`,
                  height: `${selectionGeometry.height}px`,
                  transform: `translate(${selectionGeometry.x}px, ${selectionGeometry.y}px)`,
                  opacity: selectionGeometry.width ? 1 : 0,
                }"
              >
                <SurfaceOutline />
              </div>
              <button
                v-for="item in navigation"
                :key="item.id"
                type="button"
                class="navigation-item"
                :aria-current="currentTab === item.id ? 'page' : undefined"
                :aria-label="t(item.label)"
                :title="t(item.label)"
                @click="navigate(item.id)"
              >
                <UiIcon :name="item.icon" :size="19" /><span class="sidebar-label">{{
                  t(item.label)
                }}</span>
              </button>
            </nav>
            <dl class="sidebar-metrics">
              <div>
                <dt>{{ t('metric.totalAccounts') }}</dt>
                <dd>{{ accountCount }}</dd>
              </div>
              <div>
                <dt>{{ t('metric.models') }}</dt>
                <dd>{{ modelCount }}</dd>
              </div>
            </dl>
          </div>
          <div class="sidebar-status">
            <span class="service-dot" :data-state="serviceState" :title="stateText"></span>
            <div class="sidebar-label">
              <span>{{ t('app.status') }}</span
              ><strong>{{ stateText }}</strong>
            </div>
            <button
              v-if="adminUsername"
              class="icon-button"
              type="button"
              :title="`${adminUsername} · ${t('auth.logout')}`"
              :aria-label="t('auth.logout')"
              @click="emit('logout')"
            >
              <UiIcon name="login" :size="16" />
            </button>
          </div>
        </aside>
      </Transition>
      <main
        id="main-content"
        ref="content"
        tabindex="-1"
        class="workspace-content"
        :inert="menuOpen"
        :aria-label="currentTitle"
      >
        <slot />
      </main>
    </div>
    <footer class="workspace-footer" :inert="menuOpen">
      AIStudio2API <span>{{ version }}</span>
    </footer>
    <template v-if="mobile">
      <button
        v-if="menuOpen"
        class="dock-backdrop"
        type="button"
        tabindex="-1"
        :aria-label="t('common.close')"
        @click="closeMenu"
      ></button>
      <nav class="bottom-dock" :aria-label="t('app.console')" :inert="menuOpen">
        <button
          type="button"
          class="dock-item"
          :aria-current="currentTab === 'playground' ? 'page' : undefined"
          @click="navigate('playground')"
        >
          <SurfaceOutline v-if="currentTab === 'playground'" />{{ t('nav.playground') }}
        </button>
        <button
          type="button"
          class="dock-item"
          :aria-current="currentTab === 'build' ? 'page' : undefined"
          @click="navigate('build')"
        >
          <SurfaceOutline v-if="currentTab === 'build'" />{{ t('nav.build') }}
        </button>
        <button
          ref="menuTrigger"
          type="button"
          class="dock-item"
          :aria-expanded="menuOpen"
          aria-controls="studio-navigation"
          @click="menuOpen = true"
        >
          {{ menuLabel }}
        </button>
      </nav>
    </template>
    <div class="sr-only" role="status" aria-live="polite">{{ currentTitle }} · {{ stateText }}</div>
  </div>
</template>
