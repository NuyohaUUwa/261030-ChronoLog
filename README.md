# ChronoLog（时光足迹）

个人记事与双模日历，保留注册、登录、多账号隔离。Go 服务端、Vue 3 Web 已实现；Android Debug APK 尚未交付。

## 当前状态（2026-11-07）

已实现笔记原稿落盘、异步 AI、Markdown 安全渲染、检索、日历、待办、管理员提示词配置和索引重建。第二轮问题的验收状态与剩余功能见 [开发实施状态](docs/开发实施状态.md)。审查报告和截图只保存在本机 reviews/，不跟踪、不改写旧 Git 历史。

日常在 Windows 原生运行 Go 与 Vite，使用模拟 AI／通知和隔离数据。N100 只承担正式运行，每批连接须用户明确授权；此次用户已授权完成修复、提交后更新正式服务。

## 开发与部署

- [Windows 本机开发指南](Windows本机开发指南.md)：实际启动、检查与构建命令。
- [部署与运维](docs/部署与运维.md)：GitHub 仓库、Git 发布产物、备份与回滚。
- [Android 本机开发计划](Android本机开发计划.md)：本机后端、USB 真机和最少首包检查。
- [全栈开发实战手册](全栈开发实战手册.md)、[系统设计蓝图](系统设计蓝图.md)：配置契约和完整产品规划，未完成项以实施状态为准。
- [AGENTS.md](AGENTS.md)：开发与 N100 访问规则。

GitHub： https://github.com/NuyohaUUwa/261030-ChronoLog 。main 保存源码；deploy/n100 保存本机构建的 Linux 发布产物及校验信息。N100 使用 Git 获取产物，不在正式机运行 npm、Go 或 Docker 构建。当前更新沿用已有 systemd 服务，Docker 迁移另行安排。
