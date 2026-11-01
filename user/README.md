# 账号自助应用（auth-user）

独立 Next.js 应用，提供登录、注册和首次初始化页面。网关将 `/login`、`/setup` 转发到本服务；Next 静态资源使用 `/auth-user-assets/_next/static/` 专属前缀，避免与主站共用 `/_next/static/` 时串到错误应用。

数据请求使用同源 `/api/*`，会话由账号服务管理。`bun run typecheck`、`bun run i18n:check` 和 `bun run build` 用于本地验证。

登录、注册及初始化的所有状态共用顶部语言、显示模式和调色板入口。主题沿用主站的 `metafusion_theme_{mode,accent,tone,pattern}` 本地存储，在同域跳转时保持设置；首帧脚本在内容显示前应用选择，跟随系统模式会实时响应系统变化。

主题逻辑、控件、配色、圆角、基础样式及四语主题文案来自主仓 `frontend/`。在相邻检出布局下，从主仓 `frontend/` 运行 `bun run theme:sync-auth` 更新，运行 `bun scripts/sync-account-theme.ts --check` 检查一致性，两个仓库分别提交。同步结果纳入本仓，独立构建和运行不依赖主仓源码；不要手改同步文件。公开底纹路径 `/patterns/*` 同域部署由主站提供，独立启动时本应用也有相同资产。
