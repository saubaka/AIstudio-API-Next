# AIstudio API Next

[English](README_en.md) · [更新记录](CHANGELOG.md) · [MIT 许可证](LICENSE)

更好的 AI Studio 反向代理：将 **Playground 与 App Build 分成两个独立 API 渠道**，同时提供工具调用、流式输出、账户管理和网页控制台，方便接入 Codex 等客户端。

本项目基于 [Mag1cFall/AIStudio2API](https://github.com/Mag1cFall/AIStudio2API) 的源码继续开发，是独立维护的衍生版本。感谢原作者 Mag1cFall 和上游贡献者提供的协议适配与运行时基础。重建 Git 历史不改变代码来源及原有版权归属。

## 主要功能

- **双渠道独立接入**：Playground 与 App Build 有各自的入口、模型目录和试用工作区。固定渠道请求不会因额度不足而偷偷切换到另一渠道。
- **多种兼容协议**：支持 OpenAI Chat Completions、Responses、Anthropic Messages 和 Gemini GenerateContent，提供流式与非流式响应。
- **工具调用与 Codex 接入**：支持函数工具、custom 工具、namespace、动态工具发现及常见 JSON Schema 引用适配，保留多轮工具调用历史。
- **本机浏览器登录**：使用已安装的 Chrome 等浏览器完成登录；Safari 或自行打开登录链接时，可粘贴完整 cURL、Cookie 或会话 JSON 接入。
- **媒体与文件**：提供本地文件上传，控制台可展示模型返回的图片、音频和视频；具体生成能力以当前渠道的模型目录为准。
- **请求记录与用量**：显示输入、输出和上游实际返回的缓存 token，记录保存在服务端，重新打开浏览器仍可查看。没有上游缓存数据时不会伪造命中数。
- **账户与运行管理**：在网页中管理账户、代理、模型、请求、日志和服务启停；支持纯 Go WAA 后端，也保留 Camoufox 后端。

Build 不提供 CountTokens、Live、Robotics、Veo、转录和 Interactions 等 Playground 专用入口。具体范围见[双渠道说明](渠道重构说明.md)。可用模型、额度、缓存行为及账户资格由上游决定。

## 技术栈

| 部分 | 技术 |
| --- | --- |
| 后端 | Go、标准库 HTTP 服务、账户调度与协议适配 |
| 网页控制台 | Vue 3、TypeScript、Vite、Tailwind CSS |
| WAA 运行时 | 纯 Go 运行时及内嵌 JavaScript 引擎；可选 Camoufox |
| 开发与校验 | Go test / race / vet、ESLint、vue-tsc、媒体回归、Python 源码同步工具 |
| 构建与依赖 | Go Modules（go.mod / go.sum）、npm（package.json / package-lock.json） |

前端资源在编译时嵌入 Go 程序，运行时无需另起 Node.js 服务。为保持现有内部导入兼容，Go module 路径仍沿用上游名称，这不表示当前仓库由上游维护。

## 部署准备

本仓库发布纯源码，不附带安装好的依赖、可执行文件、账户、密钥或数据库。当前没有自动创建的 Release；以下步骤均从源码构建。

- 安装 Git、Go **1.25.0 或更高版本**、Node.js **22.13+ 的 22.x 或 24+**，以及随 Node.js 提供的 npm；具体要求以 `go.mod` 和 `web/package.json` 为准。
- macOS 后台启停脚本还需要 Python 3.9+。直接运行 Go 程序不需要 Python。
- 服务和登录浏览器需要能够访问 Google AI Studio。按自己的网络情况配置代理，不必照搬开发机地址。
- 首先复制配置样例，再编辑自己的 `.env`。建议生成随机 API 密钥，例如 `openssl rand -hex 32`，不要把真实配置提交到 Git。

常用配置：

```dotenv
LISTEN_ADDR=127.0.0.1:2048
PROXY_API_KEY=填入你自己生成的随机密钥
AISTUDIO_AUTH_STATES=auth
WAA_BACKEND=go
PROXY=
ADMIN_AUTH_ENABLED=false
```

这里选择纯 Go WAA 后端，运行时无需下载或启动 Camoufox。需要该后端时再设置 `WAA_BACKEND=camoufox`，并准备浏览器及所需系统运行库。

### macOS（Apple Silicon / Intel）

```bash
git clone https://github.com/saubaka/AIstudio-API-Next.git
cd AIstudio-API-Next
cp .env.example .env
# 编辑 .env，填写 API 密钥、WAA_BACKEND=go 和必要的代理。
cd web
npm ci
npm run build
cd ..
go build -mod=readonly -buildvcs=false -trimpath -o aistudio2api ./cmd/aistudio2api
python3 local-service.py start --no-open
```

打开 <http://127.0.0.1:2048>。也可以双击 `启动.command`；停止使用 `停止.command`，或执行 `python3 local-service.py stop`。脚本在后台运行管理进程，关闭终端或浏览器后服务继续运行；没有默认配置开机自启。

不使用后台脚本时，直接执行 `./aistudio2api -open-ui=false`，用 Ctrl+C 退出。

### Windows（Windows 10 / 11，x64）

安装工具后，在 PowerShell 中执行：

```powershell
git clone https://github.com/saubaka/AIstudio-API-Next.git
Set-Location AIstudio-API-Next
Copy-Item .env.example .env
notepad .env
.\start.bat
```

首次运行 `start.bat` 时，若没有可执行文件，会自动安装前端依赖、构建网页并编译 Go 程序。以后运行会复用已有程序；改动源码后应重新构建：

```powershell
Set-Location web
npm ci
npm run build
Set-Location ..
go build -mod=readonly -trimpath -o aistudio2api.exe ./cmd/aistudio2api
.\aistudio2api.exe -open-ui=false
```

打开同一个本地控制台地址，用 Ctrl+C 停止前台进程。Chrome 自动接入依赖本机安装的受支持浏览器；也可以使用手动会话导入。

### Linux（amd64 / arm64）

```bash
git clone https://github.com/saubaka/AIstudio-API-Next.git
cd AIstudio-API-Next
cp .env.example .env
# 编辑 .env，选择 WAA_BACKEND=go 并填写自己的配置。
cd web
npm ci
npm run build
cd ..
go build -mod=readonly -buildvcs=false -trimpath -o aistudio2api ./cmd/aistudio2api
./aistudio2api -open-ui=false
```

桌面环境打开 <http://127.0.0.1:2048>。无桌面的服务器可以通过 SSH 转发访问控制台：

```bash
ssh -L 2048:127.0.0.1:2048 用户名@服务器地址
```

随后在本机浏览器访问上述地址。浏览器自动登录只操作服务所在机器的浏览器；无桌面服务器使用登录链接及手动会话导入。需要长期运行时，可自行配置 systemd，设置工作目录为项目目录、启动命令为程序绝对路径，并让服务用户有权读写 `.env`、`auth/`、`runtime/`。

仅使用纯 Go 后端时不需要 Camoufox 的图形运行库。若选择 Camoufox，按系统准备 Firefox 所需运行库；上游的[部署文档](https://github.com/Mag1cFall/AIStudio2API#安装步骤)可供参考，但账户接入以本项目流程为准。

### 部署验证与更新

macOS ARM64 已在开发环境完成构建和测试。Windows、Linux、macOS Intel 的说明提供对应源码构建方法；交叉编译成功不等于这些平台已经实际运行验收。GitHub Actions 会执行源码检查、前端构建、Go 测试及多平台交叉编译，构建附件在 Actions 中获取，不自动创建标签或 Release。

更新前停止程序，保留 `.env`、`auth/` 和 `runtime/`，拉取新源码后重新构建前端与 Go 程序。不要直接用原上游的发布包替换本项目程序，否则可能丢失本项目功能。

## 登录与启动生成服务

1. 启动管理进程，进入控制台的“账户”页面，点击“Google 账户登录”。
2. Chrome 等受支持浏览器可以使用专用登录窗口；Safari 或仅获取链接时，在 AI Studio 页面的网络面板复制完整 cURL，返回控制台检查并导入。
3. 多账号登录时，保留 cURL 的 `/u/N/`、`authuser` 或 `X-Goog-AuthUser`，确保接入所选账户。
4. 验证账户成功后，点击“启动服务”，等待生成服务就绪，再在对应渠道查看模型并发送请求。

管理进程启动后显示 `STOPPED` 属于正常状态，表示生成服务尚未启动。不要使用 `document.cookie` 导出登录会话，它不包含 HttpOnly Cookie。

详细步骤见[Safari / Chrome 登录教程](macOS浏览器登录教程.md)及[Google 登录说明](Google登录重构说明.md)。

## API 接入

| 客户端协议 | Playground | App Build |
| --- | --- | --- |
| OpenAI / Responses Base URL | `http://127.0.0.1:2048/playground/v1` | `http://127.0.0.1:2048/build/v1` |
| Anthropic / Gemini Base URL | `http://127.0.0.1:2048/playground` | `http://127.0.0.1:2048/build` |
| OpenAI 模型列表 | `/playground/v1/models` | `/build/v1/models` |

API 密钥使用 `.env` 中的 `PROXY_API_KEY`；模型名称从当前账户的对应渠道目录读取。两个固定入口共用账户配置，但保持渠道隔离。旧 `/v1`、`/v1beta` 地址仍保留兼容调度，需要明确渠道时请使用上表地址。

Codex 通过 Responses 协议接入，配置、工具调用、缓存统计及媒体上传示例见[Codex 接入说明](Codex接入与媒体上传说明.md)。程序不绕过上游账户权限和额度，也不保证每次请求都命中缓存。

## 测试与源码维护

前端必须先构建，Go 才能嵌入完整网页资源：

```bash
cd web
npm ci
npm run lint
npm run build
node qa/media-regression.mjs
cd ..
go test -race -mod=readonly ./cmd/... ./internal/...
go vet ./cmd/... ./internal/...
python3 -B -m unittest discover -s scripts/tests -v
```

当前版本以 [VERSION](VERSION) 为准，历史记录见 [CHANGELOG.md](CHANGELOG.md)。修复及文档调整递增修订号，兼容功能递增次版本号，不兼容改动递增主版本号；每轮实际更改只递增一次。

维护者的开发目录是唯一源码来源，`bakagit/` 由同步工具生成，不手动维护两份代码。上传时以其中内容作为仓库根目录，所以在 GitHub 中看不到额外嵌套的 `bakagit` 文件夹。详细同步和发布步骤见[源码发布说明](源码发布说明.md)。

提交标题只写 `vX.Y.Z`，正文使用以 `- ` 开头的简体中文要点，概括实际更改和验证结果。提交、推送、创建仓库和部署仍需维护者明确授权，不自动创建标签或 Release。

## 来源、版权与许可

- **原始项目**：[Mag1cFall/AIStudio2API](https://github.com/Mag1cFall/AIStudio2API)。原始代码遵循 MIT 许可证，`Copyright (c) 2026 Mag1cFall` 及完整许可文本保留在 [LICENSE](LICENSE) 中。
- **本项目修改**：由 [saubaka](https://github.com/saubaka) 及本仓库贡献者维护，新增及修改部分继续按 MIT 许可证发布，不主张原始代码的独占版权。上游并不负责本仓库的新增功能或维护。
- **第三方组件**：保留各自许可和版权声明，包括内嵌 [goja](internal/waa/goja/LICENSE)、[Lucene 相关代码](internal/waa/goja/ftoa/LICENSE_LUCENE)与 [V8 相关代码](internal/waa/goja/ftoa/internal/fast/LICENSE_V8)。依赖和外部运行时按各自许可证使用，本仓库不重新授予其版权。
- **视觉参考**：控制台样式参考维护者的 BakaMail（`code/codedx/8`），实现说明见 [UI 重构说明](UI重构说明.md)。
- **服务与数据**：Google、AI Studio、Gemini 等名称及商标属于各自权利人；本项目不是 Google 官方产品。MIT 许可针对本项目代码，不授予第三方服务使用权、账户权限或用户内容版权。

重新分发时请保留适用的版权及许可声明。Cookie、API 密钥、账户资料、请求记录和上传文件不属于发布源码，不应上传到公开仓库。
