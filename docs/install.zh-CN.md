# 安装 Pass Quest

[English](install.md) | **简体中文**

Pass Quest 以未签名安装包的形式发布，覆盖 Windows、macOS 和 Linux：NSIS 安装程序、`.dmg` 与
AppImage。每个包都自带桌面应用、四个 agent host 脚本、内联代码所需的第三方声明，以及固定版本的
Node 22 运行时，所以 agent 功能用包内自带的 Node 运行，不需要你安装 Node（CI 对导师、Anki 和学习
简报都证明了这一点，见本指南末尾的「CI 证明了什么，什么还没有测过」）。需要加载 npm 包的 host 以单个
自包含文件发布，其余 host 是逐字节相同的副本，因此包里没有 `node_modules`，也不含 host 用不到的东西
（[`agent/README.md`](../agent/README.md)）。Anki、AnkiConnect 和任何模型都**不**在包内；使用导师
功能需要你自己填入服务商密钥。

安装包由 CI 构建并做冒烟测试（见本指南末尾的「CI 证明了什么，什么还没有测过」）。从仓库的
[GitHub Releases](https://github.com/wxxb789/passquest/releases/latest) 下载：每个 release 带三个
安装包和一份 `SHA256SUMS.txt`，其中的安装包正是门禁的 `package` 任务在打 tag 的提交上构建、安装并冒烟
测试过的那些（[发布流程](release.md)）。每次 CI 运行保留 7 天的 `package-<os>` 工件是未打 tag 的构建，
供检查用。本指南只讲如何安装打包好的应用；应用本身做什么，见[项目 README](../README.zh-CN.md)。

## 支持的平台

Windows 和 macOS 是一线支持：每次 CI 运行都会构建、安装并驱动这两个平台的安装包。Linux 属于**第二梯队**：AppImage 可用，但 CI 覆盖更少，也没有安装程序。

| 平台 | 安装包 | 支持情况 |
|---|---|---|
| Windows x64 | `pass-quest-setup-<version>-amd64.exe` | Windows 11 及更高版本 |
| macOS（Apple 芯片） | `pass-quest-<version>-arm64.dmg` | macOS 26 与 macOS 27，即最新两个大版本 |
| Linux x86_64 | `Pass_Quest-<version>-x86_64.AppImage` | 第二梯队；仅 AppImage |

**Windows 10** 不会被阻止，但不受支持，也未经测试：安装程序能运行，包也可能可用，只是没有 CI 任务覆盖 Windows 10，我们也没有在上面实测过。macOS 26 之前的版本一律不受支持：安装包把 macOS 26 声明为最低版本，Apple 的 [Find out which macOS your Mac is using](https://support.apple.com/en-us/109033) 把 macOS 27 Golden Gate 与 macOS Tahoe 26 列为最新版本。

## Windows

**安装包。** `pass-quest-setup-<version>-amd64.exe`，在 `windows-latest`（x64）上构建的按用户 NSIS 安装程序。

**安装。** 运行安装程序并按提示操作。它只给当前用户安装，路径是 `%LOCALAPPDATA%\Programs\Pass Quest`，不需要管理员权限。它会添加开始菜单项，在结束页提供桌面快捷方式，并在**设置 > 应用**里登记 *Pass Quest*；在那里选择**卸载**（或安装目录里的 `Uninstall.exe`）即可删除。

**未签名提示。** 安装程序没有代码签名证书，所以 SmartScreen 可能显示 *Windows 已保护你的电脑*。选择**更多信息**，再选择**仍要运行**。未签名的包出现这个提示是正常的，不代表文件被修改过。

<!-- screenshot: windows-smartscreen -->
*截图：SmartScreen 的「Windows 已保护你的电脑」提示与**更多信息**链接（待补）。*

**依赖。** Microsoft Edge WebView2 Runtime（系统 WebView）。Windows 11 自带；Windows 10 可能没有，安装包也不包含它。缺少它时，应用会在打开任何窗口之前显示一条消息——*"Pass Quest needs the Microsoft Edge WebView2 Runtime, which this PC does not have. Install it from https://developer.microsoft.com/microsoft-edge/webview2/ (the Evergreen Standalone Installer), then start Pass Quest again."*——然后退出。这条消息只有英文：应用在窗口出现之前就用系统对话框显示它，那时还没有界面语言可选。

## macOS

**安装包。** `pass-quest-<version>-arm64.dmg`，在 `macos-latest`（Apple 芯片）上构建。CI 不产出 Intel 版本。

**安装。** 打开磁盘映像，先把 **Pass Quest** 拖进**应用程序**（Applications）文件夹，然后推出（eject）映像，再从**应用程序**打开应用。映像是只读（read-only）的，macOS 不会替你推出它；你下面创建的例外记录的是**应用程序**里的那个应用，而从仍挂载中的映像直接启动的是另一个副本，例外并不覆盖它。

**下载副本的首次启动。** 应用只有 ad-hoc 签名，没有公证——这是负责人在 goal 0013 里的决定，没有证书，也没有 Apple 开发者账号。因此 Gatekeeper 会拦下你下载的副本的第一次启动，而 macOS 26 和 27 已经不再提供以前那种右键 → **打开**的放行方式。可行步骤：

1. 从**应用程序**打开 **Pass Quest**，让 macOS 拒绝它。
2. 打开**系统设置** → **隐私与安全性**（Privacy & Security），滚动到被拦截应用的提示，选择**仍要打开**（Open Anyway）——macOS 在尝试后大约一小时内提供这个按钮。
3. 输入你的登录密码。macOS 会把该应用记为例外，所以这个拦截只会出现这一次。
4. 再次打开 **Pass Quest**，它会正常启动。

上面这些步骤和「约一小时」的期限来自 Apple：支持页面 [Safely open apps on your Mac](https://support.apple.com/en-us/102445)（发布于 2026-05-27）和 [macOS 使用手册：Open a Mac app from an unknown developer](https://support.apple.com/guide/mac-help/open-a-mac-app-from-an-unknown-developer-mh40616/mac)。

<!-- screenshot: macos-blocked-dialog -->
*截图：下载的应用第一次启动被拒绝时 macOS 显示的对话框（待补）。*

<!-- screenshot: macos-privacy-security -->
*截图：系统设置 → 隐私与安全性里的被拦截提示与 Open Anyway 按钮（待补）。*

<!-- screenshot: macos-open-anyway-confirm -->
*截图：要求输入登录密码的确认框（待补）。*

<!-- screenshot: macos-first-launch -->
*截图：存下例外之后正常打开的 Pass Quest（待补）。*

这一步**每次下载只需做一次**：用浏览器下载新 `.dmg` 升级时，那是一个新副本，macOS 同样会拦它的首次启动。用 Homebrew 安装则完全不会遇到这个提示——它的 cask 会在下载后清掉隔离标记，所以首次启动不被拦截：

```bash
brew install --cask wxxb789/tap/pass-quest
```

<!-- screenshot: macos-homebrew-install -->
*截图：终端里 `brew install --cask wxxb789/tap/pass-quest` 完成（待补）。*

**终端替代做法。** 如果你更习惯终端，这个例外有一行等价命令，用来清掉浏览器下载写入的隔离标记：

```bash
xattr -dr com.apple.quarantine "/Applications/Pass Quest.app"
```

<!-- screenshot: macos-xattr-terminal -->
*截图：终端里 `xattr` 命令执行成功（待补）。*

不要为了绕过警告而全局关闭 Gatekeeper。上面的步骤只针对这一个应用，系统其余部分仍然受保护。

**依赖。** macOS 26（Tahoe）或 macOS 27（Golden Gate），Apple 芯片——最新两个大版本，见 Apple 的 [Find out which macOS your Mac is using](https://support.apple.com/en-us/109033)。没有别的依赖：WKWebView 属于系统，其余需要的都在包里。

## Linux

**安装包。** `Pass_Quest-<version>-x86_64.AppImage`，在 `ubuntu-latest`（x86_64）上构建。

**安装。** 赋予可执行权限并运行：

```bash
chmod +x Pass_Quest-*-x86_64.AppImage
./Pass_Quest-*-x86_64.AppImage
```

直接运行 AppImage 会通过 FUSE 挂载它：需要内核的 FUSE 支持和 `fusermount3` 工具（Debian 与 Ubuntu：`sudo apt install fuse3`；Fedora：`sudo dnf install fuse3`）。映像是静态 type2 运行时，所以不需要 `libfuse2`。FUSE 不可用时（部分容器、WSL），改用解包运行：

```bash
./Pass_Quest-*-x86_64.AppImage --appimage-extract-and-run
# or: APPIMAGE_EXTRACT_AND_RUN=1 ./Pass_Quest-*-x86_64.AppImage
```

CI 自己就是这样运行 AppImage 的，从不使用 FUSE 挂载。

**依赖。** 系统里的 GTK 3 和 WebKitGTK 4.1；AppImage 无法合理地打包它们。Debian 与 Ubuntu：`libgtk-3-0t64`（Ubuntu 24.04 和 Debian 13 之前是 `libgtk-3-0`）和 `libwebkit2gtk-4.1-0`。Fedora：`gtk3` 和 `webkit2gtk4.1`。
缺少 WebKitGTK 时，应用会显示一条同时点名两者的消息。缺少 GTK 3 时，窗口和对话框都开不了：从终端启动 AppImage，就能在 stderr 看到同样的消息。

**除 AppImage 之外没有系统集成。** 没有安装程序、没有菜单项、没有文件关联；你的桌面环境可能会自己对 AppImage 做集成。

## 数据存放位置与删除方法

数据库、窗口状态和 agent 自己的文件都放在同一个按用户划分的目录里：

| 平台 | 目录 |
|---|---|
| Windows | `%APPDATA%\Pass Quest`（Roaming AppData 文件夹） |
| macOS | `~/Library/Application Support/Pass Quest` |
| Linux | `$XDG_CONFIG_HOME/Pass Quest`，默认 `~/.config/Pass Quest` |

数据库是该目录里的 `passquest.sqlite`。`PASSQUEST_DESKTOP_DATA` 覆盖目录，`PASSQUEST_DB` 覆盖数据库文件。这个目录刻意不在安装位置里：升级会替换安装，卸载会删掉安装，而你的学习证据必须在这两件事之后仍然存在。（放在主目录里的单个二进制文件不算安装，不会被拒绝。）

**卸载会保留数据。** Windows 上，卸载程序会删除安装目录、快捷方式和注册表项，保留 `%APPDATA%\Pass Quest`；macOS 上把应用移到废纸篓；Linux 上删除 AppImage 文件。要彻底删除数据，请自己删除上面那个目录。系统 WebView 还会在它之外保留一小份自己的配置，里面有你的主题和语言选择以及缓存，没有学习记录：Windows 上是 `%LOCALAPPDATA%\Pass Quest\WebView2`，macOS 上是 `~/Library` 下该应用的 WebKit 数据与缓存目录，Linux 上是 `~/.local/share` 和 `~/.cache` 下 WebKitGTK 的数据与缓存目录。想要干净删除，这些也要一并删掉。

**密钥不在这些文件里。** 在设置里填写的模型密钥会存进操作系统凭据库——Windows 凭据管理器、macOS 钥匙串、Linux 的 Secret Service——服务名是 `com.github.wxxb789.passquest`，每个服务商一条，命名为 `api-key:<provider>`。窗口里每个服务商那一行都有删除密钥的操作；也可以直接用系统工具删除：

| 平台 | 手动删除密钥 |
|---|---|
| Windows | `cmdkey /delete:com.github.wxxb789.passquest/api-key:<provider>` |
| macOS | `security delete-generic-password -s com.github.wxxb789.passquest -a api-key:<provider>` |
| Linux | `secret-tool clear service com.github.wxxb789.passquest account api-key:<provider>` |

环境变量仍然优先于凭据库，并且永远不会被复制进去。

## 升级

用新包覆盖旧包：运行新的安装程序、替换应用程序文件夹里的应用，或替换 AppImage 文件。数据会自动保留——它在上面那个按用户划分的目录里，从不在安装位置内，新版本能打开旧版数据库。

**macOS。** 用浏览器下载的副本是一次新的下载，所以 Open Anyway 这一步会再出现一次；用 Homebrew cask 安装的副本则由 cask 替你清掉隔离标记。

不支持降级。旧版本会拒绝更新版本写出的数据库，并且不会修改它：窗口显示 *数据来自更新的 Pass Quest*，下一步是 *安装写入这些数据的更新版本；当前版本不会改动它。*，CLI 则报告
`store: database schema version N is newer than supported version M`。

**在应用里检查更新（goal 0015）。** 打开**设置**，展开 **更新**（Updates）一行：它会显示当前版本和一个 **检查更新**（Check for updates）按钮。没有任何自动检查——启动时不检查，也没有定时器——检查也从不下载、安装或写入任何东西。点击按钮会向本仓库的 GitHub Releases 发送一个 `GET` 请求（`api.github.com`，路径 `/repos/wxxb789/passquest/releases/latest`），带上应用自己设置的两个请求头 `Accept` 和 `User-Agent: PassQuest`；Go 的传输层只额外添加主机相关头（`Host`，HTTP/2 下是 `:authority`）和 `Accept-Encoding`，请求里没有账号、没有学习数据、没有设备标识，也不下载任何内容。应用会拒绝重定向，也不使用环境变量里的代理。如果存在更新版本，这一行会显示版本号、发布日期和指向其 release 页面的链接（用浏览器打开）；按上面的方式照常安装即可。如果当前版本就是已发布的最新版，这一行会说明这一点。只有当仓库还没有任何已发布 release 时，检查结果才是 *这个仓库还没有发布任何版本，所以现在没有可安装的新版本。* 草稿和预发布版本一律不会提供。如果请求到不了 GitHub——离线、被限流，或处在必须用代理的网络里——这一行会显示 *无法检查* 和一个简短原因，学习功能不受影响。

## CI 证明了什么，什么还没有测过

CI（[`.github/workflows/gate.yml`](../.github/workflows/gate.yml)）每个操作系统跑一次 `package` 任务：
`gate (windows-latest, package)`、`gate (macos-latest, package)` 和 `gate (ubuntu-latest, package)`。
每个任务从仓库构建该操作系统的安装包，安装或挂载它（Windows 用 NSIS `/S`，macOS 挂载 `.dmg` 并用 `ditto` 把 `.app` 拷出来，Linux 就地运行 AppImage），然后在 `PATH` 中没有 `go`、`bun` 和 `node`（已断言）的情况下启动已安装的应用，并驱动学习闭环、agent 功能——包括一个由已安装应用在自带的运行时和 host 上独立完成的学习简报——Anki、前向升级、密钥用例和运行时损坏用例（`package smoke` 门禁，用例为 `contents, layout, study, agents, brief, anki, upgrade, credential, broken`）。macOS 任务还会检查所构建 `.app` 的 `Info.plist` 最低版本：安装包声明 macOS 26。每次运行都把安装包保留为 `package-windows-latest`、`package-macos-latest` 或 `package-ubuntu-latest` 工件，保存 7 天。在版本 tag 上的运行会把这些工件交给发布流程，只有负责人审阅草稿后才会发布（[发布流程](release.md)）。

只有下面这些目标被声明为受支持：

| 平台 | CI 任务 | 构建并通过冒烟测试的安装包 |
|---|---|---|
| Windows x64 | `gate (windows-latest, package)` | `pass-quest-setup-*-amd64.exe` |
| macOS Apple 芯片（arm64） | `gate (macos-latest, package)` | `pass-quest-*-arm64.dmg` |
| Linux x86_64 | `gate (ubuntu-latest, package)` | `Pass_Quest-*-x86_64.AppImage` |

以下内容没有测试，因此也不作声明：

- **其他架构。** Windows arm64、macOS x86_64（Intel）和 Linux arm64：不构建也不运行任何安装包。
- **macOS 27。** 没有 GitHub 托管的运行器提供它，所以本指南按 Apple 自己列出的版本记录这两个大版本，而 CI 只证明 macOS 26。
- **代码签名与公证。** 都没有。上面的 SmartScreen 和 Gatekeeper 步骤是学习者手动越过警告的方式，CI 也无法触发 Gatekeeper 提示：在运行器上构建的 `.dmg` 不带隔离属性。
- **Linux 上的 FUSE 与显示服务器。** CI 只用解包运行（`APPIMAGE_EXTRACT_AND_RUN=1`）跑 AppImage，直接 FUSE 挂载没有覆盖；冒烟测试在 xvfb（X11）下运行，Wayland 特有的行为也没有覆盖。
- **Anki 与 AnkiConnect。** 不在包内。能连上 AnkiConnect 端点时报告存在，否则报告不存在；没有它学习功能照常可用。
- **自带运行时下的搜索。** 包里带着搜索 host，同一个解析器也能找到它，但没有任何 package 用例驱动它：`package smoke` 的用例是 `contents, layout, study, agents, brief, anki, upgrade, credential, broken`，搜索 host 的成功路径只由源码树的 `find smoke` 覆盖。持久化 host 的成功路径则在已安装的应用里验证（上面的 `brief` 用例）。
- **更新。** package smoke 不驱动更新检查及其浏览器跳转；它们由源码检出下的 `desktop smoke` 的 `update` 场景覆盖，而不是由已安装的包覆盖。
