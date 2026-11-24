// The Benchmarks page's data: the results benchmark/runner writes, one
// file a desktop in benchmark/results/, and what the frameworks offer,
// which the page compares in a table.
import type { SiteLang } from '../locales';

export type FrameworkId = 'nsgo' | 'mygo' | 'nsh' | 'gpui' | 'gpui-fast';

/**
 * The frameworks, each in a color of its own across the page's charts:
 * NSGo in the mark's blue, the others in quiet tints that tell them apart.
 */
export const frameworks: { id: FrameworkId; name: string; url: string; color: string }[] = [
  { id: 'nsgo', name: 'NSGo', url: 'https://github.com/nsgo-dev/nsgo', color: '#007aff' },
  { id: 'mygo', name: 'MyGo', url: 'https://github.com/egoist/mygo', color: '#d4a5b4' },
  { id: 'nsh', name: 'NSH', url: 'https://github.com/longbridge/nsh', color: '#9fc4af' },
  { id: 'gpui', name: 'GPUI Kit', url: 'https://github.com/longbridge/gpui-kit', color: '#b5acd4' },
  { id: 'gpui-fast', name: 'GPUI Fast', url: 'https://github.com/longbridge/gpui-fast', color: '#d8c39c' },
];

export type CaseId = 'counter' | 'quotes' | 'list';
export const caseIds: CaseId[] = ['counter', 'quotes', 'list'];

/** A program's medians, as benchmark/runner writes them. */
export interface Measure {
  startup: number;
  memory: number;
  cpu: number;
  compositor?: number;
  fps?: number;
  error?: string;
}

/** benchmark/results/<os>.json. */
export interface Results {
  date: string;
  machine: { os: string; arch: string; cpu: string; memory: number };
  runs: number;
  seconds: number;
  versions: Partial<Record<FrameworkId, string>>;
  size: Partial<Record<FrameworkId, number>>;
  cases: Partial<Record<CaseId, Partial<Record<FrameworkId, Measure>>>>;
}

export type DesktopId = 'darwin' | 'windows' | 'linux';

export const desktops: { id: DesktopId; name: string }[] = [
  { id: 'darwin', name: 'macOS' },
  { id: 'windows', name: 'Windows' },
  { id: 'linux', name: 'Linux' },
];

const files = import.meta.glob('../../../benchmark/results/*.json', {
  eager: true,
  import: 'default',
}) as Record<string, Results>;

/** The results by desktop, for those measured. */
export const results: Partial<Record<DesktopId, Results>> = Object.fromEntries(
  Object.entries(files).map(([path, data]) => [path.split('/').pop()!.replace(/\.json$/, ''), data]),
);

type LocalizedText = Record<SiteLang, string>;

const same = (value: string, overrides: Partial<LocalizedText> = {}): LocalizedText => ({
  en: value,
  zh: value,
  'zh-tw': value,
  ja: value,
  ko: value,
  es: value,
  fr: value,
  de: value,
  pt: value,
  ru: value,
  it: value,
  vi: value,
  id: value,
  tr: value,
  ...overrides,
});

/** A capability: yes, no, or yes with a word on how. */
export type Support = boolean | LocalizedText;

export interface FeatureRow extends LocalizedText {
  values: Record<FrameworkId, Support>;
}

const all = (v: Support): Record<FrameworkId, Support> => ({ nsgo: v, mygo: v, nsh: v, gpui: v, 'gpui-fast': v });
const only = (...ids: FrameworkId[]): Record<FrameworkId, Support> =>
  Object.fromEntries(frameworks.map((f) => [f.id, ids.includes(f.id)])) as Record<FrameworkId, Support>;

/** Platforms built outside the framework's own repository. */
const community: Support = {
  en: 'Community',
  zh: '社区',
  'zh-tw': '社群',
  ja: 'コミュニティ',
  ko: '커뮤니티',
  es: 'Comunidad',
  fr: 'Communauté',
  de: 'Community',
  pt: 'Comunidade',
  ru: 'Сообщество',
  it: 'Community',
  vi: 'Cộng đồng',
  id: 'Komunitas',
  tr: 'Topluluk',
};

/** Mini programs hold an app to 2 MB, which NSGo's larger apps exceed. */
const experimental: Support = {
  en: 'Experimental',
  zh: '试验性',
  'zh-tw': '實驗性',
  ja: '実験的',
  ko: '실험적',
  es: 'Experimental',
  fr: 'Expérimental',
  de: 'Experimentell',
  pt: 'Experimental',
  ru: 'Экспериментально',
  it: 'Sperimentale',
  vi: 'Thử nghiệm',
  id: 'Eksperimental',
  tr: 'Deneysel',
};

const nativeViews: Support = {
  en: 'Native views',
  zh: '原生视图',
  'zh-tw': '原生視圖',
  ja: 'ネイティブビュー',
  ko: '네이티브 뷰',
  es: 'Vistas nativas',
  fr: 'Vues natives',
  de: 'Native Views',
  pt: 'Views nativas',
  ru: 'Нативные компоненты',
  it: 'Viste native',
  vi: 'Giao diện gốc',
  id: 'Tampilan native',
  tr: 'Yerel görünümler',
};

/**
 * What each framework offers: the languages an app is written in, what
 * draws it, and where it runs.
 */
export const features: (LocalizedText & { rows: FeatureRow[] })[] = [
  {
    en: 'Building apps',
    zh: '开发',
    'zh-tw': '開發',
    ja: 'アプリ開発',
    ko: '앱 개발',
    es: 'Desarrollo',
    fr: 'Développement',
    de: 'App-Entwicklung',
    pt: 'Desenvolvimento',
    ru: 'Разработка',
    it: 'Sviluppo',
    vi: 'Phát triển ứng dụng',
    id: 'Pengembangan aplikasi',
    tr: 'Uygulama geliştirme',
    rows: [
      {
        en: 'Language',
        zh: '语言',
        'zh-tw': '語言',
        ja: '言語',
        ko: '언어',
        es: 'Lenguaje',
        fr: 'Langage',
        de: 'Sprache',
        pt: 'Linguagem',
        ru: 'Язык',
        it: 'Linguaggio',
        vi: 'Ngôn ngữ',
        id: 'Bahasa',
        tr: 'Dil',
        values: {
          nsgo: same('Go, TSX', { zh: 'Go、TSX', 'zh-tw': 'Go、TSX', ja: 'Go、TSX' }),
          mygo: same('Go'),
          nsh: same('Rust, TSX', { zh: 'Rust、TSX', 'zh-tw': 'Rust、TSX', ja: 'Rust、TSX' }),
          gpui: same('Rust'),
          'gpui-fast': same('Rust'),
        },
      },
      {
        en: 'Drawn with',
        zh: '绘制方式',
        'zh-tw': '繪製方式',
        ja: '描画方式',
        ko: '렌더링 방식',
        es: 'Renderizado con',
        fr: 'Rendu par',
        de: 'Gezeichnet mit',
        pt: 'Desenhado com',
        ru: 'Отрисовка',
        it: 'Disegnato con',
        vi: 'Cách vẽ',
        id: 'Digambar dengan',
        tr: 'Çizim yöntemi',
        values: {
          nsgo: nativeViews,
          mygo: {
            en: 'Its own GPU renderer',
            zh: '自绘 GPU 渲染',
            'zh-tw': '自繪 GPU 算繪',
            ja: '独自 GPU レンダラー',
            ko: '자체 GPU 렌더러',
            es: 'Renderizador GPU propio',
            fr: 'Son propre moteur GPU',
            de: 'Eigener GPU-Renderer',
            pt: 'Renderizador GPU próprio',
            ru: 'Собственный GPU-рендерер',
            it: 'Renderer GPU proprio',
            vi: 'Bộ dựng GPU riêng',
            id: 'Perender GPU sendiri',
            tr: 'Kendi GPU çizicisi',
          },
          nsh: nativeViews,
          gpui: {
            en: 'GPUI on the GPU',
            zh: 'GPUI GPU 渲染',
            'zh-tw': 'GPUI GPU 算繪',
            ja: 'GPUI による GPU 描画',
            ko: 'GPUI GPU 렌더링',
            es: 'GPUI en la GPU',
            fr: 'GPUI sur GPU',
            de: 'GPUI auf der GPU',
            pt: 'GPUI na GPU',
            ru: 'GPUI на GPU',
            it: 'GPUI su GPU',
            vi: 'GPUI trên GPU',
            id: 'GPUI pada GPU',
            tr: 'GPU üzerinde GPUI',
          },
          'gpui-fast': {
            en: 'GPUI on the GPU, retained',
            zh: 'GPUI GPU 渲染，增量保留',
            'zh-tw': 'GPUI GPU 算繪，增量保留',
            ja: 'GPUI GPU 描画（リテインド）',
            ko: 'GPUI GPU 렌더링, 리테인드',
            es: 'GPUI en la GPU, retenido',
            fr: 'GPUI sur GPU, retenu',
            de: 'GPUI auf der GPU, Retained',
            pt: 'GPUI na GPU, retido',
            ru: 'GPUI на GPU (с сохранением)',
            it: 'GPUI su GPU, retained',
            vi: 'GPUI trên GPU, giữ trạng thái',
            id: 'GPUI pada GPU, dipertahankan',
            tr: 'GPU üzerinde GPUI, korumalı',
          },
        },
      },
      {
        en: 'Hot reload',
        zh: '热重载',
        'zh-tw': '熱重載',
        ja: 'ホットリロード',
        ko: '핫 리로드',
        es: 'Recarga en caliente',
        fr: 'Rechargement à chaud',
        de: 'Hot Reload',
        pt: 'Hot reload',
        ru: 'Горячая перезагрузка',
        it: 'Ricaricamento a caldo',
        vi: 'Tải lại nóng',
        id: 'Hot reload',
        tr: 'Anında yeniden yükleme',
        values: only('nsgo', 'mygo', 'nsh'),
      },
      {
        en: 'Screens loaded at run time, signed',
        zh: '运行时加载已签名的界面',
        'zh-tw': '執行時載入已簽章的畫面',
        ja: '署名済み画面の実行時ロード',
        ko: '서명된 화면의 런타임 로드',
        es: 'Pantallas firmadas cargadas en ejecución',
        fr: 'Écrans signés chargés à l’exécution',
        de: 'Signierte Screens zur Laufzeit geladen',
        pt: 'Telas assinadas carregadas em execução',
        ru: 'Загрузка подписанных экранов в рантайме',
        it: 'Schermate firmate caricate a runtime',
        vi: 'Tải màn hình đã ký khi chạy',
        id: 'Layar bertanda tangan dimuat saat runtime',
        tr: 'Çalışma zamanında yüklenen imzalı ekranlar',
        values: only('nsgo', 'nsh'),
      },
    ],
  },
  {
    en: 'Desktop',
    zh: '桌面',
    'zh-tw': '桌面',
    ja: 'デスクトップ',
    ko: '데스크톱',
    es: 'Escritorio',
    fr: 'Bureau',
    de: 'Desktop',
    pt: 'Desktop',
    ru: 'Десктоп',
    it: 'Desktop',
    vi: 'Máy tính',
    id: 'Desktop',
    tr: 'Masaüstü',
    rows: [
      { ...same('macOS'), values: all(true) },
      { ...same('Windows'), values: all(true) },
      { ...same('Linux'), values: all(true) },
    ],
  },
  {
    en: 'Beyond the desktop',
    zh: '桌面之外',
    'zh-tw': '桌面之外',
    ja: 'デスクトップ以外',
    ko: '데스크톱 외 플랫폼',
    es: 'Más allá del escritorio',
    fr: 'Au-delà du bureau',
    de: 'Jenseits des Desktops',
    pt: 'Além do desktop',
    ru: 'За пределами десктопа',
    it: 'Oltre il desktop',
    vi: 'Ngoài máy tính',
    id: 'Di luar desktop',
    tr: 'Masaüstünün ötesinde',
    rows: [
      { ...same('Web'), values: all(true) },
      { ...same('iOS'), values: only('nsgo', 'nsh') },
      { ...same('Android'), values: only('nsgo', 'nsh') },
      { ...same('HarmonyOS', { zh: '鸿蒙', 'zh-tw': '鴻蒙' }), values: { ...only(), nsh: community } },
      { ...same('App Clips', { zh: '轻 App', 'zh-tw': '輕 App' }), values: only('nsgo') },
      { ...same('tvOS'), values: only('nsgo') },
      { ...same('watchOS'), values: only('nsgo') },
      {
        en: 'Home screen widgets',
        zh: '桌面小组件',
        'zh-tw': '主畫面小工具',
        ja: 'ホーム画面ウィジェット',
        ko: '홈 화면 위젯',
        es: 'Widgets de pantalla de inicio',
        fr: 'Widgets d’écran d’accueil',
        de: 'Homescreen-Widgets',
        pt: 'Widgets da tela inicial',
        ru: 'Виджеты домашнего экрана',
        it: 'Widget della schermata Home',
        vi: 'Widget màn hình chính',
        id: 'Widget layar utama',
        tr: 'Ana ekran widget’ları',
        values: only('nsgo'),
      },
      {
        en: 'Terminals',
        zh: '终端',
        'zh-tw': '終端機',
        ja: 'ターミナル',
        ko: '터미널',
        es: 'Terminales',
        fr: 'Terminaux',
        de: 'Terminals',
        pt: 'Terminais',
        ru: 'Терминалы',
        it: 'Terminali',
        vi: 'Terminal',
        id: 'Terminal',
        tr: 'Terminaller',
        values: only('nsh'),
      },
    ],
  },
  {
    en: 'Mini programs',
    zh: '小程序',
    'zh-tw': '小程式',
    ja: 'ミニプログラム',
    ko: '미니 프로그램',
    es: 'Miniprogramas',
    fr: 'Mini-programmes',
    de: 'Mini-Programme',
    pt: 'Miniprogramas',
    ru: 'Мини-программы',
    it: 'Mini-programmi',
    vi: 'Ứng dụng mini',
    id: 'Program mini',
    tr: 'Mini programlar',
    rows: [
      { ...same('WeChat', { zh: '微信', 'zh-tw': '微信' }), values: { ...only(), nsgo: experimental, nsh: community } },
      { ...same('QQ'), values: { ...only(), nsgo: experimental } },
      { ...same('Alipay', { zh: '支付宝', 'zh-tw': '支付寶' }), values: { ...only(), nsgo: experimental } },
      { ...same('Douyin', { zh: '抖音', 'zh-tw': '抖音' }), values: { ...only(), nsgo: experimental } },
    ],
  },
];

export function supportText(v: Support, lang: SiteLang): string | undefined {
  return typeof v === 'object' ? v[lang] : undefined;
}

/** "1.2 MB", "860 kB". */
export function formatBytes(n: number): string {
  if (n >= 1e9) return `${(n / 1e9).toFixed(2)} GB`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)} MB`;
  return `${Math.round(n / 1e3)} kB`;
}

export function formatMs(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(2)} s` : `${Math.round(n)} ms`;
}

export function formatPercent(n: number): string {
  return `${n.toFixed(1)}%`;
}
