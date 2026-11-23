// nsgo.dev's settings for nsgo-website, which builds the site from them:
// the header and footer, the home page and the docs of docs/public/.
// astro.config.mjs and the share-cards command read this file.
import { defineSite } from 'nsgo-website/config';
import { docGroups } from './src/lib/docs';
import { platforms } from './src/platforms';
import { goApp } from './src/snippets';

const repo = 'https://github.com/nsgo-dev/nsgo';
/** The gallery's web build, which opens full page in a new tab. */
const gallery = 'https://nsgo.dev/gallery/';

const { pages, groups } = docGroups();


export default defineSite({
  languages: ['en', 'zh'],
  name: 'NSGo',
  title: {
    en: 'NSGo: write it once in Go, native on every platform',
    zh: 'NSGo：用 Go 写一次，在每个平台原生运行',
  },
  description: {
    en: 'NSGo is a cross-platform native UI framework. Write an app once in Go or TSX and run it on iOS and its App Clips, tvOS, watchOS, Android, macOS, Windows, Linux and the web with each platform’s own views, and Go apps, as previews, on HarmonyOS and in WeChat, QQ, Alipay and Douyin mini programs.',
    zh: 'NSGo 是一个跨平台原生 UI 框架。用 Go 或 TSX 写一次应用，就能以各平台自己的视图运行在 iOS 及其轻 App（App Clip）、tvOS、watchOS、Android、macOS、Windows、Linux、Web 上，Go 应用还能以预览形式运行在鸿蒙上，以及微信、QQ、支付宝和抖音小程序中。',
  },
  logo: './src/assets/logo.svg',
  favicon: '/icon.png',
  appleTouchIcon: '/apple-touch-icon.png',
  repo,
  changelog: `${repo}/releases`,
  nav: [
    'features',
    'docs',
    { label: { en: 'Benchmarks', zh: '性能' }, href: 'benchmarks/' },
    { label: { en: 'Demo', zh: '演示' }, href: gallery },
    'community',
    'feedback',
    'sponsors',
  ],
  community: [
    { label: { en: 'Telegram group', zh: 'Telegram 群组' }, note: '@nsgo_dev', href: 'https://t.me/nsgo_dev', icon: 'telegram' },
    { label: 'Awesome NSGo', note: { en: 'Apps, libraries and resources', zh: '应用、库和资源' }, href: 'https://awesomensgo.dev/', icon: 'star' },
  ],
  accounts: [
    { label: 'ONEO', note: '@oneo666', href: 'https://x.com/oneo666', icon: 'x.com', avatar: './src/assets/sponsors/oneo-me.jpg' },
    { label: '小莫', note: '@suzukaze_haduki', href: 'https://x.com/suzukaze_haduki', icon: 'x.com', avatar: './src/assets/sponsors/houko.png' },
  ],
  docs: {
    sidebar: [
      ...pages.map((page) => page.slug),
      ...groups.map((group) => ({ label: group.label, items: group.pages.map((page) => page.slug) })),
    ],
  },
  sponsors: {
    maintainers: [
      {
        name: 'ONEO',
        login: 'oneo-me',
        avatar: './src/assets/sponsors/oneo-me.jpg',
        text: { en: 'Founder, building NSGo since its first line of code', zh: '创始人，从第一行代码开始打造 NSGo' },
      },
      {
        name: '小莫',
        login: 'houko',
        avatar: './src/assets/sponsors/houko.png',
        text: { en: 'Core driver, taking NSGo into its fastest growth', zh: '核心推动者，带 NSGo 进入高速发展' },
      },
    ],
    tiers: [
      {
        title: { en: 'In-kind sponsors', zh: '服务赞助商' },
        layout: 'service',
        sponsors: [
          {
            name: 'Depot',
            href: 'https://depot.dev',
            image: './src/assets/sponsors/depot-light.svg',
            imageDark: './src/assets/sponsors/depot-dark.svg',
            text: { en: 'Build machines for CI', zh: '为 CI 提供构建机器' },
          },
        ],
      },
    ],
  },
  footer: {
    copyright: '© 2026 NSGO',
    license: { label: 'Apache-2.0', href: `${repo}/blob/main/LICENSE` },
    links: [
      { label: { en: 'Docs', zh: '文档' }, href: 'docs/' },
      { label: { en: 'Benchmarks', zh: '性能' }, href: 'benchmarks/' },
      { label: { en: 'Changelog', zh: '更新日志' }, href: `${repo}/releases` },
      { label: 'GitHub', href: repo },
    ],
  },
  // The icons of the workflow's steps, which its component draws.
  icons: ['folder-plus', 'play', 'test-tube', 'package-check'],
  customCss: ['./src/styles/site.css'],
  home: {
    status: { en: 'Open source · Early preview', zh: '开源 · 早期预览' },
    headline: {
      en: ['Write it once in Go.', 'Native on every platform.'],
      zh: ['用 Go 写一次，', '在每个平台原生运行'],
    },
    lead: {
      en: `NSGo is a cross-platform UI framework. Your app’s state, layout and styles run in Go, and ${platforms.length} platforms draw it with their own native views.`,
      zh: `NSGo 是一个跨平台 UI 框架。应用的状态、布局和样式在 Go 中运行，再由 ${platforms.length} 个平台用各自的原生视图绘制。`,
    },
    actions: [
      { label: { en: 'Build your first app', zh: '开始写第一个应用' }, href: 'docs/getting-started/' },
      { label: { en: 'Try the live demo', zh: '在线体验' }, href: gallery },
    ],
    visual: './src/components/home/Platforms.astro',
    sections: {
      en: [
        { type: 'component', component: './src/components/home/Screens.astro' },
        { type: 'component', component: './src/components/home/How.astro' },
        {
          type: 'features',
          id: 'features',
          eyebrow: 'What you get',
          title: 'Everything an app needs, in one language.',
          items: [
            { icon: 'layers', title: 'Truly native', text: 'Each platform draws with its own views, text, scrolling and accessibility.' },
            { icon: 'activity', title: 'A reactive engine', text: 'Signals update only what reads them. Animation runs on the platform’s frame clock.' },
            { icon: 'layout-dashboard', title: 'Layout and styles you know', text: 'Flexbox and grid, CSS-like styles, Tailwind classes, light and dark themes, right-to-left languages.' },
            { icon: 'blocks', title: 'Components included', text: 'Unstyled in base, ready to use in ui: from buttons and menus to data tables, charts, a code editor and a terminal.' },
            { icon: 'file-code', title: 'Go or TSX', text: 'Components as Go values, or TSX with Tailwind classes, compiled ahead with no Node.' },
            { icon: 'test-tube', title: 'Tests that act like users', text: 'Find a control by its text and press it, headless or in a real window.' },
          ],
        },
        {
          type: 'split',
          eyebrow: 'The API',
          title: 'A component is just Go.',
          text: 'Elements are structs, state is a signal, and a test presses the button the way a user would.',
          items: [
            { icon: 'code', title: 'No template language', text: 'A component is a function that returns elements.' },
            { icon: 'activity', title: 'Fine-grained updates', text: 'Pressing changes the one text that reads the count.' },
            { icon: 'puzzle', title: 'Embeds anywhere', text: 'Put an NSGo view inside an iOS, Android, desktop or web app.' },
          ],
          code: { lang: 'go', code: goApp },
        },
        { type: 'component', component: './src/components/home/Workflow.astro' },
        {
          type: 'end',
          logo: 'nsgo',
          title: 'Start building with NSGo.',
          text: 'Follow the guide to install the tool and create your first app.',
          actions: [
            { label: 'Get started', href: 'docs/getting-started/' },
            { label: 'Try the live demo', href: gallery },
            { label: 'GitHub', href: repo, icon: 'github' },
          ],
        },
      ],
      zh: [
        { type: 'component', component: './src/components/home/Screens.astro' },
        { type: 'component', component: './src/components/home/How.astro' },
        {
          type: 'features',
          id: 'features',
          eyebrow: '你能得到',
          title: '应用所需的一切，都在一门语言里',
          items: [
            { icon: 'layers', title: '真正的原生', text: '每个平台用自己的视图、文本、滚动和无障碍来绘制。' },
            { icon: 'activity', title: '响应式引擎', text: 'signal 只更新读取它的地方，动画运行在平台的帧时钟上。' },
            { icon: 'layout-dashboard', title: '熟悉的布局与样式', text: 'flexbox 与 grid、类 CSS 样式、Tailwind 类名、浅色与深色主题、从右到左的语言。' },
            { icon: 'blocks', title: '内置组件', text: 'base 里是无样式的，ui 里是开箱即用的：从按钮、菜单到数据表格、图表、代码编辑器和终端。' },
            { icon: 'file-code', title: 'Go 或 TSX', text: '用 Go 值写组件，或用 TSX 配合 Tailwind 类名，提前编译，不需要 Node。' },
            { icon: 'test-tube', title: '像用户一样测试', text: '按文本找到控件并按下它，无窗口或在真实窗口中运行。' },
          ],
        },
        {
          type: 'split',
          eyebrow: 'API',
          title: '组件就是 Go',
          text: '元素是结构体，状态是 signal，测试像用户一样按下按钮。',
          items: [
            { icon: 'code', title: '没有模板语言', text: '组件就是返回元素的函数。' },
            { icon: 'activity', title: '细粒度更新', text: '按下按钮，只有读取计数的那段文本会变。' },
            { icon: 'puzzle', title: '随处嵌入', text: '把 NSGo 视图放进 iOS、Android、桌面或 Web 应用。' },
          ],
          code: { lang: 'go', code: goApp },
        },
        { type: 'component', component: './src/components/home/Workflow.astro' },
        {
          type: 'end',
          logo: 'nsgo',
          title: '开始用 NSGo 构建',
          text: '跟着指南安装工具，创建你的第一个应用。',
          actions: [
            { label: '快速开始', href: 'docs/getting-started/' },
            { label: '在线体验', href: gallery },
            { label: 'GitHub', href: repo, icon: 'github' },
          ],
        },
      ],
    },
  },
});
