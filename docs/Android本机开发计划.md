# Android 本机开发计划

更新：2026-11-07。按照最新规划，Android 移动端开发处于 **阶段三（在服务端接口闭环与 Web 全量体验优化完成后启动）**；本文维护移动端原生深度适配范围与操作步骤；状态统一在 [进度表 A01–A04](开发实施状态.md)。Vue 和 Go 已有基础流程，尚无 Capacitor 配置、Android 工程或已交付 APK，不重复初始化 Vue 或重写基础后端。

## 首包与后续边界

首包为带调试签名的 Debug APK，能安装、启动、注册／登录、保存和查看笔记。依赖 Windows 本机 Go 服务，AI／通知用 mock，交付说明必须写清。只做一次可用设备的安装、启动和核心流程检查；无设备时先交构建产物并注明未实测，不反复要求连接手机。

Release 签名、多机型、长时间稳定性、后台通知、深链、离线缓存和完整升级验证不阻塞首包。收到反馈后再安排，不能因文档列有完整目标而扩大首包验收。

## 环境准备（A01）

此前 2026-11-06 本机检查：Node.js／npm、VS Code、Go、Git 可用；默认 JDK 11；Gradle 8.14.3 缓存可运行；手机助手 ADB 可执行，但不能据此认为 SDK 或真机已就绪。完整 Android 工具链未发现。本次未重新核验或安装工具。

安装任务开始时复核现状，采用独立 JDK 21、官方 SDK 管理工具、平台包和 Platform Tools，Android Studio 可选，不要求模拟器、NDK、CMake 或全局 Gradle。早期参考组合为 Capacitor 8、AGP 8.13、Gradle 8.14.3、SDK 36、minSdk 24；这些只是候选，初始化时按选定版本官方要求确认并锁定，不能为复用缓存强行降级。

保留默认 Java，项目专用 PowerShell 终端配置（项目根目录，安装并核实实际路径后）：

```powershell
$env:JAVA_HOME = 'D:\DevTools\Java\jdk-21'
$env:ANDROID_HOME = 'D:\DevTools\Android\Sdk'
$env:Path = "$env:JAVA_HOME\bin;$env:ANDROID_HOME\platform-tools;$env:Path"
java -version
javac -version
& "$env:ANDROID_HOME\platform-tools\adb.exe" version
```

计划目录不代表已经安装。保留默认 GRADLE_USER_HOME；缓存按 Wrapper 版本和分发 URL 匹配，bin／all 不互换。提交 Wrapper 和锁文件，local.properties、SDK 绝对路径、签名与产物不提交。

## 工程接入（A02）

在现有 web 中增加并锁定 Capacitor 依赖，初始化配置并只执行一次 cap add android。候选 appId 为 space.uufang.chronolog，webDir 为 dist，最终初始化时确认。

当前 API 客户端使用固定 /api/v1，需要实现 Debug／Release 地址配置。Debug 使用 http://127.0.0.1:18080/api/v1，经 USB reverse 到 Windows 8080；该地址是业务 API，不能填作替代页面来源的 server.url。页面由 APK 内嵌资源提供。Debug 仅放行所需 HTTP，Release 使用 HTTPS；服务端实现明确的 WebView origin 白名单，当前回显 Origin 不能当作已完成白名单。

Go 按 [Windows 指南](Windows本机开发指南.md) 启动，使用 mock、隔离数据和本机 8080。USB 失效应提示无法连接，不回退 N100／正式域名。安装环境和正式连接均为独立任务，更新本文不构成授权。

## 构建和最少检查（A03）

以下仅在工程及工具齐备后执行，PowerShell 从项目根目录开始：

```powershell
Set-Location web
npm ci
npm run build
npx cap sync android
Set-Location android
.\gradlew.bat assembleDebug
$adb = Join-Path $env:ANDROID_HOME 'platform-tools/adb.exe'
& $adb devices -l
# 仅有一台已授权设备时执行；多设备必须增加 -s 指定设备
& $adb reverse tcp:18080 tcp:8080
& $adb install -r app/build/outputs/apk/debug/app-debug.apk
# 完成设备检查后移除映射
& $adb reverse --remove tcp:18080
```

安装后、移除映射前，手动启动一次并完成登录、保存、重新查看；发现阻塞再针对性修复。手机不重复整套服务端验证。没有设备时停在构建交付，不将后续步骤写成通过。

交付内容：APK、源码／版本标识、本机 Go 启动与 USB 使用说明、mock 模式、设备检查结果及未测范围。随后收集用户反馈，转为 A04 的具体任务。Release 另行安排稳定签名及备份，切换签名前说明升级兼容性。

环境变化时更新实际版本和路径；工程建立后将规划命令核实为真实命令；交付时同步进度表与验证记录。不要再维护第二份勾选进度清单。
