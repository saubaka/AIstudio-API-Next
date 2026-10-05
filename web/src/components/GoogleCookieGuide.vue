<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from '@/i18n'

const props = defineProps<{ browser: string }>()
const guide = ref(props.browser === 'chrome' ? 'chrome' : 'safari')
const { locale } = useI18n()
const text = (cn: string, en: string) => (locale.value === 'zh-CN' ? cn : en)
</script>

<template>
  <details class="google-login-help" open>
    <summary>
      {{
        text(
          '跟着浏览器步骤复制（无需整理 Cookie）',
          'Copy from your browser, without editing cookies',
        )
      }}
    </summary>
    <fieldset class="google-guide-browser">
      <legend class="sr-only">{{ text('教程浏览器', 'Tutorial browser') }}</legend>
      <label><input v-model="guide" type="radio" value="safari" />Safari</label>
      <label><input v-model="guide" type="radio" value="chrome" />Chrome</label>
    </fieldset>
    <ol>
      <li>
        {{
          text(
            '在这个浏览器里登录 Google，并进入 AI Studio 的 Playground 对话页面。',
            'Sign in to Google in this browser and open the AI Studio Playground.',
          )
        }}
      </li>
      <li v-if="guide === 'safari'">
        {{
          text(
            'Safari 浏览器 → 设置 → 高级，勾选“显示网页开发者功能”。随后选择顶部菜单“开发 → 显示网页检查器”（⌥⌘I）。',
            'Safari → Settings → Advanced → Show features for web developers. Then choose Develop → Show Web Inspector (⌥⌘I).',
          )
        }}
      </li>
      <li v-else>
        {{
          text(
            '在 Chrome 的 AI Studio 页面按 ⌥⌘I 打开开发者工具。中文菜单位于顶部“显示”，英文为 View。',
            'Chrome → View → Developer → Developer Tools, or press ⌥⌘I.',
          )
        }}
      </li>
      <li>
        {{
          text(
            '在检查器中点击“网络 / Network”，再按 ⌘R 刷新 AI Studio。网络面板是请求列表，不是“存储 / Storage”。',
            'Select Network in the inspector, then press ⌘R to reload AI Studio. Use the request list, not Storage.',
          )
        }}
      </li>
      <li>
        {{
          text(
            '在网络列表的筛选框输入 aistudio.google.com。选择主页面请求（通常叫 new_chat），确认网址以 https://aistudio.google.com/ 开头。',
            'Filter the request list by aistudio.google.com. Choose the main page request (usually new_chat) and confirm its URL starts with https://aistudio.google.com/.',
          )
        }}
      </li>
      <li v-if="guide === 'safari'">
        {{
          text(
            '在这条请求上右键，选择“复制为 cURL / Copy as cURL”。不要选“复制响应”。',
            'Right-click that request and choose Copy as cURL, not Copy Response.',
          )
        }}
      </li>
      <li v-else>
        {{
          text(
            '在这条请求上右键，选择“复制 / Copy → 复制为 cURL / Copy as cURL”（若有选项，选 bash）。',
            'Right-click the request and select Copy → Copy as cURL (bash, if a format is offered).',
          )
        }}
      </li>
      <li>
        {{
          text(
            '返回这里，整段粘贴到下方，点击“检查粘贴内容”。出现字段齐全提示后，再点击“完成登录并接入”。不需要把 cURL 放到终端运行。',
            'Paste the entire copied text below and click Check pasted text. Once the required fields are present, click Finish sign-in. You do not need to run cURL in Terminal.',
          )
        }}
      </li>
    </ol>
    <details class="google-guide-trouble">
      <summary>
        {{
          text('找不到请求或复制结果没有 Cookie？', 'No request or no Cookie in the copied text?')
        }}
      </summary>
      <p>
        {{
          text(
            '先打开“网络”，再刷新；如果过滤后没有结果，清空过滤框，并选择域名为 aistudio.google.com 的请求。选中后查看“标头 / Headers → 请求标头 / Request Headers → Cookie”，也可复制 Cookie 的完整值。',
            'Open Network before reloading. If the filter hides everything, clear it and select a request whose domain is aistudio.google.com. You can also copy the full Cookie value from Headers → Request Headers → Cookie.',
          )
        }}
      </p>
      <p>
        {{
          text(
            'Safari 面板可能在窗口底部或单独窗口。右键应点在请求列表的那一行，而不是网页内容。复制内容只粘贴到本机控制台，不要发送到聊天里。',
            'Safari may dock the inspector at the bottom or use a separate window. Right-click a row in the request list, not the webpage. Paste credentials only into your local console.',
          )
        }}
      </p>
    </details>
  </details>
</template>
