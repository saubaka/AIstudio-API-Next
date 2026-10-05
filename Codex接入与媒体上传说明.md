# Codex 接入与媒体上传

2026-10-05 已在本机部署修复版本。Playground 与 App Build 均可完成真实 Codex 工具调用、执行结果回传，以及图片、音频、视频、文本和 PDF 文件输入。

## Codex 配置

### 切换模型后的搜索历史修复

2026-10-05 已部署修复：`unsupported input item type "web_search_call"` 来自服务端只接受消息和本地函数调用，却拒绝旧回答中的托管搜索记录。Codex 恢复旧会话或切换模型后，会把这些记录再次放入 `input`；错误在上游生成开始前发生。

现在接受 `web_search_call`、`code_interpreter_call` 和 `image_generation_call` 历史。搜索动作、查询、来源、状态和执行日志保留为历史上下文；生成图片保留实际像素。历史记录不会被当成新的本地工具调用执行，也不会隐式启用已经关闭的搜索工具。显式回传历史和 `previous_response_id` 两条路径均已修复。OpenAI 的[搜索工具文档](https://developers.openai.com/api/docs/guides/tools-web-search)说明了搜索输出中的动作与来源信息。

验收证据：

- 修复前相同请求返回 HTTP 400；修复后 Playground/Build 的流式、非流式以及跨模型续接共 6 个真实请求正常完成，并返回旧搜索查询中的标记。
- 回归覆盖 40 个搜索动作/状态/工具开关组合，以及 12 个根路由/渠道路由/显式历史/存储历史组合；`go test -race ./cmd/... ./internal/...` 通过。
- 使用桌面应用随附的 Codex CLI 0.160.0，在独立 `CODEX_HOME` 中创建原生会话，把本服务生成的 4 条真实搜索记录作为隔离历史夹具加入会话。恢复时由 Codex 自己重建和回传历史；本地转发层只将模型从 `gemini-3.8-flash` 切换为 `gemini-3.1-pro-preview`。随后真实完成读文件、freeform `apply_patch` 新建文件、文件内容比较和最终回答，4 轮续接请求均收到 `response.completed`。

上述原生恢复验证使用隔离历史夹具，未操作用户原桌面对话的模型菜单。原桌面对话的界面复测仍需用户在本轮结束后，在同一个窗口切换到该对话，切换模型并发送“只回复 DESKTOP_WEB_HISTORY_OK”；不需要同时打开两个窗口。

证据位于 `runtime/web-search-history-qa/` 和 `runtime/web-search-history-codex-resume-qa/playground-codex.json`；后者明确记录夹具来源、实际回传的输入类型、切换后的模型和工具调用。早先未触发真实搜索的普通 CLI 测试保留为失败证据，不计入验收。回滚程序为 `runtime/web-search-history-qa/aistudio2api-before-history-fix`。

可用以下命令重复原生恢复测试（会使用真实上游额度）：

```sh
QA_SUITE=web-search-history-codex-resume-qa QA_CHANNEL=playground \
QA_TARGET_MODEL=gemini-3.8-flash QA_WEB_SEARCH_PROBE=1 \
QA_SWITCH_AFTER_SEARCH_MODEL=gemini-3.1-pro-preview \
QA_RESUME_FROM_SEARCH_RESPONSE=runtime/web-search-history-qa/real-search-response.json \
python3 scripts/verify-codex.py
```

| 渠道 | OpenAI Base URL |
| --- | --- |
| Playground | `http://127.0.0.1:2048/playground/v1` |
| App Build | `http://127.0.0.1:2048/build/v1` |

API 密钥使用项目 `.env` 中已有的 `PROXY_API_KEY`，模型从对应渠道目录选择。以下配置以实测通过的 `gemini-flash-latest` 为例，合并到自己的 Codex 配置中：

```toml
model = "gemini-flash-latest"
model_provider = "aistudio_playground"

[model_providers.aistudio_playground]
name = "AIStudio Playground"
base_url = "http://127.0.0.1:2048/playground/v1"
wire_api = "responses"
env_key = "AISTUDIO_API_KEY"
```

在启动 Codex 的终端设置 `AISTUDIO_API_KEY` 为已有 API 密钥。切换 Build 时，把 provider 名称改为 `aistudio_build`，Base URL 改为 `http://127.0.0.1:2048/build/v1`。验收过程中使用独立的 `CODEX_HOME`，没有覆盖用户原有 Codex 配置。

2026-10-05 后续发现 CCSwitch 为 Gemini 生成的目录使用 `shell_command`，缺少 `apply_patch_tool_type`，会让 Codex 不提供专用补丁工具。现已安装 `~/.codex/models-aistudio.json`，启用 Gemini 的 `unified_exec` 和 freeform `apply_patch`，保留原有 OpenAI 模型，并保留 CCSwitch 中已有 Gemini 的上下文上限。已保存的本地 Playground 提供商指向这个目录，切换提供商时不会再使用缺少补丁工具的生成目录。

本轮工作结束后，请先重开 CCSwitch，再完全退出并重开 Codex，载入新模型目录。没有在工作途中重启正在使用的 Codex。API 中仍需选择支持工具调用的文本模型；本轮真实 Agent 验收模型是 `gemini-3.8-flash`。Codex 的 [model_catalog_json 配置](https://learn.chatgpt.com/docs/config-file/config-reference)在客户端启动时加载。

可重复安装目录修复（先备份配置和 CCSwitch 数据库，只修改本地 AIStudio 提供商的目录指针，不切换当前提供商）：

```sh
python3 scripts/configure-codex-gemini.py --install --ccswitch
```

## 缓存统计与调用历史

2026-10-05 已修复 Playground 数组协议和 Build JSON 协议的缓存用量读取。返回 Google 实际报告的缓存数，分别写入 Responses 的 `input_tokens_details.cached_tokens`、Chat 的 `prompt_tokens_details.cached_tokens`、Gemini 的 `cachedContentTokenCount` 和 Anthropic 的 `cache_read_input_tokens`。控制台现在显示输入、缓存、未缓存、思考、回复和总用量；上游未报告缓存时显示“上游未报告”，本地补算用量标记“估算用量”。

文件写入是工具执行；输入缓存命中是 Google 重用输入上下文，两者不是同一个统计。本服务未创建显式缓存，不伪造 `cache_write_input_tokens`。Google 的[隐式缓存文档](https://ai.google.dev/gemini-api/docs/caching)说明较大且稳定的提示词前缀、短时间内重复请求有助于命中，但不保证每次命中。缓存命中也不能保证不触发独立的请求次数限制；[限额文档](https://ai.google.dev/gemini-api/docs/rate-limits)列出 RPM、TPM、RPD 等维度，AI Studio 网页渠道的具体限额仍以实际上游错误为准。

真实验收使用本机 Codex CLI 0.160.0 和直接 Gemini 模型名，两个渠道均按 `AGENTS.md` 读取目标文件、调用 `apply_patch` 添加及修改文件，再用实际命令验证内容，未通过 shell EOF 写入。另用稳定的合成提示词验证缓存：Playground 的原生 Codex 用量为输入 26,411、缓存 24,341；Build 为输入 26,428、缓存 24,341。原生会话的 `token_count` 也记录了 `cached_input_tokens: 24341`，与 CCSwitch 会话导入器读取的字段一致。CCSwitch 当前关闭了本地代理，统计来源是 `codex_session`；旧记录中没有上游缓存证据的 0 不做猜测回填。本轮未切换 CCSwitch 当前的 OpenAI 提供商，也未把隔离验收会话导入用户历史。

调用记录由服务保存到 `runtime/request-history.jsonl`（权限 0600），新窗口会取得完整保留快照，重启管理服务后仍恢复。保留最近 2,000 条事件；界面默认展示最近 200 行，可搜索或加载更早日志。首次运行迁移旧 `service.log` 中已有的结构化请求记录。重启实测验证请求 ID、渠道及全部用量字段保持一致；原先服务日志里已经不存在的记录无法恢复。

证据：`runtime/account-agent-cache-final/*-direct-codex.json`、`runtime/account-agent-cache-qa/cache-live.json`、`runtime/account-agent-cache-qa/native-cache/acceptance.json`、`runtime/account-agent-cache-qa/native-cache/*-session-token-events.json`、`runtime/account-agent-cache-qa/history-restart-acceptance.json`。复测缓存会消耗真实上游额度：

```sh
python3 scripts/verify-cache.py
python3 scripts/verify-codex-cache.py
```

## 工具兼容修复

原来的适配只接受普通 function，并拒绝 custom、client tool_search 和部分 namespace 工具，也会拒绝 `strict: true`。现在支持普通函数、自由文本 custom 工具、namespace 工具名称映射、客户端工具发现和发现后的动态工具目录；多轮历史和 SSE 返回保留对应的调用类型、命名空间、调用 ID 与执行结果。

`apply_patch` 的自由文本输入通过 Gemini 字符串参数承载，再恢复为 Codex 的 `custom_tool_call`。原始语法描述保留在工具描述中，补丁由 Codex 客户端解析执行。`strict: true` 的 schema 可以传入，但这里不承诺 OpenAI 原生 strict 引擎或服务端 Lark 语法约束的等价执行。服务端只转发工具调用，工具实际在 Codex 客户端运行。

格式参考：[OpenAI function/custom tools](https://developers.openai.com/api/docs/guides/function-calling)、[client tool search](https://developers.openai.com/api/docs/guides/tools-tool-search)。

### 布尔与数字枚举修复及双渠道验收

同日后续修复：`function declaration 22 ... schema.properties.allowAsync: schema.enum 必须是字符串数组` 来自本地编码器把 Google 协议的字符串枚举表示误当成对客户端 JSON Schema 的类型要求。现在布尔、整数、小数枚举会转换为协议所需的字符串表示，参数的 `BOOLEAN` / `INTEGER` / `NUMBER` 类型保留，实际工具调用仍返回布尔和数字；没有删除枚举约束。布尔与数字 `const` 同时转换成相同类型的单值枚举，避免相邻字段再次因常量格式失败。参考 [Google function enum 示例](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/multimodal/function-calling#function_with_enum_parameter)。

本轮使用真实 `gemini-3.8-flash` 和真实账户，在 `/playground/v1/responses`、`/build/v1/responses` 分别验证流式和非流式请求。工具目录保留 23 项，索引 22 的工具包含 `allowAsync: enum[false]`、`allowAsyncTrue: enum[true]`、整数和小数枚举，以及 `$defs` / `$ref` 中的布尔与数字常量。检查实际参数为 `false`、`true`、数字 `22`、`1.25`，随后真实回传工具结果并得到 `ENUM_TYPED_OK_22`。检查每个渠道的响应标识，以及流式的 `response.completed` 终态。

另外，实际安装的 Codex CLI 用原始工具目录接入两个渠道，并通过真实本地 MCP 发现和调用上述参数类型的工具。完整复测还要求 `apply_patch` 创建文件、命令校验文件内容一致；证据只有在所有流式请求正常完成、MCP 收到正确参数和文件校验通过时才会标记成功。

```bash
python3 scripts/verify-enum-channels.py
QA_SUITE=enum-qa QA_CHANNEL=playground QA_TARGET_MODEL=gemini-3.8-flash \
  QA_SCHEMA_PROBE=1 QA_TYPED_ENUM_PROBE=1 python3 scripts/verify-codex.py
QA_SUITE=enum-qa QA_CHANNEL=build QA_TARGET_MODEL=gemini-3.8-flash \
  QA_SCHEMA_PROBE=1 QA_TYPED_ENUM_PROBE=1 python3 scripts/verify-codex.py
```

本轮证据在 `runtime/enum-qa/`：`before-fix.json` 记录修复前相同错误，`after-fix.json` 记录四组真实协议请求及工具结果，两个 `*-codex.json`、`*-mcp-probe.json` 和 `completion-audit.json` 记录真实 Codex/MCP 和部署状态。旧程序备份是 `aistudio2api-before-enum-fix`。复测会使用真实上游额度；不要把 429、HTTP 200 后的 `response.failed` 或不完整流计作成功。

### `$defs` / `$ref` 修复记录

2026-10-05 后续修复：Playground 原先在工具参数含 `$defs` 时产生 `编码 function declaration 17: parameters ... JSON schema 字段 $defs`，导致 Codex 的 SSE 在完成前失败。此前隔离 CLI 验收没有覆盖这种工具 schema。

现在在 Playground 编码前展开当前 schema 中的 `$ref`，支持 `$defs`、旧 `definitions`、嵌套对象和数组、组合分支、复用定义及 JSON Pointer 转义；保留引用的参数类型、字段和约束。不会直接删除引用对应的结构。工具发现返回的 schema 另外作为文本元数据送入 Gemini 历史，避免上游把 `$ref` 误识别为多媒体附件引用；动态工具声明仍按原目录加载。

支持本地、非循环引用。递归引用、远程 schema 引用和超出展开限制的 schema 会明确报错，不会截断为不准确的参数定义。本地展开限制为 64 层、10,000 个节点。格式参考：[OpenAI schema definitions](https://developers.openai.com/api/docs/guides/structured-outputs#definitions-are-supported)。

复测使用真实本地 MCP 工具，保留其包含 `$defs` 和嵌套 `$ref` 的输入 schema。Codex 先发现工具，再发送动态命名空间目录、实际调用 MCP、回传结果并完成回答。命令如下：

```bash
QA_SUITE=schema-qa QA_CHANNEL=playground \
  QA_TARGET_MODEL=gemini-3.8-flash QA_SCHEMA_PROBE=1 QA_SCHEMA_PROBE_ONLY=1 \
  python3 scripts/verify-codex.py
```

`runtime/schema-qa/before-fix.json` 保存修复前的相同错误；`after-fix.json` 保存 18 个工具声明（索引 17 含引用）在 `gemini-3.8-flash` 上的流式与非流式调用结果。CLI/MCP 验收文件也位于此目录，检查实际调用参数、工具结果及每轮 `response.completed`，不会把单纯 HTTP 200 判为成功。服务回滚备份为 `runtime/schema-qa/aistudio2api-before-schema-fix`。

该命令仅在隔离的 QA 配置中信任 `record_reference_probe` 测试工具，其唯一写入是本机测试目录中的标记证据；用户实际 Codex/MCP 权限不变。去掉 `QA_SCHEMA_PROBE_ONLY=1` 可增加文件读写与补丁流程，产生更多模型请求。真实上游每分钟额度仍然生效，HTTP 429 需要等额度恢复；本次测试中的 429 和隔离 MCP 审批配置问题均保留为失败记录，不计入成功验收。

## 上传接口

下表路径均可以添加 `/playground` 或 `/build` 前缀，使用相同的 Bearer API 密钥。

| 方法与路径 | 用途 |
| --- | --- |
| `POST /v1/files` | 通用上传，接受图片、音频、视频、PDF、文本等文件 |
| `POST /v1/uploads/images` | 图片上传，要求 `image/*` MIME |
| `POST /v1/uploads/audio` | 音频上传，要求 `audio/*` MIME |
| `POST /v1/uploads/videos` | 视频上传，要求 `video/*` MIME |
| `POST /v1/uploads/files` | 通用文件上传别名 |
| `GET /v1/files` | 列出本地上传文件 |
| `GET /v1/files/{id}` | 读取元数据 |
| `GET /v1/files/{id}/content` | 下载原始文件 |
| `DELETE /v1/files/{id}` | 删除文件 |

上传采用 `multipart/form-data`，字段为 `file` 和 `purpose`（建议 `user_data`），单文件上限 **32 MiB**。以下命令中的密钥和文件路径需要换成自己的值：

```bash
export AISTUDIO_API_KEY='填写已有 API 密钥'
export AISTUDIO_BASE='http://127.0.0.1:2048/playground/v1'

curl --noproxy '*' "$AISTUDIO_BASE/files" \
  -H "Authorization: Bearer $AISTUDIO_API_KEY" \
  -F 'purpose=user_data' \
  -F 'file=@/absolute/path/document.pdf;type=application/pdf'
```

响应包含 `id`、`filename`、`bytes`、`purpose` 等字段。把返回的 `file-local-...` ID 放进下一次 Responses 请求：

```json
{
  "model": "gemini-flash-latest",
  "input": [{
    "role": "user",
    "content": [
      {"type": "input_text", "text": "描述这个附件的内容"},
      {"type": "input_file", "file_id": "file-local-替换为上传返回的完整ID"}
    ]
  }]
}
```

将该 JSON 发送到 `$AISTUDIO_BASE/responses`。图片、音频、视频也可使用本服务支持的 `input_image`、`input_audio`、`input_video` 加 `file_id`。Chat Completions 可以使用 `{"type":"file","file":{"file_id":"file-local-..."}}` 内容块。

文件保存在 `runtime/uploads/`，程序重启后 ID 继续有效；本地文件可在两个渠道引用。只有生成请求引用时才向当前渠道发送内容，文本转换为 UTF-8 文本输入，图片、音频、视频与 PDF 转换为附件。上游模型需要具备相应输入能力；上传成功本身不代表任意文件格式都能解析。实测的模型是 `gemini-flash-latest`。

旧 Drive 文件 ID 保留原有逻辑，只能使用 Playground。Build 的本地视频输入已经支持；Veo 视频生成、Live、转录等原有专用能力边界不变。

## 验收和复测

使用本机实际安装的 Codex CLI 0.160.0，而不是模拟工具客户端：

- 两个渠道接收 Codex 原始 GPT 工具目录，包含 freeform `apply_patch` 和 client `tool_search`；仅在本地转发层把模型改为 `gemini-flash-latest`，工具定义、schema、请求参数和历史保持原样。真实执行读取文件、补丁新建文件、命令校验和结果回传。
- 两个渠道直接使用 Gemini 模型名称的 Codex 配置，也完成了真实命令调用和文件一致性检查。
- 两个渠道完成 client tool_search → 动态 namespace 函数调用 → 函数结果回传 → 最终回答。
- 两个渠道分别完成图片、音频、视频、文本、PDF 上传、元数据查询、原始下载、真实模型读取和测试文件删除，共 10 次。模型正确返回红色图片、蓝色视频、语音内容及两种文件标记。
- 通用 `/v1/files` 上传的文件经最终服务重启后仍能从两个渠道下载，字节一致。
- 全应用包 race 测试和前端类型检查、ESLint、Prettier、生产构建通过。

这是完整工具目录兼容及上述实际调用的证据，没有逐个执行所有 Codex 工具，也没有逐个验证全部模型。服务、账户和模型目录需要处于可用状态。

可复用脚本：

```bash
cd '/path/to/aistudio'
QA_CHANNEL=playground python3 scripts/verify-codex.py
QA_CHANNEL=build python3 scripts/verify-codex.py
QA_CHANNEL=playground QA_CLIENT_MODEL=gemini-flash-latest python3 scripts/verify-codex.py
QA_CHANNEL=build QA_CLIENT_MODEL=gemini-flash-latest python3 scripts/verify-codex.py
python3 scripts/verify-tool-discovery.py
python3 scripts/verify-media-uploads.py
```

媒体脚本使用本次生成并保留在 `runtime/codex-qa/media/` 的测试素材。脚本读取本项目 API 密钥，仅输出不含密钥的证据；测试 Agent 的读写范围限于 `runtime/codex-qa/agent-*`。复测会产生少量真实模型请求。服务接口可直接使用，无需保留测试 Agent 在后台运行。

验收证据位于 `runtime/codex-qa/`：`playground-codex.json`、`build-codex.json`、两个 `*-direct-codex.json`、`discovery-live.json`、`uploads-live.json`、`restart-evidence.json` 和 `completion-audit.json`。部署前程序备份为 `aistudio2api-before-codex`。
