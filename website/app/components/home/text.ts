import type { Locale } from "~/lib/i18n"

export interface PlatformStatus {
  name: string
  state: string
  body: string
}

export interface HomeText {
  title: string
  description: string
  badge: string
  headline: [string, string]
  lead: string
  start: string
  browse: string
  copy: string
  copied: string
  windowTitle: string
  windowCaption: string
  code: {
    title: string
    lead: string
    file: string
    result: string
    points: string[]
  }
  features: {
    title: string
    lead: string
    items: { title: string; body: string }[]
  }
  platforms: {
    title: string
    lead: string
    statuses: PlatformStatus[]
    note: string
  }
  architecture: {
    title: string
    lead: string
    engine: string
    platform: string
    platformBody: string
    batch: string
    steps: [string, string, string]
    note: string
  }
  cta: { title: string; lead: string; start: string; github: string }
  footer: { copyright: string; design: string; reference: string }
}

export const homeText: Record<Locale, HomeText> = {
  en: {
    title: "NSGo — Native UI in Go",
    description:
      "A cross-platform native UI engine with a Go backend. Signals, flexbox and grid layout, and native views on macOS, iOS, the web and beyond.",
    badge: "Early preview",
    headline: ["The engine is Go.", "The views are native."],
    lead: "NSGo runs your app's logic, reactive graph and layout in Go, then draws with each platform's own views. One codebase for macOS, iOS, the web and beyond.",
    start: "Start with the docs",
    browse: "See the example",
    copy: "Copy",
    copied: "Copied",
    windowTitle: "Hello — NSGo",
    windowCaption:
      "A recreation of examples/hello in this page. The real window is drawn by NSGo's platform backend.",
    code: {
      title: "Write Go. Draw native.",
      lead: "A component is a Go function or type that returns nodes. Signals update the parts that read them, and each commit sends one batch of changes to the platform.",
      file: "counter.go",
      result: "counter.go — running",
      points: [
        "Components are ordinary Go values, not templates or markup.",
        "Signals rebuild only the bindings and regions that read them.",
        "The same code runs on macOS, iOS, the web and the targets in progress.",
      ],
    },
    features: {
      title: "Everything an app needs, without a web view",
      lead: "Go owns the state, the layout and the update model. Each backend only mounts views and reports input.",
      items: [
        {
          title: "Go all the way down",
          body: "Application logic, signals, and flexbox and grid layout run in Go. There is no JavaScript runtime, bridge or template language.",
        },
        {
          title: "Native views",
          body: "Every platform draws with its own views and text through a thin backend. NSGo ports Native Shell's design, not Rust.",
        },
        {
          title: "Signals, not diffing",
          body: "Components run once and bind signals to element parts. Regions rebuild only their span, and each commit sends what changed.",
        },
        {
          title: "Testable layout",
          body: "Styles, flexbox, grid and text layout live in the engine, so layout and behavior can be checked before a window opens.",
        },
        {
          title: "Headless tests",
          body: "Drive any app with nsgotest: press, type and find views in an ordinary go test, on any machine.",
        },
        {
          title: "Small and embeddable",
          body: "One engine is one app instance with no global state, so a host app can embed several or ship one standalone.",
        },
      ],
    },
    platforms: {
      title: "One engine, every target",
      lead: "Every backend builds on macOS with Go and Xcode alone. A target is marked as running only after it has run.",
      statuses: [
        {
          name: "macOS",
          state: "Runs",
          body: "Native views through purego and the Objective-C runtime, with no cgo.",
        },
        {
          name: "iOS",
          state: "Runs in the simulator",
          body: "The same Apple code, linked in-process by Xcode's linker.",
        },
        {
          name: "Web",
          state: "Runs",
          body: "WebAssembly and DOM elements, through syscall/js.",
        },
        {
          name: "Windows",
          state: "Builds, never run",
          body: "Win32 through syscall, drawn with GDI and native EDIT inputs.",
        },
        {
          name: "Android",
          state: "In progress",
          body: "The Go side builds. The Java runtime has never been compiled, and classes.dex and APK packaging are pending.",
        },
        {
          name: "Linux",
          state: "Planned",
          body: "GTK 4 through purego, with no GTK toolchain needed to build.",
        },
      ],
      note: "Apps test themselves headlessly, so behavior does not wait for every target to be available.",
    },
    architecture: {
      title: "One commit, one batch",
      lead: "The engine owns the reactive graph, styles, flexbox, grid and text layout. A commit resolves them into one MountBatch, and the platform applies only what changed.",
      engine: "Engine — Go",
      platform: "Platform backend",
      platformBody: "Native views, text and input",
      batch: "MountBatch",
      steps: ["Components", "Reactive graph", "Layout"],
      note: "The engine never touches main-thread-only UI objects, and the main thread never waits for the engine.",
    },
    cta: {
      title: "Build your first window",
      lead: "Add the module, copy the hello example and run it with the nsgo tool.",
      start: "Read the docs",
      github: "Star on GitHub",
    },
    footer: {
      copyright: "© ONEO",
      design: "Design follows",
      reference: "Read the architecture",
    },
  },
  zh: {
    title: "NSGo — Go 原生 UI 引擎",
    description:
      "以 Go 为后端的跨平台原生 UI 引擎。在 macOS、iOS、Web 及更多平台上使用 signal、flexbox 与 grid 布局和原生视图。",
    badge: "早期预览",
    headline: ["引擎是 Go。", "视图是原生的。"],
    lead: "NSGo 在 Go 中运行应用逻辑、响应式图和布局，再用各平台自己的视图绘制。一份代码，覆盖 macOS、iOS、Web 及更多平台。",
    start: "从文档开始",
    browse: "查看示例",
    copy: "复制",
    copied: "已复制",
    windowTitle: "Hello — NSGo",
    windowCaption:
      "本页中用网页复刻了 examples/hello。真实窗口由 NSGo 的平台后端绘制。",
    code: {
      title: "写 Go，原生绘制。",
      lead: "组件是返回节点的 Go 函数或类型。signal 更新读取它的部分，每次提交只向平台发送一批变化。",
      file: "counter.go",
      result: "counter.go — 运行中",
      points: [
        "组件是普通的 Go 值，不是模板或标记语言。",
        "signal 只重建读取它的绑定和区域。",
        "同一份代码运行在 macOS、iOS、Web 以及开发中的目标上。",
      ],
    },
    features: {
      title: "应用所需的一切，无需 Web 视图",
      lead: "状态、布局和更新模型都归 Go 所有。各平台后端只负责挂载视图并上报输入。",
      items: [
        {
          title: "从底到顶都是 Go",
          body: "应用逻辑、signal 以及 flexbox 与 grid 布局都在 Go 中运行。没有 JavaScript 运行时、桥接层或模板语言。",
        },
        {
          title: "原生视图",
          body: "每个平台通过一个轻量后端，用自己的视图和文本绘制。NSGo 移植的是 Native Shell 的设计，而非 Rust。",
        },
        {
          title: "Signal，而非 diff",
          body: "组件只运行一次，并把 signal 绑定到元素部件。区域只重建自己的范围，每次提交只发送变化。",
        },
        {
          title: "可测试的布局",
          body: "样式、flexbox、grid 和文本布局都在引擎中，因此窗口打开前就能检查布局和行为。",
        },
        {
          title: "无窗口测试",
          body: "用 nsgotest 驱动任意应用：在普通的 go test 中点击、输入并查找视图，任何机器都能运行。",
        },
        {
          title: "小巧且可嵌入",
          body: "一个引擎就是一个应用实例，没有全局状态，宿主应用可以嵌入多个，也可以单独打包。",
        },
      ],
    },
    platforms: {
      title: "一个引擎，覆盖各目标平台",
      lead: "所有后端都只需 macOS、Go 和 Xcode 即可构建。只有真正运行过的目标才会标为可用。",
      statuses: [
        {
          name: "macOS",
          state: "可用",
          body: "通过 purego 和 Objective-C 运行时使用原生视图，不依赖 cgo。",
        },
        {
          name: "iOS",
          state: "模拟器可用",
          body: "复用 Apple 平台代码，由 Xcode 的链接器在进程内链接。",
        },
        {
          name: "Web",
          state: "可用",
          body: "通过 syscall/js 使用 WebAssembly 和 DOM 元素。",
        },
        {
          name: "Windows",
          state: "可构建，未运行",
          body: "通过 syscall 调用 Win32，用 GDI 绘制并使用原生 EDIT 输入框。",
        },
        {
          name: "Android",
          state: "开发中",
          body: "Go 侧可以构建。Java 运行时尚未编译过，classes.dex 与 APK 打包也尚未完成。",
        },
        {
          name: "Linux",
          state: "计划中",
          body: "通过 purego 使用 GTK 4，构建时不需要 GTK 工具链。",
        },
      ],
      note: "应用可以无窗口地自我测试，因此行为验证不必等待每个目标平台就绪。",
    },
    architecture: {
      title: "一次提交，一批变化",
      lead: "引擎掌管响应式图、样式、flexbox、grid 和文本布局。一次提交将它们解析为一个 MountBatch，平台只应用发生变化的部分。",
      engine: "引擎 — Go",
      platform: "平台后端",
      platformBody: "原生视图、文本和输入",
      batch: "MountBatch",
      steps: ["组件", "响应式图", "布局"],
      note: "引擎不会触碰只能在主线程使用的 UI 对象，主线程也不会等待引擎。",
    },
    cta: {
      title: "构建你的第一个窗口",
      lead: "添加模块，复制 hello 示例，然后用 nsgo 工具运行它。",
      start: "阅读文档",
      github: "在 GitHub 上加星",
    },
    footer: {
      copyright: "© ONEO",
      design: "设计参考",
      reference: "阅读架构文档",
    },
  },
}
