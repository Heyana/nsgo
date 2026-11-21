---
title: 构建与发布
description: 为每个平台打包应用、生成安装程序、签名与公证，发布到 GitHub，并把它嵌入其他应用。
group: 工具链
order: 22
---

## 构建

```sh
go tool nsgo build
```

`build` 为 `nsgo.yaml` 列出的每个目标写出精简的发布包，放在 `dist/<name>-<version>-<target>`：`.app`、`.ipa`、`.apk`、Windows 和 Linux 的目录，或一个网站，网站页面会在 `import()` 执行时才获取它指定的 TSX 模块。`-target` 替换目标列表，`all` 表示所有只需 Go 和 Xcode 的目标；安装 Android NDK 后可再加上 `android_x64`；`miniprogram` 构建全部[小程序](#小程序)，也可单独指定 `miniprogram_wechat`、`miniprogram_qq`、`miniprogram_alipay` 或 `miniprogram_douyin`；`harmonyos` 构建[鸿蒙](#鸿蒙)预览版。`-run` 启动为本机构建的包，或把它安装到 `-device` 选中的 iPhone、模拟器或 Android 设备上。

所有要交付出去的东西都在 `dist/releases`，只需看这一个目录：

| `dist/releases` 中 | 内容 |
| --- | --- |
| `*.dmg`、`*-setup.exe`、`*.deb`、`*.pkg.tar.gz`、`*.AppImage` | macOS、Windows 和 Linux 的[安装程序](#安装程序) |
| `*-ios.ipa`、`*-tvos.ipa`、`*.apk` | iOS、tvOS 和 Android 的包，用于商店或设备 |
| `*.pkg`、`*.msix`、`*.aab`、`*.tar.gz` 和 `flatpak/` | `store` 构建的[商店包](#商店包) |
| `install.sh`、`install.ps1` | 从 [GitHub Releases](#发布到-github) 安装应用的脚本 |
| `appcast-<目标>.xml` | [自动更新](#自动更新)的应用读取的更新源 |
| `*-web.zip` | 网站，解压后原样部署即可 |
| `miniprogram/<平台>/` | 各小程序的项目，用对应开发者工具上传 |

每个文件都命名为 `<name>-<version>-<系统>-<架构>`，例如 `hello-1.2.0-windows-x64-setup.exe`，方便用户在 Release 页面找到自己系统的文件；每个文件旁都有 `.sha256`。构建其他版本时会先清空该目录；同一版本的构建（包括在其他机器上的）会往里追加。模拟器和 watchOS 的包留在各自的 `dist/<name>-<version>-<target>` 目录中。

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # 应用的 ID；省略时为 dev.nsgo. 加上 name 中的字母和数字
version: 0.1.0
build_number: 1         # CFBundleVersion 和 Android 的 versionCode；省略时由 version 推出
# copyright: © 2026 Example  # Windows 可执行文件的详细信息和安装程序，以及 macOS 的“关于”面板
# appearance: dark      # 无论系统设置如何，都使用 light 或 dark
# theme: theme.json     # TSX 应用的主题：NSGo 的主题或 shadcn/ui 主题
# i18n: { dir: locales, source: en }  # 应用的文案目录及其编写语言
icon:
  apple: assets/AppIcon.icon  # macOS、iOS 和 watchOS，或仅用于 macOS 的 .icns
  png: assets/Icon.png        # 其他所有目标
  tvos: assets/AppIconTV.brandassets  # Apple TV 的分层图标和顶部横幅，或一张宽图
  watchos: assets/AppIconWatch.icon   # Apple Watch 自己的图标，按圆形绘制，或一张方图
  android:
    foreground: assets/IconForeground.png  # Android 自适应图标：432 像素，标志放在中央 264 像素内
    background: "#1F1F1F"                 # 颜色或方形 PNG
    monochrome: assets/IconMonochrome.png  # 可选：Android 13 主题图标着色的形状
dev:
  target: native        # native、web、ios、ios_simulator、tvos、tvos_simulator、watchos、watchos_simulator、android、harmonyos 或 miniprogram_<平台>
  hot_reload: true      # false 时只构建一次，不监听文件
  # icon: { png: assets/Icon-dev.png }  # 在 dev 构建中替换 icon 的文件；默认取 assets/*-dev.*
build:
  output: dist
  tags: []              # 应用每次构建、测试和检查都使用的 Go 构建标签，例如 timetzdata
  ldflags: []           # 应用自己的链接器参数，例如 -X main.releaseDate=2026-11-17
  open: false           # true 时构建后打开输出目录
  installers: true      # false 时不生成磁盘映像、Windows 安装程序和 Linux 软件包
  jobs: 0               # 同时构建的目标数；0 为 CPU 数的一半，1 为逐个构建
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # 「通过 Apple 登录」的授权，用于 iOS、tvOS 和 Mac App Store 构建
# apple: { purchases: true }  # App 内购买所经由的 StoreKit 库，放进 macOS、iOS 和 tvOS 包
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # 向 PrivacyInfo.xcprivacy 添加应用使用的 API 和跟踪声明
# apple: { requires_full_screen: false }  # 让 iPad 在分屏、侧拉和台前调度中运行 App，支持全部四种方向
# apple: { orientations: [portrait, landscape_left] }  # 全屏 App 在 iPad 上支持的方向
# apple: { multiple_scenes: true }  # 允许在 iPad 上打开 App 的多个窗口，每个窗口显示主视图并有各自的状态
# apple: { info: { UIBackgroundModes: [audio] } }  # iOS 画中画的后台播放模式
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # 运行时所需之外的清单权限
# android: { recording: true }  # microphone.RecordWAV 录音所用的麦克风服务，连同 RECORD_AUDIO、前台服务权限和 POST_NOTIFICATIONS；从 Android 13 起 RecordWAV 会一并申请后者以显示录音通知，用户拒绝时照常录音
# android: { loopback_cleartext: true }  # 允许 APK 和 App Bundle 通过明文 HTTP 访问回环地址
# windows: { command_line: true }  # 添加 <name>.com 控制台伴侣，把参数和标准流转给 <name>.exe；store 构建不含
# windows: { publisher: Example Ltd }  # Windows 可执行文件的详细信息、安装程序和卸载项显示的发布者；省略时为 windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # Linux 软件包的维护者，省略时为应用名
# linux: { categories: [Office, Dictionary] }  # 应用桌面入口的 freedesktop 菜单分类，至少有一个 Office、Utility、Game 这样的主分类，否则菜单把它归到“其他”；自定义分类以 X- 开头
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # .deb 和 Arch 软件包在 GTK 4 和 WebKitGTK 6 之外还依赖和推荐的包，按 dpkg 或 pacman 的写法填写
# release: { github: me/hello }  # 安装脚本和自动更新从其 Releases 安装的 GitHub 仓库，省略时取 Git 远程 origin 的；tag_prefix 默认为 v
# updates: { key: <公钥> }  # 应用按平台和渠道更新的方式，见“自动更新”
# store: { dir: store }     # 商店列表目录（按语言存放的 YAML 和 Markdown）
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # 小程序的设置
```

只写操作系统名的目标表示它的全部架构（`windows` 即 `windows_arm64` 和 `windows_x64`；`android` 仅表示 `android_arm64`，因 x86_64 构建需要 NDK），可用于 `targets`、`nsgo build -target` 和 `nsgo check -target`。`macos_universal` 不在 `macos` 和 `all` 之内：它构建 macOS 的两种架构并合成一个应用（库和小组件也一并合成）。Steam 渠道使用合成后的应用包。安装器构建会生成一个磁盘映像，直接分发渠道还会生成一份 Sparkle feed。`nsgo test -target` 也接受测试实际运行的那个架构的名字：桌面系统是本机的架构，Android 是 `arm64`。所以在 x64 的 Windows 机器上 `windows_x64` 即 `windows` 目标；目标不在其上运行的架构会被拒绝，并提示应使用的名字。

命令行参数会替换同名的值，例如 `-target`、`-version` 或 `-output`；`-h` 列出全部参数。

`build.ldflags` 是应用自己的链接器参数，每一项按 go build 的 `-ldflags` 的写法书写，例如 `-X main.releaseDate=2026-11-17` 或 `-X 'main.channel=beta builds'`。链接应用的每次构建都会使用它们：`dev` 及其每次重新加载、`build`（包括 `-embed`）和 `test`（包括 `-target`），覆盖所有目标。`build` 和 `test` 的 `-ldflags` 会追加到其后而不是替换它们，需要多个值时可以重复使用。它们排在工具自己的 `-X` 参数之前，而链接器以同一变量的最后一个值为准，因此 `core.App` 中的名称、ID 和版本始终取自 `nsgo.yaml`。工具会给 go 传入自己的 `-ldflags`，它会替换 `GOFLAGS` 中的 `-ldflags`，所以链接器参数应写在这里；无窗口 `test` 中写在目录之后的 `-ldflags` 属于 go test，会替换它们全部。

工具为应用运行的每个 go 命令都使用 `build.tags`：`dev` 及其每次重新加载、`build`（包括 `-embed`）、`test`（包括 `-target`）和 `check`，覆盖所有目标。NSGo 自己的标签（例如 `dev` 中的 `nsgodev`、目标测试中的 `nsgotest`）会加在它们之后，`GOFLAGS` 中 `-tags` 的标签也会合并进来，而不是被替换；`GOFLAGS` 的其余选项照常生效。

Apple TV 的图标不是方形，而是一张分层卡片：`tvos` 取 Xcode 的 `.brandassets`（其中的 `App Icon` 堆栈和顶部横幅），或一张如 1280×768 的宽图，工具用它的边缘色铺底、让图铺满卡片，并居中生成顶部横幅；两者都没有时，`png` 按卡片高度的四分之三居中。Apple Watch 会把图标裁成圆形，所以 `watchos` 取按 watchOS 圆形绘制的 `.icon` 或一张方图；没有它时依次用 `apple` 和 `png`。两者与 `apple`、`png` 一样默认取 `assets/AppIconTV.*` 和 `assets/AppIconWatch.*`。

在 Android 上，`icon.android` 让启动图标成为自适应图标，启动器按自己的形状裁剪、按需移动它，而不是把 `png` 缩小放在启动器自己的底板上。每个图层是 108 dp 的方形，PNG 缩放到 432 像素；启动器显示中央 72 dp（288 像素），并用一个可能裁到中央 66 dp（264 像素）以外任何部分的形状遮罩它，所以标志要放在这个圆内，背景铺满整个方形。`background` 是一张 PNG，或 `"#1F1F1F"` 这样的颜色。`monochrome` 是一个只看透明度的形状，用户打开主题图标时，Android 13 及以后的版本给它着色；没有它时，图标在那里保持原色。安装包仍保留 `png` 作为普通位图图标，但 Android 9（NSGo 支持的最低版本）及以后的启动器都会选用自适应图标；没有 `png` 时，工具用前景叠在背景上画出它。没有 `icon.android` 时，启动器照旧显示 `png`。

`android.loopback_cleartext: true` 允许 APK 或 App Bundle 通过明文 HTTP 访问 `localhost`、`127.0.0.1` 和 `::1`，例如播放应用自己的本地服务器提供的视频。该网络安全策略禁止访问其他主机时使用明文。嵌入 AAR 时，在宿主应用中配置 Android 网络安全策略。

每个构建在启动时还接受 NSGo 自己的参数，例如 `--nsgo-fps`（[运行与调试](debugging.md#所有构建都接受的参数)）。在 `nsgo dev` 中，Command/Control+R 可从所有打开的窗口重新加载；当应用函数仍有其他窗口运行时，主视图关闭后也仍然可用。

## Android 原生类

在应用的 `nsgo.yaml` 中列出预编译的 Android 类，即可通过 `core.Native` 使用自定义 View。

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

相对路径以 `nsgo.yaml` 所在目录为基准。每个路径指向一个 `.dex` 文件，或一个包含 D8 输出的 JAR，其中 `classes.dex`、`classes2.dex` 等文件位于归档根目录。准备这些文件时，给 D8 传入 `--min-api 28`。有了预编译文件，NSGo 在 arm64 目标上只用 Go 就能构建 APK 或 Android App Bundle。Android 开发会话会在所列文件发生变化时重新构建。嵌入式 AAR 所用的 Java 依赖由宿主的 Android 构建提供。[D8 文档](https://developer.android.com/tools/d8)

[Android 原生示例](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native) 注册了一个带公开 `Context` 构造函数的类，并通过 Go 属性配置它。

## 安装程序

对桌面目标，`build` 会把供用户安装的文件写到 `dist/releases`，每个文件旁附带 `.sha256` 校验和：

| 目标 | 安装程序 |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`，打开后显示应用、一个箭头和“应用程序”文件夹的链接，提示把应用拖过去，用 `sign.identity` 签名并[公证](#公证) |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`，[NSIS](https://nsis.sourceforge.io) 安装程序，为当前用户安装，再次安装时装回用户之前选的目录，应用运行中会先关闭它，带开始菜单快捷方式、可选的桌面快捷方式，以及只删除自己装过的文件的卸载程序（应用为 Windows 包签名时，安装包与卸载器都[已签名](#为-windows-包签名)）；`windows.command_line` 还会安装 `.com` 伴侣，并把应用目录加入用户的 `Path`，安装后新开的控制台可直接运行应用的命令；Win+R 同样会找到伴侣，它会启动应用并关掉为它打开的控制台 |
| Linux | 供 Debian 和 Ubuntu 使用的 `<name>-<version>-linux-<arch>.deb`，以及供 Arch Linux 使用的 `<name>-<version>-linux-<arch>.pkg.tar.gz`，把应用装到 `/opt/<name>`，并带命令、桌面入口和图标；还有 `<name>-<version>-linux-<arch>.AppImage`，无需安装即可在任何发行版上运行 |

Windows 安装程序需要 NSIS 3.06 或更高版本的编译器，在 Mac 上用 `brew install makensis` 安装，因此 Mac 可以构建全部安装程序；没有它时，`build` 把脚本 `<name>.nsi` 留在包里，在任何系统上都可原样编译。安装程序提供应用文案中 NSIS 有翻译的语言，包括简体和繁体中文，并按系统语言选择。`-installers=false` 或 `build.installers: false` 只构建包本身。

安装程序可以使用应用自己的图片和图标，未指定时使用应用图标或内置图片：

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # 磁盘映像窗口的背景，窗口按它的点尺寸设定大小
  dmg_icon: assets/DMGIcon.png              # 磁盘映像的图标：.icns 或正方形 PNG
windows:
  installer_icon: assets/Setup.png          # 安装程序的图标：.ico 或正方形 PNG
  uninstaller_icon: assets/Uninstall.png    # 卸载程序的图标
```

背景 PNG 旁放一张两倍尺寸的同名图（例如 `DMGBackground@2x.png`），在 Retina 屏幕上就能保持清晰。应用图标位于距左边四分之一处，“应用程序”位于距右边四分之一处，图标连同下方的名称整体上下居中，内置图片的箭头就画在它们之间；自定义背景时请为它们留出位置。Linux 软件包没有专门的图片，安装的是应用图标。AppImage 需要 FUSE（大多数发行版自带），与软件包一样使用系统的 GTK 4；它的运行时首次构建时下载到用户缓存中。

## 商店包

`-channel store` 或 `updates.channel: store` 为各商店构建桌面和 Android 目标，由商店负责更新，不再生成安装程序和安装脚本：

| 目标 | 商店包 |
| --- | --- |
| macOS | 用于 Mac App Store 的 `<name>-<version>-macos-<arch>.pkg`：应用运行在 App Sandbox 中，用描述文件对应的身份签名，再用团队的安装器身份打包 |
| Windows | 用于 Microsoft Store 的 `<name>-<version>-windows-<arch>.msix`，发布时由商店签名 |
| Linux | 带应用 AppStream 元数据的 `<name>-<version>-linux-<arch>.tar.gz`，以及 `flatpak/<id>.yml`：Flathub 据此构建 Flatpak 的清单，在 GNOME 运行时上从 Release 下载这些归档 |
| Android | 用于 Google Play 的 App Bundle `<name>-<version>-android-<arch>.aab`，用上传密钥 `sign.keystore` 签名 |

在商店渠道，`-target macos` 选择两种 macOS 架构，和 `-target macos_universal` 一样，生成一份 `<name>-<version>-macos-universal.pkg`。使用 `-target android_arm64,android_x64` 生成一份包含 `arm64-v8a` 和 `x86_64` 的 `<name>-<version>-android-universal.aab`。构建 x86_64 需要 Android NDK。

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType，Mac App Store 要求填写
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # 网络、用户选择的文件和 apple.info 说明的权限之外的授权
windows:
  store:                               # 按 Partner Center 的“产品标识”填写
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # 应用使用的设备，由 Windows 向用户询问
linux:
  summary: Says hello                  # 软件中心显示的一句话简介
  license: MIT                         # SPDX 表达式；默认为 LicenseRef-proprietary
  # flatpak_runtime: "51"              # GNOME 运行时的版本
android:
  target_sdk: 36                       # Google Play 要求的 targetSdkVersion
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # Mac App Store 的描述文件
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # 默认取钥匙串中描述文件所属团队的安装器身份
```

未设置 `android.target_sdk` 时，运行时以 API 36 为目标。[Google Play](https://support.google.com/googleplay/android-developer/answer/11926878) 自 2026 年 8 月 31 日起要求新移动应用和更新以 API 36 为最低目标。低于 API 36 时，Android App Bundle 构建失败，独立 APK 构建发出警告后继续。

没有 `sign.macos_profile` 时，Mac 包不签名：它能在你的 Mac 上以沙盒方式运行，可用来测试沙盒，但无法上传。沙盒中的应用只能访问自己的容器、网络和用户选择的文件；小组件暂时无法在沙盒中共享时间线，所以 store 构建会拒绝带小组件的应用。用 Transporter 上传 `.pkg`，在 Partner Center 上传 `.msix`，在 Play Console 上传 `.aab`；Release 中有了清单引用的归档后，把 `flatpak/<id>.yml` 提交到 Flathub，也可以用 `flatpak-builder` 在本地构建。Windows 只安装已签名的 MSIX，商店之外安装需要签名，上架时由商店签名。

在 macOS 和 iOS 上，滚动视图的边缘效果（`Scroll.EdgeEffects`）、macOS 上没有指定 `IOS.BackdropMaterial` 的背景模糊 `Blur`，以及背景颜色滤镜（单独使用，或与没有指定 `IOS.BackdropMaterial` 的模糊组合使用）像 AppKit 和 UIKit 自己的材质与边缘效果一样，通过 Core Animation 的私有 `CABackdropLayer` 和 `CAFilter` 绘制，因为没有公开 API 能按半径模糊、让模糊程度在视图上渐变或对背景应用颜色矩阵。App Store 审核可能拒绝使用私有 API 的应用：NSGo 只在带边缘栏的滚动视图和这些背景滤镜中调用它们，但每个 macOS 和 iOS 构建都包含它们的名称。

`build` 写出的每个 Apple 包，即应用、它的小组件扩展、App Clip 和 Apple Watch 应用，以及 `-dynamic` 的 `-embed` 框架，都带有隐私清单 `PrivacyInfo.xcprivacy`，声明 NSGo 和 Go 运行时用到的需要理由的 API：用户默认设置（`CA92.1`）、文件时间戳（`C617.1`）和系统启动时间（`35F9.1`）。`apple.privacy` 添加应用自己的声明：`accessed_apis` 给出每个类别的理由，另有 `tracking`、`tracking_domains` 和 `collected_data`，后者的条目按 Apple 的命名写成 `NSPrivacyCollectedDataTypes` 的条目；`resources/` 中的 `PrivacyInfo.xcprivacy` 会与这些声明合并，而不是被替换。

iOS、tvOS 和 watchOS 的设备构建会在应用及其扩展中写入 App Store Connect 检查的 SDK 与 Xcode 元数据，并加入 `UIRequiredDeviceCapabilities: [arm64]`。模拟器构建不会写入这些仅设备条目；`apple.info` 中显式设置的值优先。`apple.multiple_scenes: true` 会在 iOS 应用的场景清单中将 `UIApplicationSupportsMultipleScenes` 设为 true，默认值为 false：此后用户可以在 iPad 上打开 App 的多个窗口，每个窗口都重新显示 App 的主视图，拥有各自的状态，同时共享 App 的信号和存储，直到用户关闭该窗口。


## 商店列表

商店元数据（标题、副标题、简介、完整描述、关键词、更新说明、链接、截图和预览视频）以纯文本 YAML 和 Markdown 存放在仓库的 `store/` 目录中（可在 `nsgo.yaml` 中用 `store.dir` 修改）：

```text
store/
  listing.yaml              # version: 1、default_locale、app_store 与 google_play 设置
  en-US/
    listing.yaml            # version: 1、title、subtitle、short_description、keywords、URL 与媒体路径
    description.md          # 完整描述（可选文件，代替 YAML 中的 description）
    release_notes.md        # 更新说明（可选文件，代替 YAML 中的 release_notes）
    promotional_text.md     # App Store 宣传文本（可选文件）
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

顶层和各语言的 `listing.yaml` 都必须包含 `version: 1` 并拒绝未知字段。规范的语言目录名（例如 `zh-Hans`）会自动映射到各商店的语言代码（App Store Connect 使用 `zh-Hans`，Google Play 使用 `zh-CN`），也可用 `app_store.locale` 或 `google_play.locale` 单独覆盖。

```sh
go tool nsgo publish -store all -dry-run                  # 仅校验字数限制、语言与媒体文件，不上传
go tool nsgo publish -store appstore,play                 # 校验并上传至 App Store Connect 与 Google Play
go tool nsgo publish -store play -track production        # 同时更新指定 Google Play 轨道的发布说明
```

校验会在读取凭据或发起任何 HTTP 请求之前完成，检查必填项、语言代码、URL 协议、媒体文件及数量（App Store 每种显示类型最多 10 张截图、3 段预览；Google Play 每种图片类型最多 8 张截图）以及字符上限（App Store：标题 30、副标题 30、关键词 100、宣传文本 170、描述 4000、更新说明 4000；Google Play：标题 30、简短描述 80、完整描述 4000、更新说明 500）。

上传凭据仅从环境变量读取：App Store Connect 读取 `NSGO_APP_STORE_KEY`、`NSGO_APP_STORE_KEY_ID` 和 `NSGO_APP_STORE_ISSUER`（未设置时回退到公证密钥 `NSGO_NOTARY_KEY`、`NSGO_NOTARY_KEY_ID` 和 `NSGO_NOTARY_ISSUER`）；Google Play 读取 `NSGO_PLAY_SERVICE_ACCOUNT`（服务账号 JSON 文件路径或 JSON 文本）。

### 商店合规

可以在 `nsgo.yaml` 中记录各项发布问卷的进度。

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

完成问卷后将对应条目标记为 `true`。匹配的 `-channel store` 或 `-channel steam` 构建会提醒你处理仍为 `false` 的条目。无需在 YAML 中跟踪的条目可以省略。

- 在 App Store Connect 填写[年龄分级问卷](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating)和[应用隐私信息](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy)。
- 根据 Apple 的[加密出口合规问卷](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation)，将应用的回答以 YAML 布尔值写入 `apple.info.ITSAppUsesNonExemptEncryption`。如果 Apple 提供批准代码，将其以字符串写入 `apple.info.ITSEncryptionExportComplianceCode`。也可以在上传时回答问卷并省略这些键。Apple 商店构建会检查这些键的类型和包内隐私清单的结构。
- 在 Play Console 填写[数据安全表单](https://support.google.com/googleplay/android-developer/answer/10787469)。仅通过内部测试轨道分发的应用可免填。
- 在提交游戏审核前，在 Steamworks 填写[内容调查](https://partner.steamgames.com/doc/gettingstarted/contentsurvey)。
## Steam depot

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

可设定 `updates.channel: steam` 作为默认渠道。请选择 macOS、Windows 和 Linux 目标。将各个 `dist/<name>-<version>-<target>` 目录分别上传到对应的 Steam depot，并将 macOS 的应用包或 Windows、Linux 的可执行文件设为启动选项。Steam 负责更新。产物不包含安装器、自更新程序或 `steam_appid.txt`。

要在 Steam 客户端之外本地测试，在 `nsgo.yaml` 中设定 `steam: {app_id: 480}`，将 480 换成你的 Steamworks App ID。`go tool nsgo dev` 会将 ID 写入原生构建目录内的 `steam_appid.txt`，并从该目录启动应用。[Steamworks API](https://partner.steamgames.com/doc/api/steam_api)、[SteamPipe](https://partner.steamgames.com/doc/sdk/uploading)。

## 发布到 GitHub

构建包含 macOS、Windows 或 Linux 目标时，`dist/releases` 还会包含 `install.sh` 和 `install.ps1`，它们从 `release.github` 指定的 GitHub 仓库的 Releases 安装应用；未指定时取应用 Git 远程 `origin` 所在的仓库，两者都没有时 `build` 不写脚本并给出提示。把其中顶层的文件（不含 `miniprogram/`）上传到标签为 `v<version>` 的 Release：

```sh
gh release create v1.2.0 dist/releases/*.*
```

之后用户用一行命令即可安装或更新应用，构建时会打印出来：

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS 和 Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

在 macOS 上，脚本把应用从磁盘映像复制到 `/Applications`，无写权限时复制到 `~/Applications`；在 Linux 上通过 `sudo` 用 `pacman` 安装 Arch 软件包，否则用 `apt-get` 或 `dpkg` 安装 Debian 软件包；在 Windows 上静默运行安装程序，装到应用已安装的目录。安装前都会先核对校验和。`<NAME>_VERSION`（应用名的大写形式，例如 `HELLO_VERSION=1.1.0`）安装其他版本，`<NAME>_DOWNLOAD_URL` 从镜像下载。

## 发布到 itch.io

在 `nsgo.yaml` 中将 `itch.project` 设为 itch.io 游戏的 `owner/game` 标识。可以按构建目标在 `itch.channels` 中覆盖频道名称。默认频道为 Web 的 `html5`、macOS 的 `osx-<arch>`、Windows 的 `windows-<arch>` 和 Linux 的 `linux-<arch>`。

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

在 CI 中将 `BUTLER_API_KEY` 设为机密变量，然后构建并上传。

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

如果覆盖 `-output` 或 `-version`，两条命令需使用相同的值。工具上传各目标的包目录，并将 itch.io 的版本设为构建版本。NSGo 使用 `PATH` 上的 `butler`，也可从 itch.io 下载固定版本。首次上传 Web 构建后，将 itch.io 游戏类型设为 HTML，并将上传的版本设为可在浏览器中运行。[Butler 文档](https://itch.io/docs/butler/pushing.html)

## 自动更新

`updates` 让应用按所在平台和用户获取它的渠道保持最新：

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # nsgo keygen 打印的公钥
  # channel: store                    # direct、store、steam 或 none；有 key 时默认 direct，没有时为 none
  # feed: https://example.com/hello/  # appcast 和安装程序所在的地址；默认为 GitHub 上最新的 Release
  # automatic: false                  # 只在要求时检查；默认为 true
```

`nsgo keygen` 打印一对密钥，不写任何文件：把公钥填进 `updates.key` 并提交；私钥不要放进仓库，存进密码管理器，并在构建发布的地方设为 `NSGO_UPDATES_KEY`，例如 GitHub Actions 的 secret。每次 direct 构建都用私钥签名安装程序，并在旁边写出 `appcast-<目标>.xml`：一个 [Sparkle](https://sparkle-project.org) 格式的更新源，应用从 `feed` 下读取它，把 appcast 和安装程序一起上传即可。没有私钥时，`build` 不写 macOS 和 Windows 的 appcast，因为它们只安装签过名的安装程序；Linux 和 Android 应用读到未签名的 appcast 时，打开该 Release 的页面，不安装其中的软件包。

`-release-notes notes.md` 把一份 Markdown 格式的更新说明（例如 changelog 中该版本的一节）写进每个 appcast，渲染为 HTML，文件里原有的 HTML 不保留。Sparkle 和 WinSparkle 在更新对话框中显示它，Linux 和 Android 的更新提示把它作为文本显示在版本号下方，最多 1,500 个字符；应用通过 `core.UpdateCheck.Notes` 读取。

| 目标 | direct 构建的更新方式 |
| --- | --- |
| macOS | `build` 打进应用的 Sparkle 每天检查，提示新版本，下载磁盘映像，校验签名，安装并重启应用；默认的应用菜单里有「检查更新…」，自己设置菜单的应用用 `updater.Check` 加上这一项 |
| Windows | 放在可执行文件旁的 WinSparkle 同样如此，WinSparkle 有该语言时用应用启动时显示的语言，并静默运行安装程序，由它启动新版本 |
| Linux、Android | 应用读取自己的 appcast，用户选择更新后下载并校验签名软件包，通过 `pkexec dpkg` 安装后重启，或交给 Android 的软件包安装器；从 Arch 软件包或 AppImage 安装的应用，以及未签名的 appcast，改为打开该 Release 的页面 |
| iOS | 应用查询负责更新它的 App Store，并打开其中的页面 |

### 渠道

渠道决定由谁更新应用。`-channel` 在单次构建中替换 `updates.channel`，所以同一个项目可以为每个渠道各构建一次：

| 渠道 | 用于 | 构建出的包 |
| --- | --- | --- |
| `direct` | GitHub、网站，以及直接安装 Release 文件的 Homebrew cask 和 winget | 按上表自行更新 |
| `store` | App Store、Mac App Store、Microsoft Store、Google Play、Flathub 和其他应用商店 | 即[商店包](#商店包)，不带更新组件；iOS 查询 App Store，不需要 key |
| `steam` | Steam | 生成由 Steam 更新的 [depot 目录](#steam-depot) |
| `none` | Linux 发行版，以及重新打包应用的人 | 不检查更新 |

应用商店会拒绝自行更新的应用，所以上传 `store` 构建，不要上传 direct 构建。direct 应用的 Homebrew cask 应写 `auto_updates true`。凡是系统能说明应用由谁安装的地方，应用也会检查：从 Google Play 或其他商店安装的 Android 应用、Flatpak 或 Snap，以及 TestFlight 构建都不检查，由各自的商店更新。

### 在应用中检查

`updater.Check(cx)` 如同“检查更新…”菜单项，检查并显示结果；`updater.CheckInBackground(cx)` 只在有新版本时提示，`updater.CheckDaily(cx)` 也是如此，但每天至多检查一次。`automatic` 开启时，Linux、Android 和 iOS 上的应用在启动时和用户回到应用时这样检查；检查失败（例如离线）时，一小时后再试。开发构建从不更新。 在 Steam 构建中，这三个函数不会触发更新检查。

## 文件类型

`file_types` 登记应用能打开的文档：macOS 上从访达、程序坞和“打开方式”打开，Windows 上通过 NSIS 安装程序或 MSIX 出现在资源管理器的“打开方式”里，Linux 上通过 Debian、Arch、AppImage 和 Flatpak 包的桌面项登记：

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # 默认为 <id>.<第一个扩展名>；用别人的 ID 则导入该类型
    # icon: assets/Flow.icns       # 或正方形 PNG
    # role: viewer                 # 默认为 editor
    # conforms: [public.json]      # 默认为 public.data
    # mime: [application/json]     # 已有的 MIME 类型；省略时 Linux 自行推断
```

Windows 安装程序把应用加进“打开方式”，不替换用户的默认程序，并在升级和卸载时移除自己的登记。Linux 用固定的表查出常见扩展名的 MIME 类型；应用自有类型中表里没有的扩展名得到 `application/x-<类型 ID>` 和一份共享 MIME-info 定义。`mime` 改用已有的类型；导入的类型若有表里没有的扩展名，必须写它。只运行 AppImage 不会登记任何东西，要由桌面集成工具安装它的桌面项和定义。

应用用 `desktop.OnOpenFiles` 接收这些文件，在 Windows 和 Linux 上也包括应用运行时再次启动所带的文件。启动应用时带来的文件会等到第一个处理函数注册：

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

`desktop.AddRecentFile(cx, path)` 把用户打开过的文档列进 Dock 菜单和“文件 › 打开最近使用”，选中后同样交给这个处理函数；`desktop.ClearRecentFiles` 清除这些记录。

## 链接

`url_schemes` 让应用拥有自己的链接，例如 `myapp://settings`，可以从浏览器、其他应用或 `open`、`start`、`xdg-open` 命令打开它。macOS、iOS 和 Android 应用、Windows 的安装程序和 MSIX 包，以及 Linux 的桌面项（Debian、Arch、AppImage 和 Flatpak 包都带着它）会注册这些链接，卸载时一并移除。

```yaml
url_schemes: [myapp]
```

应用用 `desktop.OnOpenLinks` 接收这些链接，运行中也能收到：在 Windows 和 Linux 上，第二次启动会把链接交给正在运行的应用后退出。启动应用时带来的链接会等到第一个处理函数注册。任何人都能构造这样的链接，而且在 Windows 和 Linux 上，应用启动时带的任何 URL 都会交给处理函数，所以先检查 scheme，并在链接改变任何东西之前征得用户确认：

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

在 iOS 和 Android 上，应用用 `mobile.OnOpenLinks` 接收启动时和运行中打开的链接。在 iOS 模拟器上可以用 `xcrun simctl openurl booted myapp://settings` 打开，在 Android 上可以用 `adb shell am start -a android.intent.action.VIEW -d myapp://settings` 打开。测试里用 `app.OpenLinks("myapp://settings")` 送入链接。

## 接收 Android 分享

在 `nsgo.yaml` 中声明应用可以从 Android 分享面板接收的 MIME 类型：

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

APK 和 App Bundle 都会为单项和多项分享注册这些类型。用 [`mobile.OnShared`](services.md#打开链接与文件) 接收每次分享。

## 小程序

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

每个目标写出对应平台开发者工具的项目，也放在 `dist/releases/miniprogram/<平台>`，`-run` 和 `dev` 会用该工具打开；`dev` 在你保存时重新构建，工具随之刷新：

| 目标 | 平台 | 工具 |
| --- | --- | --- |
| `miniprogram_wechat` | 微信 | [微信开发者工具](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html)，需开启服务端口（设置 → 安全设置） |
| `miniprogram_qq` | QQ | [QQ 小程序开发者工具](https://q.qq.com/wiki/tools/devtool/)，限 macOS |
| `miniprogram_alipay` | 支付宝 | [支付宝小程序开发者工具](https://opendocs.alipay.com/mini/ide/download)，限 macOS |
| `miniprogram_douyin` | 抖音及字节系其他应用 | [抖音开发者工具](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download)，限 macOS |

`miniprogram.appid` 按平台（`wechat`、`qq`、`alipay`、`douyin`）设置小程序的 AppID；没有时微信开发者工具以测试号预览，其他工具会要求填写。微信的 `-run` 和 `dev` 每次重新构建都会通过 CLI 关闭并自动打开项目，因此开发者工具会编译最新文件。此流程已用 2.01.2510290 版开发者工具检查。平台的导航栏显示应用的标题；开启 `custom_navigation` 后应用占满页面，安全区让它避开右上角的胶囊按钮。应用读到的是手机的操作系统，`Env.Simulated` 为 true，与网页的 `--nsgo-os` 相同：导航栈的页面显示自己的 `Header`，没有系统标题栏代替它。QQ、支付宝和抖音尚未在各自的工具中试过；支付宝暂不支持由应用处理返回、关键帧动画和键盘高度。

小程序为试验性支持。小程序只运行 Go 应用，不运行 TSX，且各平台把应用压缩后的模块限制在 2 MB 以内：基于 `base` 的几个页面的应用放得下，`examples/gallery-miniprogram` 就是一例，用 `ui` 控件的应用暂时放不下，放不下时 `build` 会说明。着色器在小程序中不绘制，网页视图、视频、原生组件和应用自带的字体文件暂不显示。

## 鸿蒙

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos` 在 `dist/releases` 写出 `<name>-<version>-harmonyos.hap`。引擎以 WebAssembly 运行在 ArkWeb 中，ArkTS 宿主用 ArkUI 视图绘制它挂载的内容，文字、输入框、滚动和对话框都是系统自己的。`PATH` 中有 `hdc` 时，`-run` 把 HAP 安装到连接的设备或模拟器上并启动，`nsgo test -target harmonyos` 使用独立的 `.nsgotest` bundle 标识在模拟器上运行未签名的测试 HAP，并在结束后删除测试 HAP，因此已安装的应用仍然可用。

要从模板编译 ArkTS 宿主并用应用的 `id` 作为包名，请将 `hvigorw` 加入 `PATH`，并将 `OHOS_BASE_SDK_HOME` 设为包含 API 26（`26.0.0`）的 OpenHarmony SDK 路径。模板的编译和目标 SDK 版本都是 `26.0.0`。`build.harmonyos_bundles` 可选 `auto`（默认）、需要 SDK 的 `exact`，或无需 SDK 的 `padded`（将 id 填充到预编译宿主的固定长度）。目标测试使用同一设置，并输出打包时使用的宿主来源。`harmonyos.permissions` 列出应用要申请的 `ohos.permission` 名称，如 `MICROPHONE` 和 `CAMERA`；`widget.title` 是桌面卡片显示的标题。

鸿蒙为预览支持。模拟器可安装未签名的 HAP。[架构说明](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md)列出了预览版已支持的 ArkUI 功能。

### HAP 签名

设置 `harmonyos.signing` 后，构建会用你的 AppGallery Connect 资料为 HAP 签名。直接安装到华为手机时，使用覆盖该设备的调试证书和 profile。发布时使用发布资料。[华为签名指南](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

路径相对于 `nsgo.yaml`。在环境中设置 `NSGO_HARMONY_KEYSTORE_PASSWORD` 和 `NSGO_HARMONY_KEY_PASSWORD`。签名使用 SDK 的 `hap-sign-tool.jar`，并从 `JAVA_HOME` 或 `PATH` 找到 Java。构建保留应用的完整 `id`，使其与 profile 的包名匹配，因此 `auto` 模式签名也需要 SDK 和 `hvigorw`。在应用目录运行 `go tool nsgo doctor` 可检查签名配置和 profile 包名，无需密码。`build -run` 会把已签名的 HAP 交给 `hdc` 安装。

## 签名

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # 为 macOS 应用签名，省略时为临时签名
  profile: assets/AppStore.mobileprovision  # 为 IPA 签名
  keystore: assets/release.p12              # 与 key_alias 一起为 APK 签名
  certificate: assets/codesign.pfx          # 为 Windows 应用及其安装包签名
  # widgets_profile: assets/Widgets.mobileprovision  # 为 IPA 中的小组件扩展签名
  # clip_profile: assets/Clip.mobileprovision        # 为 IPA 中的轻 App 签名
```

密钥库的密码来自 `NSGO_KEYSTORE_PASSWORD` 和 `NSGO_KEY_PASSWORD`。`-team` 用于为 iPhone 包签名。命令行可用 `-profile`、`-widgets-profile`、`-clip-profile` 和 `-macos-profile` 替换应用、小组件扩展、轻 App 和 Mac App Store 的描述文件，`-installer` 替换 Mac Installer Distribution 身份。

### 为 Windows 包签名

不带 Authenticode 签名、或签名证书不被 SmartScreen 识别的 Windows 下载，打开前会收到警告。`sign.certificate` 是一个 `.pfx`，为可执行文件、命令行伴随程序、应用资源里自带的程序与库、以及安装包和其中的卸载器签名；密码来自 `NSGO_CERTIFICATE_PASSWORD`。签名在 Go 中生成，与 `signtool` 的结果相同，因此在 Mac 上不需要 Windows SDK：

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # 为签名加时间戳的 RFC 3161 服务器
```

`sign.timestamp_url` 默认为 DigiCert 的服务器；设为 `none` 则不加时间戳，签名随证书到期而失效。证书在硬件令牌或云服务中时，改用一条命令签名，`%1` 是待签文件，`build` 对上面每个文件都运行它：

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

在安装包里签卸载器需要 NSIS 3.08 或更高版本，且只能由 `.pfx` 完成，`sign.sign_command` 无法签卸载器。命令行可用 `-certificate`、`-sign-command` 和 `-timestamp-url` 指定这些值。

### 公证

经 Apple 公证的应用在 Mac 上下载后打开时不会出现 Gatekeeper 警告。用 Developer ID 签名，并指定 notarytool 存在钥匙串中的凭据：

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # 上面的钥匙串配置；也可用 -notarize hello
```

之后 `build` 会以强化运行时签名，提交磁盘映像，等待 Apple 的结果（通常几分钟），并把公证票据装订到磁盘映像和应用上；被拒时会打印 Apple 的日志。在 CI 中可用 `NSGO_NOTARY_KEY`、`NSGO_NOTARY_KEY_ID` 和 `NSGO_NOTARY_ISSUER`（App Store Connect API 密钥的 `.p8` 文件、ID 和 issuer）代替钥匙串配置。强化运行时默认禁止使用麦克风、摄像头、日历和定位，`apple.info` 含 `NSMicrophoneUsageDescription`、`NSCameraUsageDescription`，或日历、提醒事项、定位的用途说明时，`build` 会为应用声明相应权限。

## 小组件与实时活动

`build` 和 `dev` 会用 Xcode 的 Swift 编译器把绘制应用的小组件和实时活动的扩展编进 macOS 和 iOS 包，不需要单独的步骤。在 iOS 上，应用与小组件共享一个 App Group：用 `-run` 装到 iPhone 时，Xcode 会为两者签名并注册它；打 IPA 时需要为 `<id>` 和 `<id>.widgets` 都开启它，并用 `sign.widgets_profile` 为扩展签名。Android 16 把实时活动显示为 Live Update，不需要扩展。[小组件](widgets.md)和[实时活动](live-activities.md)介绍它们的配置和 API。

## 轻 App（App Clip）

轻 App 是 iOS 应用中的一小部分，用户从链接、App Clip 码或 NFC 标签打开它，无需安装应用。`app_clip` 把它编进 iOS 包，它有自己的 main 包，只链接自己显示的内容：

```yaml
app_clip:
  main: ./clip              # 轻 App 的 main 包，默认为应用自己的
  domains: [example.com]    # 哪些域名的链接打开它
  # name: Hello Clip        # 默认为 "<name> Clip"
  # id: dev.example.hello.Clip  # 默认为 <id>.Clip
  # minimum: "17.0"         # 默认 16.0；17.0 在只从链接打开时允许 100 MB
  # notifications: true     # 请求一天内的通知
  # location: true          # 让轻 App 确认打开它的位置
```

轻 App 用 `mobile.OpenedURL` 读取打开它的链接，再从另一个链接打开时它随之变化；`mobile.IsAppClip` 说明代码是否作为轻 App 运行。系统下载的轻 App 最大 15 MB，iOS 17 起只从链接打开时最大 100 MB，超出时 `build` 会拒绝；链接 `ui` 的轻 App 约 11 MB。`dev -target ios_simulator -clip https://example.com/order?table=12` 在模拟器中单独运行轻 App，如同从该链接打开；`-target ios` 则在 iPhone 上运行，并打印该链接的二维码：在“设置 → 开发者 → Local Experiences”中登记这个链接后，用相机扫码即可打开轻 App。Apple 只为 Apple Developer Program 的团队签名轻 App，个人团队不行。上架 App Store 时需要为 `<id>` 开启 App Clips，用 `sign.clip_profile` 为轻 App 签名，并在这些域名上提供 apple-app-site-association 文件。仓库中的 `examples/appclip` 是一个咖啡馆，它的轻 App 在桌边点单。

## 嵌入其他应用

`-embed` 把应用打包给你自己的 iOS 或 Android 应用使用：一个 XCFramework（加 `-dynamic` 时由动态框架组成）或 AAR，通过其中的 `NSGoView` 或 `NsgoView` 显示应用。Go 桌面程序用 `nsgo.NewView` 显示应用，网页则用站点的 `nsgo.js`，其主机为页面所在的主机，或 `nsgo.yaml` 中 `web.host` 指定的主机。仓库中的 `examples/embed-*` 分别给出了示例；宿主侧见[嵌入](embedding.md)。
