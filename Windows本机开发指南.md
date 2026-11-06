# ChronoLog · Windows 本机开发指南

更新日期：2026-11-06（Asia/Shanghai）。本指南区分已验证的工具环境与待实现的应用流程。当前只有文档与仓库配置，没有 `server`、`web`、配置加载器和部署文件；第二节以后的应用启动、打包、备份等命令是后续实施示例，不能在当前目录直接运行。

## 一、设备分工与环境核验

Windows 项目目录为 `D:\AppPrograms\261030-ChronoLog`。本机原生运行 Go 与 Vite，完成浏览器和 Android 真机调试、功能验收及产物构建；N100 Linux 仅使用 Docker 正式运行预构建镜像。本机日常不需要 Docker、WSL2 或 Make，正式发布前的 Linux 容器镜像构建能力另行补齐。

**N100 连接必须先取得用户对本次目的和操作范围的明确同意。** 包括 SSH、SCP、HTTP／AI API、只读检查、连通性探测、端口转发和浏览器访问。历史可连接记录不是当前授权；日常开发禁止连接 N100，模拟服务失败时不得自动回退正式服务。完整规则见 [AGENTS.md](AGENTS.md)。

### 1. 本机检查结果

| 项目 | 2026-11-05 实测 | 结论 |
|---|---|---|
| 系统与资源 | Windows 11 x64，PowerShell 7.6.6；约 21.7 GiB 内存，D 盘约 369 GiB 空闲 | 可开始基础开发 |
| Git | 2.49.0，提交身份已配置 | 可进行本地版本管理 |
| Go | 1.27.0 windows/amd64，`CGO_ENABLED=0` | 工具可用；`go test fmt encoding/json` 通过 |
| Node.js／npm | 24.13.0／11.6.2，Node 内置模块检查通过 | 可用于前端；实际工程依赖尚未安装验证 |
| 浏览器与编辑器 | Chrome、Edge、VS Code 可用 | 可进行前端调试 |
| SSH／SCP | OpenSSH 可用，曾在先前用户授权下连接 N100 | 后续每次连接仍须用户明确同意，仅用于正式部署／运维 |
| Java | JDK 11.0.28，`JAVA_HOME` 指向该版本 | 不据此认定可构建未来 Android 工程 |
| Gradle | 用户缓存内 8.14.3 `bin` 分发完整，`--version` 可运行 | 通过匹配版本的项目 Wrapper 复用，无需全局安装 |
| Android | 未发现完整 SDK／Android Studio；手机助手有 ADB 1.0.41（34.0.3）和 1.0.40 | 另补 JDK 21 和官方 SDK／Platform Tools，Android Studio 可选 |
| Docker／Make | 当前 PATH 未找到 | 本机开发不要求安装 |

本机 npm 使用 `https://registry.npmmirror.com/`，Go 使用 `https://proxy.golang.org,direct` 与 `sum.golang.org`。已验证这些服务以及官方 npm、Android SDK 元数据、Gradle 元数据端点可达；未验证全部包、镜像或 SDK 文件下载。检查时本机 5173、8080、18088、3000 端口均未发现监听，启动前需要复查。

Windows PowerShell，可在项目根目录执行的只读检查：

```powershell
git --version
go version
go env GOOS GOARCH CGO_ENABLED GOPROXY GOSUMDB
node --version
npm --version
java -version
javac -version
Get-Command git,go,node,npm,ssh,scp
Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue |
    Where-Object { $_.LocalPort -in 5173,8080,18088,3000 }
```

### 2. N100 历史检查记录（不代表当前连接许可）

以下来自此前获用户授权的检查，本轮没有连接或复查 N100。保留用于正式运行规划，不作为开发步骤。

| 项目 | 2026-11-05 实测 | 结论 |
|---|---|---|
| 连接 | `fun@10.126.126.9`，历史检查认证可用 | 地址仅供获准后的正式部署／运维使用 |
| 系统 | Ubuntu 24.04.5 LTS，Linux x86_64 | 与目标 linux/amd64 一致 |
| 容器工具 | Docker 29.9.2、Compose 5.6.1、Buildx 0.38.1 | 当前用户可访问 Docker |
| 资源 | 可用内存约 3.3 GiB，可用磁盘约 1.7 TiB | 仅为历史快照；不在 N100 构建或开发压测 |
| 应用端口 | 18088 未被占用 | 可作为正式端口候选，部署前复查 |
| AI 服务 | `127.0.0.1:3000/v1/models` 返回 401 | 服务可达，需要凭据验证 |
| 域名入口 | 用户级 Cloudflare Tunnel 服务 active | 项目域名及路由尚未验证 |
| 旧目录 | `/home/fun/桌面/ChronoLog` 只有原有三份文档，与本机原始文件哈希一致 | 未发现源码或 Git 历史，正式运行目录另行确定 |

资源、端口和服务状态是检查时快照，不能保证后续仍相同。如正式部署／运维需要复查，先说明目标和具体操作，取得用户同意后才连接。日常开发不执行远程预检，Android 本机复核见 [Android 本机开发计划](Android本机开发计划.md)。

## 二、首次启动前提与配置（待实现）

先建立实战手册中的 `server`、`web`、配置示例与构建文件，并提交 `go.mod`、`go.sum`、`package-lock.json`。本机 Go 版本不等于已确定的最低依赖版本；实际版本组合必须在工程初始化时锁定并验证。安装依赖时优先按锁文件执行，不在每次开发时重新选择最新版本。

未来需提供 `config/config.example.yaml`、`config/prompts.example.yaml`；复制到 `config/config.local.yaml`、`config/prompts.local.yaml` 后使用。真实配置不提交。配置键和环境变量的对应关系见[实战手册](全栈开发实战手册.md)，其加载器当前尚未实现。

| 配置 | 本机默认规划 | N100 正式规划 |
|---|---|---|
| HTTP 监听 | `127.0.0.1:8080` | 容器内 `0.0.0.0:8080` |
| 数据 | 根目录下独立 `data-dev` | 独立运行目录的 `data` 挂载到 `/app/data` |
| AI | `AI_MODE=mock` 固定响应 | `AI_MODE=remote`，密钥仅传给 Go |
| 通知 | `NOTIFICATION_MODE=mock`，只记录发送结果 | `NOTIFICATION_MODE=remote`，使用已配置渠道 |
| 前端 API | 相对地址 `/api/v1`，Vite 代理 `/api` | Go 同源提供 `/api/v1` |
| 时间 | `APP_TIMEZONE=Asia/Shanghai` | 同本机，镜像包含时区数据 |

配置路径和相对数据路径统一相对于项目根目录解析；为避免运行目录不同产生歧义，本机示例通过环境变量传绝对路径。正式容器使用 `/app` 作为工作目录，配置和数据路径也使用绝对路径。

## 三、日常开发（待实现）

### 1. 准备依赖与配置

仅在源码、配置示例及锁文件已存在后，在 Windows PowerShell、项目根目录执行：

```powershell
Copy-Item -LiteralPath config/config.example.yaml -Destination config/config.local.yaml
Copy-Item -LiteralPath config/prompts.example.yaml -Destination config/prompts.local.yaml
Push-Location web
npm ci
Pop-Location
Push-Location server
go mod download
Pop-Location
```

复制命令仅用于首次配置，已有本地配置时直接编辑，不重复覆盖。开发用 JWT 密钥在本地配置中生成和保存；不使用文档中的占位符。

### 2. 启动 Go

Windows PowerShell、项目根目录，终端 A：

```powershell
$env:CHRONOLOG_CONFIG = Join-Path $PWD.Path 'config/config.local.yaml'
$env:PROMPTS_FILE = Join-Path $PWD.Path 'config/prompts.local.yaml'
$env:DATA_DIR = Join-Path $PWD.Path 'data-dev'
$env:AI_MODE = 'mock'
$env:NOTIFICATION_MODE = 'mock'
Set-Location server
go run -tags dev .
```

开发构建采用 `dev` build tag，静态资源包提供不依赖 `web_dist` 的实现，API 由 Go 提供，页面由 Vite 提供。生产构建不带此 tag，先生成并拷贝前端资源再 `go build`；两种文件使用互斥的构建约束，避免首次运行出现 `go:embed` 空目录错误。此机制待实现，当前不能直接使用该命令。

成功标志：服务报告脱敏后的监听地址和开发数据目录；匿名请求 `GET /api/v1/health` 返回 200，业务接口仍须登录。

### 3. 启动 Vite

Windows PowerShell、项目根目录，终端 B：

```powershell
Set-Location web
npm run dev -- --host 127.0.0.1 --port 5173
```

Vite 固定将 `/api` 代理到 `http://127.0.0.1:8080`，启用 `strictPort` 以避免端口自动变化。浏览器打开 `http://127.0.0.1:5173`。正常调试无需浏览器跨域许可，也不直接从浏览器访问 AI 服务。

手机浏览器调试时，Vite 改为监听 `0.0.0.0`，手机访问 Windows 的可达局域网 IP 与 5173 端口；确认网络连通及防火墙允许该端口。此时代理目标仍是 Windows 的 `127.0.0.1:8080`。APK 的 WebView 不经过 Vite，需使用实际可达的业务 API 地址并配置其来源的 CORS 规则。

### 4. 简短验证

日常只检查本次变更相关内容：

- 注册、登录后创建笔记，确认 `data-dev/users/.../notes` 有原稿及标准元数据。
- 固定模拟 AI 返回后显示卡片、标签和日历；模拟超时或非法输出时原稿仍存在，界面可见失败状态。
- 提醒模拟发送后 `notification_status=sent`，待办仍为 `pending`，由用户操作完成。
- 修改鉴权或存储时，用第二个账号确认不能查询、修改其他账号的笔记。

Go 逻辑变更在 `server` 运行相关包测试；整体检查时使用 `go test -tags dev ./...`。前端改动在 `web` 使用实际存在的检查脚本及 `npm run build`。当前无源码，以上应用检查尚不可执行。

## 四、AI 地址与最小排障

| 请求发起方 | AI 基地址 | 说明 |
|---|---|---|
| Windows Go 开发服务 | `AI_MODE=mock`，不发远程请求 | 可另用独立本机模拟服务，禁止指向 N100 |
| N100 正式服务（宿主机） | `http://127.0.0.1:3000/v1` | 仅正式运行配置；代理检查须事先获同意 |
| N100 正式服务（容器） | `http://host.docker.internal:3000/v1` | Compose 配置宿主机网关；不能作为本机开发地址 |
| 浏览器／APK | 不连接 AI | 调用 Go 业务 API，不能持有 AI 密钥 |

| 现象 | 检查方式 |
|---|---|
| 5173／8080 被占用 | 使用第一节端口检查；关闭自己启动的旧进程，或同时调整服务端和代理端口 |
| `go:embed` 找不到文件 | 开发使用已实现的 `dev` 构建；生产先构建并复制前端，不能用空占位页面交付 |
| 浏览器 API 404／502 | 确认 Go 正在运行、Vite 代理 `/api` 保留路径、API 前缀为 `/api/v1` |
| 真实 AI 返回 401 | 检查服务端配置的有效密钥，不把密钥粘到日志、浏览器或 Git 中 |
| 正式容器访问 AI 失败 | 先取得用户对本次排障连接的同意，再在授权范围内核查配置与网络 |
| APK 构建提示 Java 不兼容 | 在项目 PowerShell 中设置独立 JDK 21，确认 `gradlew.bat --version` 使用同一版本 |
| Windows 能访问但手机失败 | 手机的 `localhost` 是手机自身；检查实际 IP、监听地址、网络与 CORS |

## 五、本机验收与正式交付边界（待实现）

服务端相关功能、失败场景、索引重建和恢复验证按变更在本机独立测试数据上完成；Android 首包只做安装、启动及基础记事检查，不重复整套后端测试。移除 N100 阶段验收容器、18089 测试端口和开发 SSH 隧道安排。

正式镜像由开发侧构建。当前本机没有 Docker，发布准备时需单独补齐本机 Linux 容器构建能力；在此之前可继续原生 Go／Web／Android 开发，不能将镜像构建移到 N100。实际 Dockerfile、锁文件、应用入口与构建工具齐备后，在开发机 PowerShell、源码根目录执行规划示例：

```powershell
$releaseTag = 'replace-with-commit-id'
docker build --platform linux/amd64 -t "chronolog:$releaseTag" .
docker save -o "../chronolog-$releaseTag.tar" "chronolog:$releaseTag"
```

先替换提交标识；当前缺少源码与 Docker，以上命令尚不可运行。确认镜像与已签名 APK、变更说明及回滚方案在本机可审查后，向用户说明正式发布所需的连接目标与操作范围，等待明确同意。获准后才传输产物、加载镜像和更新正式服务，N100 不接收用于远程编译的源码工作流。

备份至少包含完整数据目录、账号与用户配置、提示词及部署配置；正式停写备份、文件传输、重启及回滚须属于本次同意范围。恢复演练使用本机隔离目录，默认使用合成测试数据；确需拷贝正式数据时必须另有对应同意。具体操作命令在正式部署目录确定且获准后补齐。

## 六、Android 本机方案（待补齐与实现）

完整工具清单、版本依据、安装缺口和命令见 [Android 本机开发计划](Android本机开发计划.md)。核心安排：

- 复用 Node.js 24、VS Code、Go 和 Gradle 8.14.3 缓存；保留系统 JDK 11，仅在项目终端使用独立 JDK 21。
- 补齐官方 SDK 管理工具、平台包与 Platform Tools；无需全局 Gradle，Android Studio／模拟器按需使用。
- USB 真机通过 `adb reverse` 连接 Windows Go，Debug API 为本机映射地址；不使用 N100、正式域名或远程隧道调试。
- 首版直接交付带调试签名的 Debug APK，只检查安装、启动、登录和笔记保存／查看。无可用真机时先提供构建产物与未测说明，由用户安装反馈；不等待完整适配或测试矩阵。
- 后续根据用户需求再安排 Release 签名与升级验证；构建中设置正式 URL 不代表允许代理访问正式服务，访问仍需用户同意。

首版通知规划使用 Ntfy Android 客户端接收正式推送，日常只测试本机模拟通知；APK 原生推送与 App Links 仍需独立实现和本机验证。
