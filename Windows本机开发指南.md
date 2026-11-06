# ChronoLog Windows 本机开发指南

更新：2026-11-07。已有 server、web、配置加载器及应用测试。PowerShell 在项目根目录执行以下命令；不连接 N100。

## 首次配置

保留已有本地配置，不覆盖。新环境先复制 config/config.example.yaml、config/prompts.example.yaml 为同目录的 config.local.yaml、prompts.local.yaml。JWT 密钥和管理员初始化密钥在本机随机生成，真实值不提交。普通注册无需管理员密钥；管理员候选邮箱注册必须提供与 ADMIN_SETUP_KEY 相符的密钥。旧账号不会被自动删除或重置。

依赖按锁文件安装：在 web 执行 npm ci，在 server 执行 go mod download。已有 Go、Node.js、Git；Android JDK／SDK 以专门计划为准，日常不要求 Docker、WSL 或 Make。

## 本机运行

终端 A，根目录：

```powershell
$env:CHRONOLOG_CONFIG = Join-Path $PWD 'config/config.local.yaml'
$env:PROMPTS_FILE = Join-Path $PWD 'config/prompts.local.yaml'
$env:DATA_DIR = Join-Path $PWD 'data-dev'
$env:AI_MODE = 'mock'
$env:NOTIFICATION_MODE = 'mock'
$env:GOMAXPROCS = '2'
$env:GOMODCACHE = Join-Path $env:LOCALAPPDATA 'ChronoLog/go-mod'
Set-Location server
go run -tags dev .
```

独立缓存避免本机旧 GOPATH 目录权限问题；限制并发避免页面文件不足。只影响当前终端，无需改全局环境。

终端 B，根目录：

```powershell
Set-Location web
npm run dev
```

访问 http://127.0.0.1:5173，Vite 代理到本机 8080。mock AI 回显原稿、空标签／提醒；以 [fail] 开头可模拟失败。mock 通知不对外推送。需要模拟复杂 AI 协议时使用独立本机服务，不指向 N100 或正式域名。

## 验证与恢复

server 目录运行 go test -count=1 -p 1 -tags dev ./...；web 目录运行 npm run build（包含类型检查）。登录、保存、刷新查看、跨账号隔离、AI 失败保留原稿是核心检查。手机端不重复整套后端验证。

启动时在接受请求前从 Markdown 重建所有账号索引，包括非空索引的外部修改和删除，再恢复 processing 任务。运行期间外部编辑不会实时监听；管理员可在停止业务写入后调用 POST /api/v1/system/rebuild-index 或重启服务同步。损坏的笔记不会被静默忽略后重建为空。

## 配置补充

Env > YAML > 默认值。ADMIN_SETUP_KEY 对应 auth.admin_setup_key。AI_MODE 和 NOTIFICATION_MODE 仅接受 mock／remote，remote AI 必须提供有效 HTTP(S) base URL 和模型，密钥只留服务端；不得把凭据放入 URL。AI 超时必须大于零。

提示词 temperature 支持 0–2（0 会保留），max_tokens 为 1–16000，默认 0.2／1500；few-shot 按独立参考时间组成 user／assistant 消息。错误不回传上游原文，响应最大 1 MiB，同时最多 4 个真实 AI 请求，繁忙时保留原稿并提示重试。

## 发布

正式部署遵循 [部署与运维](docs/部署与运维.md)。Windows 构建前端并内嵌，交叉编译 linux/amd64，提交源码和发布分支，N100 通过 Git 拉取固定版本。日常禁止随意连接 N100，历史地址与既往授权不是新任务许可。

Android 首包保持 Debug APK + 本机后端方式，按用户要求只做一次可用设备的安装、启动和登录／保存／查看；无手机则交付产物并说明未实测，不增加离线缓存或完整兼容性门槛。
