import type { IconName } from './components/Icon.astro';

/**
 * The platforms NSGo runs on, each shown on its own, with its kind and what draws it; zh and zh-tw name one in Chinese where its users call it otherwise, and experimental marks one not ready for most apps.
 */
export type PlatformKind = 'app' | 'web' | 'mini';

export const platforms: {
  name: string;
  zh?: string;
  'zh-tw'?: string;
  kind: PlatformKind;
  icon: IconName;
  stack: string;
  experimental?: boolean;
}[] = [
  { name: 'iOS', kind: 'app', icon: 'smartphone', stack: 'UIKit · CoreText' },
  { name: 'App Clips', zh: '轻 App', 'zh-tw': '輕 App', kind: 'app', icon: 'scan', stack: 'UIKit · CoreText' },
  { name: 'tvOS', kind: 'app', icon: 'tv', stack: 'UIKit · CoreText' },
  { name: 'watchOS', kind: 'app', icon: 'watch', stack: 'SwiftUI · CoreText' },
  { name: 'macOS', kind: 'app', icon: 'laptop', stack: 'AppKit · CoreText' },
  { name: 'Android', kind: 'app', icon: 'android', stack: 'Android Views' },
  { name: 'HarmonyOS', zh: '鸿蒙', 'zh-tw': '鴻蒙', kind: 'app', icon: 'smartphone', stack: 'ArkUI · ArkWeb', experimental: true },
  { name: 'Windows', kind: 'app', icon: 'windows', stack: 'Direct2D · DirectWrite' },
  { name: 'Linux', kind: 'app', icon: 'terminal', stack: 'GTK 4 · Pango' },
  { name: 'Web', kind: 'web', icon: 'globe', stack: 'WebAssembly · DOM' },
  { name: 'WeChat', zh: '微信', 'zh-tw': '微信', kind: 'mini', icon: 'chats', stack: 'WebAssembly · WXML', experimental: true },
  { name: 'QQ', kind: 'mini', icon: 'bird', stack: 'WebAssembly · QML', experimental: true },
  { name: 'Alipay', zh: '支付宝', 'zh-tw': '支付寶', kind: 'mini', icon: 'wallet', stack: 'WebAssembly · AXML', experimental: true },
  { name: 'Douyin', zh: '抖音', 'zh-tw': '抖音', kind: 'mini', icon: 'music', stack: 'WebAssembly · TTML', experimental: true },
];

/** platformName is how lang names a platform. */
export const platformName = (platform: (typeof platforms)[number], lang: string) =>
  (lang === 'zh' && platform.zh) || (lang === 'zh-tw' && platform['zh-tw']) || platform.name;
