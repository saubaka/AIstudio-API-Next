# Google 账户登录（本机浏览器）

2026-10-02 已更新本地部署：http://127.0.0.1:2048 。在“账户”页面点击“Google 账户登录”，也可双击 `登录账户.command`。

2026-10-03 新增整段 cURL 粘贴解析、剪贴板粘贴检查和 Safari / Chrome 分步引导。可以直接粘贴浏览器“复制为 cURL”的全部文字，不需要自己提取 Cookie；详情见 `macOS浏览器登录教程.md`。检查格式不等同于真实登录验证。

2026-10-03 修复 Safari cURL 检查通过但接入时无法识别邮箱的问题。支持页面邮箱的 JSON 转义；页面缺少账户元数据时，使用同一会话和账户代理查询 Google ListAccounts，兼容当前 Base64 protobuf 与旧 JSON 响应。2026-10-05 再修复多账号选择：保留 cURL 的 `/u/N/`、`authuser=N` 或 `X-Goog-AuthUser`，将同一编号用于身份验证、预热、Playground 和 Build 生成；默认未指定时为 0。拒绝已退出、过期或未验证的所选账户，不根据页面文本或用户填写的邮箱猜测身份。

重复导入已验证的相同邮箱会更新原账户会话，保留已有配置及目录，不再尝试创建同名目录而报 `file exists`。会话 JSON 导出再导入也保留所选账号编号，但不信任导入文件中的邮箱或 OAuth 扩展。真实 Safari 原有 `/u/2/` cURL 已成功接入第二个账户；真实重复导入验证账户数保持 2、配置保持不变，双渠道推理及 Codex 工具验收见 `Codex接入与媒体上传说明.md`。

2026-10-03 进一步确认并修复实际根因：`FdrFJe` 是会话编号，邮箱字段是 `oPEP7c`。当前 AI Studio 把 bootstrap 元数据放在随机 ID 的 `application/json` 脚本容器中，不再直接赋值 `window.WIZ_global_data`。解析器现在兼容两种格式；JSON 容器还需匹配 MakerSuite 应用、公开 API key 与会话编号字段，避免读取任意页面文本、提示词或其他 JSON 中的邮箱。AI Studio 元数据有效时，不以其他 Google 站点的账号列表为空否定该会话。

## 使用方式

### Chrome

选择“打开本机浏览器”及 Google Chrome，点击“打开登录页面”。服务调用本机已安装的 Chrome，创建独立登录资料目录，不读取日常 Chrome 资料。

在打开的窗口自行完成 Google 登录，进入可使用的 AI Studio 页面，然后回到控制台点击“完成登录并接入”。服务读取此专用窗口的会话，验证 Google 邮箱、AI Studio 页面及模型端点，成功后保存账户并关闭专用窗口。重新登录已有账户时，邮箱必须与目标账户一致。

Chrome 专用窗口使用账户代理；留空时继承服务默认代理。本机目前默认代理为 `http://127.0.0.1:10808`。

### Safari / 仅获取登录链接

Safari 调用已安装的系统浏览器打开 Google 官方登录页；“仅获取登录链接”只显示可复制的官方链接，不启动浏览器。

普通 Google 登录页不会把浏览器 Cookie 自动回传给本服务。这两种方式需要在登录并进入 AI Studio 后，手动接入会话：

1. 打开浏览器开发者工具的“网络”，刷新 AI Studio 页面。
2. 选择发往 `aistudio.google.com` 的请求，复制请求头 `Cookie` 的完整值。
3. 返回控制台粘贴该值，或选择已导出的 Cookie 数组 / storage state JSON，然后点击“完成登录并接入”。

不要使用 `document.cookie`，它不包含 HttpOnly Cookie。Safari 使用自身的网络和代理设置；控制台的账户代理用于服务验证和后续请求，也需能够访问 `accounts.google.com`。多账号登录时，复制目标账户 AI Studio 页面的完整 cURL，并保留网址中的 `/u/N/` 或账号选择头；无需为邮箱识别切换 Chrome。

验证通过后账户才会加入列表。账户接入后点击“启动服务”，开始同步模型目录并提供生成 API。停止生成服务时，接入及验证账户不会启动生成 Worker。

## 会话与兼容性

- 本机自动检测 Chrome、Safari、Edge、Firefox、Brave；仅显示已安装的浏览器。Chrome / Edge / Brave 支持专用窗口自动接入；Safari / Firefox 使用手动接入。本机实际检测到 Chrome 与 Safari。
- 登录会话有效期 15 分钟，最多同时保留 4 个。取消、成功、到期或退出管理进程时清理专用窗口及临时资料目录。Safari 窗口由用户管理。
- 手动数据限 512 KB，仅接收 Google 域的会话 Cookie；不保存第三方域 Cookie 或导入文件内自称可信的认证标记。会话输入不写入浏览器本地存储，退出对话框时清空。
- 新接口沿用管理端认证及同源检查。日志不记录 Cookie，接口只返回账户资料。
- 此次替换的是账户交互登录和账户验证；生成 API 的 WAA 后端沿用项目配置，本机已于 2026-10-03 切换为纯 Go，生成服务和账户登录都无需启动 Camoufox。
- `setup --login` 现在显示控制台入口和官方登录链接，不再启动 Camoufox 登录窗口。

新接口：`GET /api/auth/google/options`、`POST /api/auth/google/start`、`POST /api/auth/google/{session}/complete`、`DELETE /api/auth/google/{session}`。原 `POST /api/accounts` 和 `POST /api/accounts/{id}/login` 登录接口返回 410，提示使用新流程。

## 验收与边界

通过前端格式、ESLint、类型及生产构建检查；全部应用包测试 `go test ./cmd/... ./internal/...`，以及登录、HTTP 验证、管理 API、运行时包的 race 测试通过。测试覆盖会话格式、域过滤、过期 Cookie、到期清理、取消、会话上限、账号不匹配、管理员鉴权和同源限制。

真实本机验收：从控制台打开 Chrome 后观察到 Google 官方登录页；未登录点击接入得到错误提示，未创建账户；取消后专用资料目录为空。Safari 实际打开官方账户选择页。官方链接在控制台复制成功；无效输入返回 400、取消返回 204、取消后接入返回 410。手机宽度的手动接入布局和模拟成功反馈另经 fixture 检查。

2026-10-03 邮箱识别修复验收：全应用包 race 测试通过，覆盖两种 AI Studio bootstrap 格式、随机 JSON 容器 ID、邮箱转义、任意提示词和其他 JSON 中的邮箱拒绝、两种 ListAccounts 协议、多账号顺序、过期会话、登录跳转和 Cookie 轮换。另通过实际 HTTP 传输测试，确认 Cookie 和 POST 请求体完整发送。

真实验收：控制台原有 cURL 在新 JSON 元数据解析修复后成功通过 AI Studio 页面和模型端点验证，账户已保存为可用状态。此前认为 Cookie 失效、要求重新复制的结论已更正；实际问题是服务解析格式错误。没有替用户输入 Google 密码，也没有把 Cookie 或 cURL 写入诊断或验收文件。该次验收只验证了账户接入；后续生成服务验收见下文。

验收文件：`runtime/google-login-qa/live-check.json`；实际界面：`runtime/google-login-qa/live-picker.png`、`live-link.png`；本机 Chrome：`chrome-native.png`；手机 fixture：`fixture-link-mobile.png`。

## 回滚

重构前部署程序保存在 `runtime/login-backup/aistudio2api-before-native-login`。需要回滚时停止本地管理进程，将此文件复制为根目录 `aistudio2api`，然后重新启动。回滚不修改 `.env`、已有 `auth/` 或源代码；再次构建当前源代码会恢复本机登录功能。

邮箱识别修复前的程序另存于 `runtime/identity-backup/aistudio2api-before-identity-fix`。

## 生成服务启动修复（2026-10-03）

“没有符合条件的 AI Studio 账户”的实际原因是验证账户清空实时模型目录，与启动预热并发时导致账户无法被选中。验证接口现在直接发布刚通过认证的 ListModels 目录，不再写入空目录；同时添加启动中验证账户的回归测试，覆盖原目录为空和已有缓存两种情况。

本机另有 Camoufox 等待官网 Run 按钮超时的问题；已改用项目内置的纯 Go WAA 后端，并保存至 `.env`。新版程序部署并重启后，启动与验证账户同时执行成功，账户仍有 42 个模型，服务进入 RUNNING。使用真实会话调用 `gemini-flash-latest` 得到 HTTP 200 和 `OK`，已验证模型推理。无需重新复制 Cookie 或重新登录。
