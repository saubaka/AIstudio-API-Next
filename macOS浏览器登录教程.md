# macOS：Safari / Chrome 接入 Google 账户

本地工具入口：http://127.0.0.1:2048/ → 账户 → Google 账户登录。

2026-10-03，本机安装 Safari 27.0、Chrome 154.0.8037.93。Safari 当前中文菜单名称是“显示网页检查器”；Chrome 顶部 View 菜单的中文名称是“显示”。按键里的 ⌥ 是 Option，⌘ 是 Command。

## 最省步骤：Chrome 自动接入

如果只是想把账户接入本服务，不需要学习获取 Cookie：

1. 打开本地控制台，点击“账户 → Google 账户登录”。
2. 选择“打开本机浏览器”，浏览器选“Google Chrome”，点击“打开登录页面”。
3. 服务会打开本机已安装 Chrome 的专用登录窗口。在**这个新窗口**中自行登录 Google，进入 AI Studio 的 Playground 页面。日常 Chrome 里已经登录的账号不会自动复制到这个专用窗口。
4. 保留 Chrome 窗口，返回本地控制台，点击“完成登录并接入”。
5. 验证成功后，对话框关闭、账户列表出现邮箱。再点击“启动服务”同步模型目录。

必须使用控制台打开的专用窗口；自行打开普通 Chrome 标签页，应使用下方手动方式。

## Safari：复制一条请求，工具自动提取 Cookie

不需要安装浏览器扩展，不需要自己拆开 Cookie，不需要在终端执行 cURL。

### 1. 准备登录

本地控制台 → 账户 → Google 账户登录 → 打开本机浏览器 → Safari → 打开登录页面。

Safari 里选择需要接入的 Google 账号，完成登录，进入 https://aistudio.google.com/prompts/new_chat 。确认已经看到 Playground / 对话输入框。若有首次使用条款，需要自行阅读完成。

保留本地登录对话框，登录流程有效期为 15 分钟。Safari 已经登录的话不必退出 Google 重新登录；本地会话过期时重新打开本地登录入口即可。

### 2. 显示开发菜单

屏幕顶部“Safari 浏览器”菜单 → “设置…” → “高级” → 勾选“显示网页开发者功能”。已经有“开发”菜单的话，可以跳过。

旧版 Safari 的选项可能叫“在菜单栏中显示‘开发’菜单”。启用方式见 [Apple 官方说明](https://support.apple.com/zh-cn/guide/safari/sfri20948/mac)。

### 3. 打开 AI Studio 的网页检查器

先点击 Safari 中的 AI Studio 标签页，使它成为当前页面。然后选择屏幕顶部菜单“开发 → 显示网页检查器”，或按 **Option + Command + I（⌥⌘I）**。

检查器可能显示在 Safari 窗口底部，也可能单独显示在另一个窗口。这两种形式都可以。[Apple 网页检查器说明](https://developer.apple.com/documentation/safari-developer-tools/inspecting-safari-macos)

### 4. 网络 → 刷新

检查器顶部点“网络 / Network”。如果面板过窄，可以把窗口放大，或查看更多标签页菜单。

**先打开网络面板，再按 Command + R（⌘R）刷新 AI Studio 页面。** 下面应该出现多条请求记录。不要切换到“存储 / Storage”；此处需要的是实际发出的网络请求。[WebKit 网络面板说明](https://webkit.org/web-inspector/network-tab/)

### 5. 找到需要复制的那一行

网络列表的筛选框输入：

```text
aistudio.google.com
```

选择主页面的请求，名字通常为 `new_chat`，也可能带查询参数。点击后，在“标头 / Headers”里确认完整网址以 **`https://aistudio.google.com/`** 开头。

不要选择 `accounts.google.com` 登录请求，也不要选择 Google 字体、图片、YouTube 或其他域的请求。若筛选后没有结果，清空过滤框，再看请求的域名 / 完整 URL；当前标签页应确实是 AI Studio。

### 6. 在请求那一行上右键

右键点的是**网络面板请求列表里的那一行**，不是上面的网页空白处。

触控板可以双指轻点，也可以按住 Control 再点击。选择“复制为 cURL / 拷贝为 cURL / Copy as cURL”。不同语言或版本的文字可能略有区别。

选择的是复制**请求**，不是“复制响应 / Copy Response”。复制后，剪贴板里通常是一段以 `curl` 开头的文字，可能有很多行。

### 7. 返回本地控制台，整段粘贴

回到本地 Google 登录对话框，将刚才复制的整段文字粘贴到“粘贴浏览器复制的 cURL、Cookie 或会话 JSON”输入框。可以点击输入框按 ⌘V，然后点“检查粘贴内容”；也可以直接点“从剪贴板粘贴并检查”。

不需要删除 `curl`、引号、反斜线或其他请求头。工具会解析 cURL 文字，只取 Cookie；不会执行命令、读取文件或发送复制的请求。

如果浏览器拒绝读取剪贴板，使用 ⌘V 手动粘贴即可。

### 8. 看检查结果，再接入

- 提示“已识别 … 个 Cookie，登录所需字段齐全”：可以点“完成登录并接入”。
- 提示缺少字段：回 Safari，确认已经登录并打开 AI Studio，再刷新，重新复制。
- 提示不是 AI Studio 请求：选错了请求域名，回网络列表重选。
- 提示复制内容没有 Cookie：使用下面的备用方式。

“字段齐全”只说明内容格式可以用于验证，并不表示 Google 已通过验证。点击“完成登录并接入”后，服务才会验证会话、识别邮箱并保存账户。失败会给出错误提示。

## 备用：直接复制 Cookie 请求头

如果 cURL 没有包含 Cookie：

1. 仍然选中刚才那条 `aistudio.google.com` 请求。
2. 在详情中选择“标头 / Headers”。
3. 展开“请求标头 / Request Headers”。
4. 找到 `Cookie` 或 `cookie`，选中它后面的完整值，复制。
5. 粘贴到本地同一个输入框，点击“检查粘贴内容”。

完整值大致是这种格式，下方全是虚构值：

```text
SID=演示值; SAPISID=演示值; __Secure-1PAPISID=演示值; __Secure-3PAPISID=演示值; 其他名称=演示值
```

不要只复制某一项；不要复制响应标头中的 `Set-Cookie`。输入可以带 `Cookie:` 前缀，但不能带其他标头或自行添加的换行。界面上的自动折行不影响复制。

不要通过控制台执行 `document.cookie` 获取，因为它读不到 HttpOnly Cookie。[MDN 说明](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Set-Cookie#httponly)

## Chrome 已经登录，想使用手动方式

如果想使用普通 Chrome 窗口中已经登录的账号：

1. 本地控制台选择“仅获取登录链接”。
2. 在普通 Chrome 中打开并登录 AI Studio。
3. 按 ⌥⌘I 打开开发者工具，选择“Network”，再按 ⌘R 刷新。
4. 在请求筛选框输入 `aistudio.google.com`，选择对应主页面请求。
5. 右键该请求 → “复制 / Copy → 复制为 cURL / Copy as cURL”。若区分 bash 与 cmd，选择 bash。
6. 回本地控制台整段粘贴，点击“检查粘贴内容”，再完成接入。

Chrome 的网络面板提供复制为 cURL；不需要导出整个 HAR 文件。[Chrome 官方说明](https://developer.chrome.com/docs/devtools/network/reference/#copy)

## 常见卡点

| 看到的情况 | 下一步 |
| --- | --- |
| 开发菜单没有“显示网页检查器” | 确保在 Safari 的普通网页标签页，再启用高级设置中的开发者功能 |
| 网络里没有任何请求 | 保持检查器打开，回网页刷新；确认网络记录没有暂停 |
| 右键只有“检查元素”等网页菜单 | 点错地方了，应右键检查器请求列表中的行 |
| cURL 复制结果没有 Cookie | 先确认登录，再选择同域请求；使用 Cookie 请求头备用方式 |
| 缺少 `SAPISID`、`__Secure-1PAPISID` 或 `__Secure-3PAPISID` | 登录凭据不足或过期，确认账号后刷新 AI Studio，重新复制完整请求 |
| 显示“登录已过期或取消” | 重新打开本地登录对话框，再粘贴；不必因此退出 Safari 里的 Google 账号 |
| 连接超时 | 确保本机代理运行；Safari 使用自身网络设置，服务验证使用配置中的代理 |
| 无法识别邮箱 | 改用 Chrome 专用窗口的自动接入 |

Cookie 属于登录凭据，只粘贴到自己的本地控制台，不要发送给聊天或分享截图中的真实值。

## 本次验收

已核对本机安装版本及 Safari “开发”菜单；Chrome 顶部中文 View 菜单为“显示”。Safari 原生检查器截图接口不可用，因此没有制作带真实 Cookie 的浏览器截图。复制操作依据官方浏览器工具文档；解析用模拟 Safari / Chrome cURL 文本验证。

本地实际界面已验证 cURL 整段解析、缺失字段反馈、教程切换、输入修改后结果清除和响应式布局；后端全部应用包 race 测试与前端构建通过。没有读取或导入真实 Google Cookie，没有新增账户。现有已登录会话的真实接入仍需按上面步骤完成。

工具接口：`POST /api/auth/google/inspect`，输入 `{ "data": "粘贴内容" }`，只返回格式、Cookie 数量和缺失字段；沿用管理端认证和同源限制，响应不缓存、不回显 Cookie 值。

部署前程序备份：`runtime/cookie-guide-backup/aistudio2api-before-paste-tool`。停止本地管理服务后复制回根目录程序，再启动即可回滚。
