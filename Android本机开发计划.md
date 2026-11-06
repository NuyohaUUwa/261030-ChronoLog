# ChronoLog · Android 本机开发计划

更新日期：2026-11-06。本文根据本机检查和用户最新决定调整，尚未安装新工具或创建 Android 工程。N100 仅正式运行，每次连接必须先取得用户对本次目的和范围的明确同意，详见 [AGENTS.md](AGENTS.md)。

## 首版交付目标（用户最新决定）

优先交付一个可安装、可启动、能完成基础记事流程的 APK，由用户实际使用后提出需求变更。先提供带调试签名的 Debug APK，不以完整 Release 发布流程、多机型适配或大量手机测试作为首包交付条件。

首版基础流程为“连接本机后端 → 注册／登录 → 输入并保存笔记 → 查看已保存笔记”。APK 仍依赖运行中的本机 Go 服务；USB 映射及连接方法随包提供，不宣称首版可离线独立运行。AI 和通知先使用模拟模式并明确标识，不把模拟效果描述为真实能力已通过验收。

先修复影响安装、启动、保存和查看的阻塞问题；其他交互优化、机型适配与扩展功能，收到用户反馈后再安排。此调整缩减 Android 验证与首包交付门槛，不取消项目整体功能规划和服务端必要的数据保护。

## 一、现有工具与可复用范围

| 工具 | 本机实测 | 本项目处理方式 |
|---|---|---|
| Node.js／npm | 24.13.0／11.6.2，可执行 | 复用，用于 Vue 和 Capacitor；依赖通过项目锁文件管理 |
| VS Code、Chrome／Edge | 已安装，可用于编辑和 Web 调试 | 继续作为主要开发工具，无需为日常页面开发另装 IDE |
| Go、Git、PowerShell | 已具备，Go 1.27.0、Git 2.49.0、PowerShell 7.6.6 | Go 服务在本机运行，所有日常命令使用 PowerShell |
| JDK | 11.0.28，`C:\Program Files\Java\jdk-11`；当前 `JAVA_HOME` 指向它 | 保留供原项目使用；另配 JDK 21 给本项目，避免修改全局默认值 |
| Gradle | `C:\Users\FZK-HP\.gradle\wrapper\dists\gradle-8.14.3-bin` 有完整解压分发，执行 `--version` 成功 | 可按项目 Wrapper 的相同版本与分发 URL 复用；无需全局安装 Gradle |
| Vivo 手机助手 ADB | `D:\Applications\VivoWorkHelper\pcsuite\adb_41\adb.exe`：1.0.41／34.0.3；另一个 `adb\adb.exe` 为 1.0.40 | 已确认版本命令可运行；不据此推断真机授权或 SDK 齐全，项目统一使用独立官方 Platform Tools |
| Visual Studio 2022／.NET Android Manifest | 注册表有安装记录，但未找到完整 Android SDK | 不切换到 MAUI／Xamarin，不把 Manifest 视为可用 Android 构建链 |
| Android Studio、完整 SDK、新 Android CLI／sdkmanager | 在已检查的安装目录、PATH 和常见 SDK 位置未发现完整工具链；SDK 环境变量未设置 | 补齐命令行 SDK 工具和平台包；Android Studio 为可选的原生调试界面 |

Gradle 在 JDK 11 下可以打印版本，不代表 Capacitor Android 能在 JDK 11 下编译。尚未检测手机连接、安装 APK 或验证设备驱动；现有模拟器目录也不能作为已可用模拟器的证明。

## 二、最小补齐方案

采用 **现有 VS Code + Node.js + 独立 JDK 21 + 官方 Android SDK + 项目 Gradle Wrapper + USB 真机**。先跑通命令行构建，遇到原生插件、布局或诊断需求时再安装 Android Studio。

| 项目 | 计划 |
|---|---|
| JDK | 新增独立 JDK 21，建议目录 `D:\DevTools\Java\jdk-21`；安装后核验实际路径和补丁版本 |
| SDK | 独立目录 `D:\DevTools\Android\Sdk`，与手机助手隔离 |
| SDK 管理 | 新装优先使用 Google 官方 Android CLI；已有旧工具时才使用固定版本 `sdkmanager` 兼容流程 |
| 最小平台包 | `platform-tools`、`platforms/android-36`；按 AGP 8.13 参考组合使用 `build-tools/35.0.0` |
| Capacitor | 规划使用兼容 Node 24 的 Capacitor 8 系列，core／cli／android 版本匹配并锁定；Keyboard 等插件使用匹配主版本 |
| 原生参考组合 | JDK 21、AGP 8.13、Gradle 8.14.3、compileSdk／targetSdk 36、minSdk 24；最终按所选 Capacitor 的实际模板与依赖验证 |
| 非必需项 | Android Studio、模拟器、NDK、CMake、全局 Gradle；仅在具体功能需要时引入 |

这些目录是计划位置，当前没有创建。Gradle `bin` 和 `all` 是不同分发，缓存是否命中取决于 Wrapper 配置；不能强行引用用户缓存的绝对路径或为了复用缓存降级实际模板。需下载时校验官方分发校验值，并提交 Wrapper 文件和依赖锁文件。

选型依据：Capacitor 的 [8.0 升级说明](https://capacitorjs.com/docs/updating/8-0)提供 SDK／AGP／Gradle 基线，其 [Android 构建定义](https://raw.githubusercontent.com/ionic-team/capacitor/8.0.0/android/capacitor/build.gradle)使用 Java 21；[AGP 8.13 兼容表](https://developer.android.com/build/releases/agp-8-13-0-release-notes)区分 JDK 最低要求与 Build Tools 默认版本。因此本项目采用 JDK 21，而不能只按 AGP 的 JDK 17 最低要求选择。

Google 已将 [`sdkmanager`](https://developer.android.com/tools/sdkmanager)标记为弃用，推荐 [Android CLI 的 `android sdk`](https://developer.android.com/tools/agents/android-cli/commands/sdk)管理 SDK。后续安装时选择官方 Windows 分发，核验版本和实际路径，避免照抄旧版工具布局。命令行完成 Gradle 构建、Debug 安装和 Release 签名有 [Android 官方说明](https://developer.android.com/build/building-cmdline)；Android Studio 是本项目的可选 GUI，并不改变工程所需 SDK／JDK。

## 三、项目级环境配置（安装后执行）

在专用 PowerShell 终端为本项目设置环境变量，关闭终端即结束本次设置；不修改系统或用户级 Java 默认值。以下目录仅在安装完成并核对后使用：

```powershell
$env:JAVA_HOME = 'D:\DevTools\Java\jdk-21'
$env:ANDROID_HOME = 'D:\DevTools\Android\Sdk'
$env:Path = "$env:JAVA_HOME\bin;$env:ANDROID_HOME\platform-tools;$env:Path"
java -version
javac -version
& "$env:ANDROID_HOME\platform-tools\adb.exe" version
```

SDK 管理工具按实际安装位置调用，不把手机助手的 ADB 目录加入项目 PATH。保留默认的 `GRADLE_USER_HOME` 以复用当前缓存。未来 `web/android/local.properties` 写入 `sdk.dir=D:/DevTools/Android/Sdk`，该本机文件已被 Git 忽略；绝对 SDK／JDK 路径不写入共享 Gradle 配置。

## 四、本机开发与 USB 真机联调（工程建立后执行）

1. 本机 Go 使用 `AI_MODE=mock`、`NOTIFICATION_MODE=mock` 和独立开发数据目录，监听 `127.0.0.1:8080`。Web 日常通过 Vite 代理访问它。
2. Vue／Capacitor 工程初始化时配置 appId 和 Debug／Release API 地址，`npx cap add android` 只执行一次；提交 Android 工程与 Wrapper，排除构建产物和本机设置。
3. 手机启用 USB 调试，并在手机上确认此开发机；用项目官方 ADB 查看设备。多台设备时通过 `-s` 显式选择序列号。
4. USB 联调优先使用 `adb reverse`，使 Debug APK 连接手机回环端口映射到 Windows Go 服务，不需要 N100 或远程隧道。
5. UI 小改动先用浏览器验证；涉及 APK／原生行为时才重新 build、sync、assembleDebug、安装。

工具与工程齐备后，Windows PowerShell、项目根目录的规划示例：

```powershell
Set-Location web
npm ci
npm run build
npx cap sync android
Set-Location android
.\gradlew.bat --version
.\gradlew.bat assembleDebug
$adb = Join-Path $env:ANDROID_HOME 'platform-tools/adb.exe'
& $adb devices -l
# 以下只适用于仅有一台已授权连接的设备；多设备时增加 -s <序列号>
& $adb reverse tcp:18080 tcp:8080
& $adb install -r app/build/outputs/apk/debug/app-debug.apk
```

Debug 的业务 API 地址为 `http://127.0.0.1:18080/api/v1`，仅在开发变体允许所需 HTTP；WebView 的实际 origin 在本机 Go CORS 白名单中显式配置。原生页面仍由 Capacitor 内嵌资源提供，不能把这个业务 API 地址误填为替代页面来源的 `server.url`。USB 断开或映射失效时应报离线，不得回退 N100／正式域名。

结束 USB 联调后使用同一设备的 `adb reverse --remove tcp:18080` 移除映射。此处 18080 是手机端映射端口，Windows 仍监听 8080；不等同于 N100 的正式服务端口。

## 五、首包交付与最少检查

首包只保留以下检查，不建立额外 Android 自动化测试工程：

1. `assembleDebug` 成功，得到可安装的已签名 APK；检查开发 API 配置不指向 N100 或正式域名。
2. 有可用真机时只做一次安装、启动和“登录 → 保存一条笔记 → 重新查看”的基础检查，遇到阻塞问题再针对性修复。
3. 交付 APK、版本标识和简短使用说明：如何启动本机后端、USB 连接方式、已实现功能与已知限制。

若当时没有可用或已授权的真机，先交付构建成功的 APK，并如实注明“安装与启动尚未实测”，由用户安装反馈；不反复要求连接手机，也不把未测项目写成通过。基本配置与原稿保存要求仍需落实，但不在手机端重复执行整套后端测试。

多机型／多系统版本矩阵、长时间稳定性、性能压测、复杂弱网和后台保活、通知到达率、深层链接、完整升级兼容性等不作为首包门槛。真实 AI、通知和交互调整按后续需求推进；不为填满验证清单延迟交付。

后续用户需要正式发布时，再安排 Release 签名、版本升级和正式 HTTPS API 配置。保留首版签名以便继续提供同签名更新，未来切换签名前说明安装兼容性。构建中写入正式 URL 不需要连接服务器；代理实际访问正式服务仍必须事先获得用户同意。

Go／Web 正式镜像也必须在开发侧构建。当前本机没有 Docker，正式发布前单独补齐本机 Linux 容器构建能力并完成镜像检查；这不是 Android 日常开发的前提，也不能以此为理由在 N100 构建。N100 只在用户明确同意的正式部署／运维范围内接收预构建镜像并运行。

## 六、实施清单

- [x] 核对本机工具、Gradle 缓存与手机助手 ADB 版本。
- [x] 核对官方工具链资料，更新最小安装与构建方案。
- [x] 将 N100 限定为正式运行，连接必须先取得用户明确同意。
- [ ] 安装独立 JDK 21 与官方 Android SDK 管理工具，记录实际版本。
- [ ] 安装最小 SDK／Platform Tools，设置项目终端环境。
- [ ] 初始化 Vue／Capacitor 工程，锁定匹配版本和 Wrapper。
- [ ] 实现注册／登录、保存原稿和查看笔记的最小流程，开发默认模拟 AI 与通知。
- [ ] 构建带调试签名的首个 APK；可用真机上只做一次安装、启动和核心流程检查。
- [ ] 交付 APK 与本机后端／USB 使用说明，明确已测范围和已知限制。
- [ ] 收集用户实际使用反馈，再按提出的需求变更迭代。

Release 签名、完整升级验证与开发侧正式镜像构建归入后续正式发布任务，不阻塞首个 APK 交付。

本次只更新计划与开发约束，未执行上述未勾选事项。
