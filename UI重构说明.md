# AIStudio2API 界面重构

2026-10-02 已构建并部署至 http://127.0.0.1:2048/。

## 参考与实现

参考项目为 `code/codedx/8`（BakaMail）。读取了其工作区隔离示例、已有截图及 `project1-small-window-theme.css`、`local-theme.css`、`dashed-accent.css`、`workspace.css`、`theme-controls.css`、`login.css`、`dialogs.css`、`notifications.css` 和 `surfaceLines.ts`。采用该项目最终的白色实线轮廓样式。

| 样式 | 本项目实现 |
| --- | --- |
| 雾白底色、淡蓝轮廓、石墨文字及柔和阴影 | `web/src/style.css` 的共用主题与表单样式 |
| 居中品牌、全高工作区、244/72 px 可折叠侧栏 | `WorkspaceShell.vue` |
| 900 px 手机断点、底部三项胶囊、浮动菜单 | `WorkspaceShell.vue` 与移动端样式 |
| 按实际尺寸绘制的四段 SVG 选中轮廓 | `SurfaceOutline.vue` |
| 可逆侧栏、轮廓移动、方向相关的页面切换 | 工作区过渡与 `KeepAlive` |
| 顶部液滴展开胶囊通知 | `App.vue` 与通知动画 |
| 居中登录卡片、浅色弹窗、下拉与确认框 | `LoginPage.vue`、`UiDialog.vue`、`UiSelect.vue`、`UiConfirm.vue` |

日志、账户、模型目录、冷却与请求、服务配置及 API 试用均完成迁移。页面保留 AIStudio2API 的业务内容与原有接口；后端 Go 源码、认证配置及账户数据未修改。页面切换保留模型搜索、筛选和试用草稿。弹窗与手机菜单支持焦点循环、Escape 退出及恢复焦点；支持系统减少动态效果偏好。

## 验收

- `npm run format:check`、`npm run lint`、`npm run build` 通过；Go ARM64 程序重新编译并重启成功。
- 正式地址六个页面均已浏览器加载，无浏览器运行错误；健康、状态、账户、模型与实际生产 JS/CSS 资源均 HTTP 200。
- 隔离示例验证了有账户、有模型、有冷却与请求记录、Chrome 导入弹窗、管理员登录界面及错误通知。该示例在应用挂载前替换全部 HTTP 和 SSE，不执行真实账户操作。
- 在 320、390、900、901、1024、1440 px 检查响应布局；窄屏无页面横向溢出。稳定状态下侧栏为 72/244 px，选中轮廓与按钮边界一致。
- 正式服务另验证 390 × 844 的浮动菜单与浏览器登录弹窗；弹窗宽 358 px，边界位于视口内。
- 验证了模型键盘筛选、模型搜索与试用草稿跨页保留、弹窗/菜单焦点循环、Escape 恢复焦点及减少动态效果时过渡为 0。

记录与截图位于 `runtime/ui-qa/`，其中 `live-*` 为正式服务，`fixture-*` 与 `login-desktop.png` 为隔离示例。`browser-check.json` 中断点采样包含动画过程，最终稳定侧栏尺寸另列于 `settledSidebarWidths`。

当前没有已认证的 Google 账户，因此真实模型生成尚未验收。有数据的界面截图来自隔离示例。临时验收浏览器标签与 5190/5191 开发服务已清理，2048 管理服务继续运行。

## 回退与后续构建

重构前前端备份：`runtime/ui-backup/web-before-bakamail.tgz`。
重构前可运行程序：`runtime/ui-backup/aistudio2api-before-bakamail`。

需要临时回退运行界面时，先停止管理进程，复制备份程序至根目录 `aistudio2api`，再启动；不会覆盖 `.env` 或 `auth/`。修改 UI 后必须重新执行前端构建及 Go 构建，Go 程序内嵌生产前端资源，操作见 `本地部署.md`。

隔离示例保存在 `web/qa/`，可在前端开发服务中打开 `/qa/index.html`；`?login` 查看登录页，`?reduce` 检查减少动态效果。该目录不作为生产构建入口。
