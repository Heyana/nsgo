# NSGo

[English](README.md) | **简体中文**

NSGo 是一个以 Go 为后端的跨平台原生 UI 引擎。应用逻辑、响应式图和布局在 Go 中运行，各平台使用自己的原生 UI 绘制。

NSGo 参考 [Native Shell（NSH）](https://nsh.dev) 的设计，NSH 用 Rust 实现了同样的模型。NSGo 面向想要 NSH 的模型、但不想写 Rust 的团队。

## 状态

NSGo 处于设计阶段，还没有代码、发布版本或稳定的 API。[ARCHITECTURE.md](ARCHITECTURE.md) 描述目标设计。

## 目标

- 用 Go 编写应用和组件，使用 signal、memo 和 effect。
- 使用 flexbox 和 grid 布局，并用各平台自己的视图和文本渲染。
- 与 NSH 一样支持 iOS、Android、macOS、Windows、Linux 和 Web。
- 可嵌入现有原生应用，也可打包为独立应用。

## 开发

要求：Go 1.25 或更高版本。

NSGo 以 NSH 作为参考实现。将它克隆到仓库根目录的 `nsh/`，Git 会忽略该目录：

```sh
git clone https://github.com/longbridge/nsh.git nsh
```

修改代码前请先阅读 [ARCHITECTURE.md](ARCHITECTURE.md) 和 [AGENTS.md](AGENTS.md)。

## 作者

[博客](https://oneo.me) · [X @oneo666](https://x.com/oneo666)
