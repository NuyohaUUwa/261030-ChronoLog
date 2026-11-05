# ChronoLog · Windows 本机开发指南

更新日期：2026-11-05（Asia/Shanghai）。本指南区分已验证的工具环境与待实现的应用流程。当前只有文档与仓库配置，没有 `server`、`web`、配置加载器和部署文件；第二节以后的应用启动、打包、备份等命令是后续实施示例，不能在当前目录直接运行。

## 一、设备分工与环境核验

Windows 项目目录为 `D:\AppPrograms\261030-ChronoLog`。本机原生运行 Go 与 Vite，使用浏览器完成日常调试；N100 Linux 使用 Docker 构建镜像并运行应用。本机不需要 Docker、WSL2 或 Make。

### 1. 本机检查结果

| 项目 | 2026-11-05 实测 | 结论 |
|---|---|---|
| 系统与资源 | Windows 11 x64，PowerShell 7.6.6；约 21.7 GiB 内存，D 盘约 369 GiB 空闲 | 可开始基础开发 |
| Git | 2.49.0，提交身份已配置 | 可进行本地版本管理 |
| Go | 1.27.0 windows/amd64，`CGO_ENABLED=0` | 工具可用；`go test fmt encoding/json` 通过 |
| Node.js／npm | 24.13.0／11.6.2，Node 内置模块检查通过 | 可用于前端；实际工程依赖尚未安装验证 |
| 浏览器与编辑器 | Chrome、Edge、VS Code 可用 | 可进行前端调试 |
| SSH／SCP | OpenSSH 可用，已连接 `fun@10.126.126.9` | 可进行远程检查及后续交付 |
| Java | JDK 11.0.28，`JAVA_HOME` 指向该版本 | 不据此认定可构建未来 Android 工程 |
| Android | 未发现 Android Studio、完整 SDK、`sdkmanager`；PATH 无 ADB，手机助手目录有 ADB | APK 工具链待补齐 |
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

### 2. N100 检查结果

| 项目 | 2026-11-05 实测 | 结论 |
|---|---|---|
| 连接 | `ssh fun@10.126.126.9`，已有认证可用 | SSH 已通 |
| 系统 | Ubuntu 24.04.5 LTS，Linux x86_64 | 与目标 linux/amd64 一致 |
| 容器工具 | Docker 29.9.2、Compose 5.6.1、Buildx 0.38.1 | 当前用户可访问 Docker |
| 资源 | 可用内存约 3.3 GiB，可用磁盘约 1.7 TiB | 构建峰值资源尚待实际测量 |
| 应用端口 | 18088 未被占用 | 可作为正式端口候选，部署前复查 |
| AI 服务 | `127.0.0.1:3000/v1/models` 返回 401 | 服务可达，需要凭据验证 |
| 域名入口 | 用户级 Cloudflare Tunnel 服务 active | 项目域名及路由尚未验证 |
| 旧目录 | `/home/fun/桌面/ChronoLog` 只有原有三份文档，与本机原始文件哈希一致 | 未发现源码或 Git 历史，正式运行目录另行确定 |

Windows PowerShell 发起连接：

```powershell
ssh fun@10.126.126.9
```

连接后，在 N100 Linux shell 执行以下只读检查：

```bash
uname -srm
docker version
docker compose version
docker buildx version
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}'
free -h
df -h /home
ss -ltn
```

资源、端口和服务状态是检查时快照，不能保证后续仍相同。完整核验记录见[初始化计划](开发环境与项目初始化计划.md)。

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
| Windows Go 服务端 | `http://10.126.126.9:3000/v1` | 已验证网络通，返回 401，真实调用待验证 |
| N100 宿主机 | `http://127.0.0.1:3000/v1` | `localhost` 指 N100 自身 |
| N100 ChronoLog 容器 | `http://host.docker.internal:3000/v1` | Compose 需设置 `host-gateway`；容器内连通性待验证 |
| 浏览器／APK | 不连接 AI | 调用 Go 业务 API，不能持有 AI 密钥 |

| 现象 | 检查方式 |
|---|---|
| 5173／8080 被占用 | 使用第一节端口检查；关闭自己启动的旧进程，或同时调整服务端和代理端口 |
| `go:embed` 找不到文件 | 开发使用已实现的 `dev` 构建；生产先构建并复制前端，不能用空占位页面交付 |
| 浏览器 API 404／502 | 确认 Go 正在运行、Vite 代理 `/api` 保留路径、API 前缀为 `/api/v1` |
| 真实 AI 返回 401 | 检查服务端配置的有效密钥，不把密钥粘到日志、浏览器或 Git 中 |
| 容器访问 AI 失败 | 复查 `extra_hosts`、宿主机监听地址、防火墙和容器内请求路径 |
| APK 构建提示 Java 不兼容 | 使用项目要求的 Android Studio 配套 JDK，确认终端 Gradle 使用同一版本 |
| Windows 能访问但手机失败 | 手机的 `localhost` 是手机自身；检查实际 IP、监听地址、网络与 CORS |

## 五、N100 阶段验收与正式运行（待实现）

只在阶段完成后交付源码和构建文件，由 N100 构建 linux/amd64 镜像。源码包不包含 `.git`、个人数据和真实配置；可在本机干净提交后用 `git archive` 导出到项目目录外，再通过 SCP 传到 N100 经确认的源码目录。远程正式运行目录不默认复用桌面文档副本。

部署前确保实际 Dockerfile、Compose 文件、配置模板和启动入口均已实现；镜像基底版本、锁文件和实际构建结果已核验。在 N100 Linux shell、已确认的源码根目录执行构建示例：

```bash
docker build -t chronolog:<commit> .
```

`<commit>` 是要替换的 Git 提交标识，不是可直接执行的参数。Compose 使用同一镜像、不同项目名和独立 `.env`：正式运行候选端口 18088；临时验收候选端口 18089。两者部署前都要检查占用。禁止设置固定 `container_name`，以免临时验收与正式容器冲突。

N100 Linux shell、已确认的运行根目录，准备好 Compose 和对应配置后执行的规划示例：

```bash
docker compose --env-file acceptance/.env -p chronolog-acceptance up -d
docker compose --env-file acceptance/.env -p chronolog-acceptance ps
docker compose --env-file acceptance/.env -p chronolog-acceptance logs --tail 50
```

验收配置使用独立配置目录、独立数据目录，默认模拟通知，避免向正式渠道重复发送。验证原稿持久化、重启保留、登录隔离、AI 状态、检索、日历和提醒后，按正式更新任务授权进行更新。验收结束仅停掉对应项目容器，不删除正式数据。

Compose 模板默认绑定 N100 回环地址；Windows 查看验收服务时，在本机 PowerShell 运行 `ssh -N -L 18089:127.0.0.1:18089 fun@10.126.126.9`，保持会话并用浏览器打开 `http://127.0.0.1:18089`。正式域名由 N100 宿主机上的隧道转发到 18088，实际路由待验证。

备份至少包含完整数据目录、账号与用户配置、提示词及实际部署配置。首次采用停写备份，避免文件与数据库处于不同时间点；恢复只在新的隔离目录验证，索引可重建但账号凭据不能从笔记推导。正式升级保留上一版镜像和备份，回滚前确认数据格式兼容。具体备份、恢复命令待部署目录确定后补齐。

## 六、Android（工具链与工程均待补齐）

安装 Android Studio 及其配套 JDK、SDK、Platform Tools；使用项目对应的 Gradle Wrapper，无需全局安装 Gradle。优先使用真机调试，模拟器非必需；不要把手机助手的 ADB 当作完整 Android 开发环境。

2026-11-05 查阅的 [Capacitor 官方环境要求](https://capacitorjs.com/docs/getting-started/environment-setup)说明 Capacitor 8 需要 Node.js 22+、Android Studio 2025.2.1+，Android Studio 可提供配套 JDK。项目具体版本尚未锁定，安装时按最终 Capacitor、AGP、SDK 组合确认，保留现有 JDK 11 供其他项目使用。

未来 Android 工程存在且依赖锁定后，在 Windows PowerShell、`web` 执行：

```powershell
npm ci
npm run build
npx cap sync android
Set-Location android
.\gradlew.bat assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

`npx cap add android` 仅首次创建工程时执行，重复开发只需 sync。调试包可直接安装；Release 必须配置本地签名、版本号和稳定签名密钥，未签名包不能作为正式交付。签名文件、密码和本机 SDK 路径不提交，保留受控备份。

Debug 内网 HTTP 与 Release HTTPS 分开配置。首版通知规划使用 Ntfy Android 客户端接收远程推送，ChronoLog APK 本身不自动获得原生推送能力；先验证通知打开笔记 Web 链接，再单独实现并验证 Android App Links 或其他深层链接。具体打包及通知约定见[实战手册](全栈开发实战手册.md)。
