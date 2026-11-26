// The Benchmarks page's words across all supported site languages.
import type { SiteLang } from '../locales';

const en = {
  title: 'Benchmarks',
  description:
    'NSGo against MyGo, Fyne, Wails, NSH, GPUI Kit and GPUI Fast: the same three apps measured on macOS, Windows and Linux, and where each framework runs.',
  heading: 'The same apps, seven frameworks.',
  lead: 'Three small apps, written for each framework the way it is meant to be used, built as releases and measured one after another on the same machine.',
  desktops: 'Desktop',
  missing: 'Not measured on this desktop yet.',
  machine: (date: string, cpu: string, memory: string, runs: number, seconds: number) =>
    `Measured on ${date} on ${cpu} with ${memory} of memory: the median of ${runs} runs, ${seconds} s each.`,
  versions: 'Versions',
  lower: 'Lower is better',
  higher: 'Higher is better',
  failed: 'Failed',
  best: 'Best',
  cases: {
    counter: {
      title: 'Counter',
      body: 'A count and two buttons, left idle: how fast an app opens and what it costs to keep it open.',
    },
    quotes: {
      title: 'Streaming quotes',
      body: 'A table of 40 quotes, ten of which change every display frame, four texts each.',
    },
    list: {
      title: 'Scrolling list',
      body: '10,000 rows of a dot and two texts, scrolled a row further every display frame.',
    },
  },
  metrics: {
    startup: 'Startup',
    memory: 'Memory',
    size: 'Binary size',
    cpu: 'CPU',
    fps: 'Frames per second',
  },
  app: 'App',
  compositor: 'Compositor',
  cpuNote:
    'Percent of one core. Native views leave their drawing to the system compositor, where a GPU renderer draws in its own process, so each bar adds the compositor’s CPU to the app’s.',
  fpsNote: 'Updates follow the display: an app that keeps up stays at its refresh rate. Fyne draws 60 frames a second and the system webview animates at 60, whatever the display.',
  features: {
    title: 'Where each one runs.',
    yes: 'Yes',
    no: 'No',
  },
  method: {
    title: 'How it is measured.',
    items: [
      'Each app prints a line on its first frame, then every second how many frames it updated. The runner times the first line from the start and reads the process’s CPU time and memory from the system.',
      'Startup runs from launching the binary to the first frame. Memory is read at the end of a run: the physical footprint on macOS, the proportional set size on Linux, the working set on Windows. On macOS a system webview’s WebKit processes count with the app that started them.',
      'CPU is measured for 10 seconds after 3 to settle, the apps taking turns. The compositor is WindowServer on macOS, the Desktop Window Manager on Windows, and the Wayland compositor and Xwayland on Linux, less what it spends with none of the apps open.',
      'Release builds: go build -ldflags "-s -w", and cargo build --release with symbols stripped. GPUI Kit and GPUI Fast share one source, built against upstream GPUI and against GPUI Fast. Fyne builds with -tags release and Wails with -tags production, as fyne package -release and wails3 build do.',
    ],
    source: 'The apps and the runner',
  },
  fair: {
    title: 'Something unfair?',
    body: 'These apps were written by NSGo’s authors. If one is not written the way its framework would write it, uses an older version, or a measurement favors one framework, open an issue with what to change and we will measure again.',
    action: 'Report it in an issue',
  },
};

export type BenchmarkCopy = typeof en;

const zh: BenchmarkCopy = {
  title: '性能',
  description: 'NSGo 与 MyGo、Fyne、Wails、NSH、GPUI Kit、GPUI Fast 的对比：同样的三个应用在 macOS、Windows 和 Linux 上的测量结果，以及各框架支持的平台。',
  heading: '同样的应用，七个框架',
  lead: '三个小应用，按每个框架推荐的写法各实现一遍，以发布模式构建，在同一台机器上依次测量。',
  desktops: '桌面平台',
  missing: '还没有在这个平台上测量。',
  machine: (date, cpu, memory, runs, seconds) => `${date} 测于 ${cpu}，内存 ${memory}：取 ${runs} 次运行的中位数，每次 ${seconds} 秒。`,
  versions: '版本',
  lower: '越低越好',
  higher: '越高越好',
  failed: '失败',
  best: '最优',
  cases: {
    counter: {
      title: '计数器',
      body: '一个数字和两个按钮，打开后保持空闲：应用打开有多快，开着要花多少资源。',
    },
    quotes: {
      title: '实时行情',
      body: '40 行行情表，每个显示帧更新其中 10 行，每行 4 段文字。',
    },
    list: {
      title: '滚动列表',
      body: '10,000 行，每行一个圆点和两段文字，每个显示帧向下滚动一行。',
    },
  },
  metrics: {
    startup: '启动时间',
    memory: '内存',
    size: '二进制体积',
    cpu: 'CPU',
    fps: '每秒帧数',
  },
  app: '应用',
  compositor: '合成器',
  cpuNote: '以单核为 100%。原生视图把绘制交给系统合成器，而 GPU 渲染在应用自己的进程里绘制，所以每条都把合成器的 CPU 加在应用的之上。',
  fpsNote: '更新跟随显示器：跟得上的应用保持在屏幕刷新率。Fyne 固定每秒绘制 60 帧，系统 WebView 的动画也是 60 帧，与屏幕无关。',
  features: {
    title: '各自能跑在哪里',
    yes: '支持',
    no: '不支持',
  },
  method: {
    title: '测量方法',
    items: [
      '每个应用在第一帧打印一行，之后每秒打印更新了多少帧。测量程序从启动开始计时到第一行，并从系统读取进程的 CPU 时间和内存。',
      '启动时间是从启动二进制到第一帧。内存在每次运行结束时读取：macOS 为物理内存占用（footprint），Linux 为按比例分摊的常驻内存（PSS），Windows 为工作集。macOS 上，系统 WebView 的 WebKit 进程计入启动它的应用。',
      'CPU 在稳定 3 秒后测量 10 秒，各应用轮流运行。合成器在 macOS 是 WindowServer，Windows 是桌面窗口管理器（DWM），Linux 是 Wayland 合成器和 Xwayland，并扣除未打开任何测试应用时它自身的占用。',
      '均为发布构建：go build -ldflags "-s -w"，以及去除符号的 cargo build --release。GPUI Kit 与 GPUI Fast 共用一份源码，分别基于上游 GPUI 和 GPUI Fast 构建。Fyne 加 -tags release、Wails 加 -tags production 构建，与 fyne package -release 和 wails3 build 一致。',
    ],
    source: '测试应用与测量程序',
  },
  fair: {
    title: '发现不公平的地方？',
    body: '这些应用由 NSGo 的作者编写。如果某个实现不符合它所在框架的推荐写法、用了旧版本，或者测量方式偏向了某个框架，欢迎在 Issue 中指出应该怎么改，我们会重新测量。',
    action: '在 Issue 中报告',
  },
};

const zhTw: BenchmarkCopy = {
  title: '效能基準',
  description: 'NSGo 與 MyGo、Fyne、Wails、NSH、GPUI Kit、GPUI Fast 的對比：同樣三個應用在 macOS、Windows 與 Linux 上的實測數據，以及各框架支援的平台。',
  heading: '同樣的應用，七個框架',
  lead: '三個小型應用，依照每個框架推薦的方式各實作一次，以發佈模式建置，並在同一台機器上依序測量。',
  desktops: '桌面平台',
  missing: '尚未在此桌面平台上測量。',
  machine: (date, cpu, memory, runs, seconds) => `${date} 實測於 ${cpu}，記憶體 ${memory}：取 ${runs} 次執行的中位數，每次 ${seconds} 秒。`,
  versions: '版本',
  lower: '越低越好',
  higher: '越高越好',
  failed: '失敗',
  best: '最佳',
  cases: {
    counter: {
      title: '計數器',
      body: '一個數字與兩個按鈕，開啟後保持閒置：測量應用啟動速度以及維持開啟的資源耗用。',
    },
    quotes: {
      title: '即時行情',
      body: '40 列行情表，每個顯示影格更新其中 10 列，每列 4 段文字。',
    },
    list: {
      title: '捲動清單',
      body: '10,000 列，每列一個圓點與兩段文字，每個顯示影格向下捲動一列。',
    },
  },
  metrics: {
    startup: '啟動時間',
    memory: '記憶體',
    size: '執行檔大小',
    cpu: 'CPU',
    fps: '每秒影格數',
  },
  app: '應用',
  compositor: '合成器',
  cpuNote: '以單一核心為 100%。原生視圖將繪製交給系統合成器，而 GPU 算繪則在應用自身行程中完成，因此每條數據都將合成器的 CPU 加在應用之上。',
  fpsNote: '更新跟隨顯示器：能跟上的應用會維持在螢幕更新率。Fyne 固定每秒繪製 60 格，系統 WebView 的動畫也是 60 格，與螢幕無關。',
  features: {
    title: '各自能執行於哪些平台',
    yes: '支援',
    no: '不支援',
  },
  method: {
    title: '測量方式',
    items: [
      '每個應用在第一個影格輸出文字，接著每秒輸出更新的影格數。測量程式從啟動計時至第一行輸出，並向系統讀取行程的 CPU 時間與記憶體。',
      '啟動時間從啟動執行檔算至第一個影格。記憶體於每次執行結束時讀取：macOS 為實體記憶體佔用（footprint），Linux 為比例常駐集（PSS），Windows 為工作集（working set）。macOS 上，系統 WebView 的 WebKit 程序計入啟動它的應用。',
      'CPU 在穩定 3 秒後量測 10 秒，各應用輪流執行。合成器在 macOS 為 WindowServer、Windows 為桌面視窗管理員（DWM）、Linux 為 Wayland 合成器與 Xwayland，並扣除未開啟測試應用時的基準耗用。',
      '均為發佈建置：go build -ldflags "-s -w"，以及移除符號的 cargo build --release。GPUI Kit 與 GPUI Fast 共用同一份原始碼，分別建置於上游 GPUI 與 GPUI Fast。Fyne 加 -tags release、Wails 加 -tags production 建置，與 fyne package -release 和 wails3 build 一致。',
    ],
    source: '測試應用與測量程式',
  },
  fair: {
    title: '發現不公平之處？',
    body: '這些應用由 NSGo 作者群撰寫。若某個實作不符合該框架的慣用寫法、使用了舊版本，或測量方式偏袒某個框架，請開啟 Issue 告訴我們如何改進，我們將重新測量。',
    action: '在 Issue 中回報',
  },
};

const ja: BenchmarkCopy = {
  title: 'ベンチマーク',
  description: 'NSGo と MyGo・Fyne・Wails・NSH・GPUI Kit・GPUI Fast の比較：同じ 3 つのアプリを macOS・Windows・Linux で計測した結果と、各フレームワークの対応プラットフォーム。',
  heading: '同じアプリ、7 つのフレームワーク。',
  lead: '3 つの小さなアプリをそれぞれのフレームワーク本来の書き方で実装し、リリースビルドにして同じマシン上で順に計測しました。',
  desktops: 'デスクトップ',
  missing: 'このデスクトップ環境ではまだ計測されていません。',
  machine: (date, cpu, memory, runs, seconds) => `${date} に ${cpu}（メモリ ${memory}）で計測：各 ${seconds} 秒 × ${runs} 回の中央値。`,
  versions: 'バージョン',
  lower: '低いほど優秀',
  higher: '高いほど優秀',
  failed: '失敗',
  best: '最良',
  cases: {
    counter: {
      title: 'カウンター',
      body: '数値 1 つとボタン 2 つを開いて待機：起動の速さとアイドル時のリソース消費を測ります。',
    },
    quotes: {
      title: 'ストリーミング気配値',
      body: '40 行の価格表で毎フレーム 10 行（各 4 テキスト）を更新します。',
    },
    list: {
      title: 'スクロールリスト',
      body: 'ドット 1 つとテキスト 2 つからなる 10,000 行を、毎フレーム 1 行ずつスクロールします。',
    },
  },
  metrics: {
    startup: '起動時間',
    memory: 'メモリ',
    size: 'バイナリサイズ',
    cpu: 'CPU',
    fps: 'フレームレート',
  },
  app: 'アプリ',
  compositor: 'コンポジタ',
  cpuNote: '1 コアあたりの割合です。ネイティブビューは描画をシステムのコンポジタに委ねる一方、GPU レンダラーは自プロセス内で描画するため、コンポジタの CPU 使用率をアプリの分に加算しています。',
  fpsNote: '更新はディスプレイに同期します。追従できるアプリはリフレッシュレートを維持します。Fyne は毎秒 60 フレームで描画し、システムの WebView もディスプレイに関係なく 60 フレームでアニメーションします。',
  features: {
    title: '各フレームワークの動作先。',
    yes: '対応',
    no: '非対応',
  },
  method: {
    title: '計測方法。',
    items: [
      '各アプリは最初のフレームで 1 行出力し、その後 1 秒ごとに更新フレーム数を出力します。ランナーは起動から最初の出力までを計測し、プロセスの CPU 時間とメモリを OS から取得します。',
      '起動時間はバイナリ起動から最初のフレームまでです。メモリは計測終了時に取得します（macOS は physical footprint、Linux は PSS、Windows は working set）。macOS では、システム WebView の WebKit プロセスも起動したアプリに含めます。',
      'CPU は 3 秒の安定化後に 10 秒間、各アプリを順番に計測します。コンポジタは macOS では WindowServer、Windows では DWM、Linux では Wayland コンポジタと Xwayland で、アプリ未起動時の基準値を差し引いています。',
      'リリースビルドは go build -ldflags "-s -w" およびシンボル除去付き cargo build --release を使用。GPUI Kit と GPUI Fast は同一ソースをそれぞれのクレートに対してビルドしています。Fyne は -tags release、Wails は -tags production でビルドし、fyne package -release と wails3 build に合わせています。',
    ],
    source: 'テストアプリとランナー',
  },
  fair: {
    title: '不公平な点がありますか？',
    body: 'これらのアプリは NSGo の作者が書きました。各フレームワークの推奨される書き方に合っていない、バージョンが古い、または計測方法が偏っている場合は、修正点を Issue でお知らせください。再計測します。',
    action: 'Issue で報告する',
  },
};

const ko: BenchmarkCopy = {
  title: '벤치마크',
  description: 'NSGo와 MyGo, Fyne, Wails, NSH, GPUI Kit, GPUI Fast 비교: macOS, Windows, Linux에서 동일한 세 앱을 측정한 결과와 각 프레임워크의 지원 플랫폼.',
  heading: '같은 앱, 일곱 개의 프레임워크.',
  lead: '세 가지 작은 앱을 각 프레임워크가 권장하는 방식대로 작성해 릴리스 모드로 빌드하고, 같은 머신에서 차례로 측정했습니다.',
  desktops: '데스크톱',
  missing: '이 데스크톱에서는 아직 측정되지 않았습니다.',
  machine: (date, cpu, memory, runs, seconds) => `${date} ${cpu}(메모리 ${memory})에서 측정: 각 ${seconds}초씩 ${runs}회 실행의 중앙값.`,
  versions: '버전',
  lower: '낮을수록 좋음',
  higher: '높을수록 좋음',
  failed: '실패',
  best: '최고',
  cases: {
    counter: {
      title: '카운터',
      body: '숫자 하나와 버튼 두 개를 열어 둔 유휴 상태: 앱이 얼마나 빨리 열리고 유지 비용이 얼마인지 측정합니다.',
    },
    quotes: {
      title: '실시간 시세',
      body: '40개 행의 시세표에서 매 디스플레이 프레임마다 10개 행(각 텍스트 4개)을 갱신합니다.',
    },
    list: {
      title: '스크롤 목록',
      body: '점 하나와 텍스트 두 개로 이루어진 10,000개 행을 매 프레임 한 행씩 스크롤합니다.',
    },
  },
  metrics: {
    startup: '시작 시간',
    memory: '메모리',
    size: '바이너리 크기',
    cpu: 'CPU',
    fps: '초당 프레임',
  },
  app: '앱',
  compositor: '컴포지터',
  cpuNote: '단일 코어 대비 백분율입니다. 네이티브 뷰는 그리기를 시스템 컴포지터에 맡기는 반면 GPU 렌더러는 자체 프로세스에서 그리므로, 각 막대에 컴포지터 CPU를 앱에 더해 표시합니다.',
  fpsNote: '업데이트는 디스플레이를 따릅니다. 따라가는 앱은 화면 주사율을 유지합니다. Fyne은 초당 60프레임을 그리고, 시스템 WebView 애니메이션도 디스플레이와 관계없이 60프레임입니다.',
  features: {
    title: '각 프레임워크의 실행 환경.',
    yes: '지원',
    no: '미지원',
  },
  method: {
    title: '측정 방법.',
    items: [
      '각 앱은 첫 프레임에 한 줄을 출력하고, 이후 매초 갱신한 프레임 수를 출력합니다. 러너는 시작부터 첫 줄까지의 시간을 재고 시스템에서 프로세스의 CPU 시간과 메모리를 읽습니다.',
      '시작 시간은 바이너리 실행부터 첫 프레임까지입니다. 메모리는 실행 종료 시점에 읽습니다(macOS: physical footprint, Linux: PSS, Windows: working set). macOS에서는 시스템 WebView의 WebKit 프로세스도 이를 시작한 앱에 포함합니다.',
      'CPU는 3초 안정화 후 10초 동안 앱을 번갈아 실행하며 측정합니다. 컴포지터는 macOS의 WindowServer, Windows의 DWM, Linux의 Wayland 컴포지터 및 Xwayland이며 앱을 열지 않았을 때의 사용량을 뺍니다.',
      '릴리스 빌드: go build -ldflags "-s -w" 및 심볼이 제거된 cargo build --release. GPUI Kit과 GPUI Fast는 동일한 소스를 각각의 크레이트로 빌드했습니다. Fyne은 -tags release, Wails는 -tags production으로 빌드해 fyne package -release, wails3 build와 같습니다.',
    ],
    source: '테스트 앱과 러너',
  },
  fair: {
    title: '공정하지 않은 부분이 있나요?',
    body: '이 앱들은 NSGo 작성자가 구현했습니다. 해당 프레임워크의 관용적 방식이 아니거나 구버전을 사용했거나 측정이 한쪽에 유리하다면 이슈로 알려 주세요. 수정 후 다시 측정하겠습니다.',
    action: '이슈로 제보하기',
  },
};

const es: BenchmarkCopy = {
  title: 'Benchmarks',
  description: 'NSGo frente a MyGo, Fyne, Wails, NSH, GPUI Kit y GPUI Fast: las mismas tres aplicaciones medidas en macOS, Windows y Linux, y dónde corre cada framework.',
  heading: 'Las mismas apps, siete frameworks.',
  lead: 'Tres pequeñas aplicaciones, escritas para cada framework tal como está pensado usarse, compiladas en modo release y medidas una tras otra en la misma máquina.',
  desktops: 'Escritorio',
  missing: 'Aún no medido en este sistema de escritorio.',
  machine: (date, cpu, memory, runs, seconds) => `Medido el ${date} en ${cpu} con ${memory} de memoria: mediana de ${runs} ejecuciones de ${seconds} s cada una.`,
  versions: 'Versiones',
  lower: 'Menor es mejor',
  higher: 'Mayor es mejor',
  failed: 'Error',
  best: 'Mejor',
  cases: {
    counter: {
      title: 'Contador',
      body: 'Un número y dos botones en reposo: qué tan rápido abre una app y cuánto cuesta mantenerla abierta.',
    },
    quotes: {
      title: 'Cotizaciones en vivo',
      body: 'Una tabla de 40 cotizaciones donde diez cambian en cada fotograma de pantalla, con cuatro textos cada una.',
    },
    list: {
      title: 'Lista con desplazamiento',
      body: '10.000 filas con un punto y dos textos, desplazadas una fila más en cada fotograma.',
    },
  },
  metrics: {
    startup: 'Arranque',
    memory: 'Memoria',
    size: 'Tamaño del binario',
    cpu: 'CPU',
    fps: 'Fotogramas por segundo',
  },
  app: 'App',
  compositor: 'Compositor',
  cpuNote: 'Porcentaje de un núcleo. Las vistas nativas delegan el dibujo al compositor del sistema, mientras un renderizador GPU dibuja en su propio proceso, por lo que cada barra suma la CPU del compositor a la de la app.',
  fpsNote: 'Las actualizaciones siguen a la pantalla: una app que mantiene el ritmo iguala su tasa de refresco. Fyne dibuja 60 fotogramas por segundo y la webview del sistema anima a 60, sea cual sea la pantalla.',
  features: {
    title: 'Dónde funciona cada uno.',
    yes: 'Sí',
    no: 'No',
  },
  method: {
    title: 'Cómo se mide.',
    items: [
      'Cada app imprime una línea en su primer fotograma y luego cada segundo cuántos fotogramas actualizó. El runner mide desde el inicio hasta la primera línea y lee del sistema el tiempo de CPU y memoria del proceso.',
      'El arranque va desde lanzar el binario hasta el primer fotograma. La memoria se lee al final: physical footprint en macOS, PSS en Linux y working set en Windows. En macOS, los procesos de WebKit de una webview del sistema cuentan con la app que los inició.',
      'La CPU se mide durante 10 segundos tras 3 de estabilización, turnando las apps. El compositor es WindowServer en macOS, DWM en Windows y el compositor Wayland más Xwayland en Linux, restando su uso sin apps abiertas.',
      'Compilaciones release: go build -ldflags "-s -w" y cargo build --release sin símbolos. GPUI Kit y GPUI Fast comparten el mismo código, compilado contra GPUI y contra GPUI Fast. Fyne se compila con -tags release y Wails con -tags production, como hacen fyne package -release y wails3 build.',
    ],
    source: 'Las apps y el runner',
  },
  fair: {
    title: '¿Algo injusto?',
    body: 'Estas apps fueron escritas por los autores de NSGo. Si alguna no sigue el estilo propio de su framework, usa una versión antigua o la medición favorece a alguno, abre un issue indicando qué cambiar y volveremos a medir.',
    action: 'Informar en un issue',
  },
};

const fr: BenchmarkCopy = {
  title: 'Benchmarks',
  description: 'NSGo face à MyGo, Fyne, Wails, NSH, GPUI Kit et GPUI Fast : les trois mêmes applications mesurées sur macOS, Windows et Linux, et les plateformes de chaque framework.',
  heading: 'Les mêmes apps, sept frameworks.',
  lead: 'Trois petites applications, écrites pour chaque framework selon sa façon idiomatique, compilées en release et mesurées l’une après l’autre sur la même machine.',
  desktops: 'Bureau',
  missing: 'Pas encore mesuré sur ce système de bureau.',
  machine: (date, cpu, memory, runs, seconds) => `Mesuré le ${date} sur ${cpu} avec ${memory} de mémoire : médiane de ${runs} exécutions de ${seconds} s chacune.`,
  versions: 'Versions',
  lower: 'Plus bas est mieux',
  higher: 'Plus haut est mieux',
  failed: 'Échec',
  best: 'Meilleur',
  cases: {
    counter: {
      title: 'Compteur',
      body: 'Un nombre et deux boutons au repos : à quelle vitesse l’app s’ouvre et ce qu’elle coûte ouverte.',
    },
    quotes: {
      title: 'Cours en continu',
      body: 'Un tableau de 40 cotations dont dix changent à chaque image d’écran, quatre textes chacune.',
    },
    list: {
      title: 'Liste déroulante',
      body: '10 000 lignes d’un point et deux textes, défilées d’une ligne à chaque image.',
    },
  },
  metrics: {
    startup: 'Démarrage',
    memory: 'Mémoire',
    size: 'Taille du binaire',
    cpu: 'CPU',
    fps: 'Images par seconde',
  },
  app: 'App',
  compositor: 'Compositeur',
  cpuNote: 'Pourcentage d’un cœur. Les vues natives délèguent le dessin au compositeur système, alors qu’un moteur GPU dessine dans son propre processus ; chaque barre ajoute donc le CPU du compositeur à celui de l’app.',
  fpsNote: 'Les mises à jour suivent l’écran : une app qui tient la cadence reste à la fréquence de rafraîchissement. Fyne dessine 60 images par seconde et la webview du système anime à 60, quel que soit l’écran.',
  features: {
    title: 'Où chacun fonctionne.',
    yes: 'Oui',
    no: 'Non',
  },
  method: {
    title: 'Méthode de mesure.',
    items: [
      'Chaque app affiche une ligne à sa première image, puis chaque seconde le nombre d’images mises à jour. Le runner chronomètre jusqu’à la première ligne et lit le temps CPU et la mémoire auprès du système.',
      'Le démarrage va du lancement du binaire à la première image. La mémoire est lue en fin d’exécution : empreinte physique sur macOS, PSS sur Linux, working set sur Windows. Sur macOS, les processus WebKit d’une webview du système comptent avec l’app qui les a lancés.',
      'Le CPU est mesuré pendant 10 s après 3 s de stabilisation, les apps tournant à tour de rôle. Le compositeur est WindowServer sur macOS, DWM sur Windows et le compositeur Wayland + Xwayland sur Linux, diminué de sa consommation à vide.',
      'Compilations release : go build -ldflags "-s -w" et cargo build --release sans symboles. GPUI Kit et GPUI Fast partagent une même source, compilée contre GPUI et contre GPUI Fast. Fyne est compilé avec -tags release et Wails avec -tags production, comme le font fyne package -release et wails3 build.',
    ],
    source: 'Les apps et le runner',
  },
  fair: {
    title: 'Quelque chose d’injuste ?',
    body: 'Ces apps ont été écrites par les auteurs de NSGo. Si l’une ne suit pas les usages de son framework, utilise une ancienne version ou si une mesure en favorise un, ouvrez une issue avec les changements souhaités et nous remesurerons.',
    action: 'Signaler dans une issue',
  },
};

const de: BenchmarkCopy = {
  title: 'Benchmarks',
  description: 'NSGo im Vergleich zu MyGo, Fyne, Wails, NSH, GPUI Kit und GPUI Fast: dieselben drei Apps auf macOS, Windows und Linux gemessen, plus Plattformübersicht.',
  heading: 'Dieselben Apps, sieben Frameworks.',
  lead: 'Drei kleine Apps, jeweils idiomatisch für ihr Framework geschrieben, als Release gebaut und nacheinander auf derselben Maschine gemessen.',
  desktops: 'Desktop',
  missing: 'Auf diesem Desktop noch nicht gemessen.',
  machine: (date, cpu, memory, runs, seconds) => `Gemessen am ${date} auf ${cpu} mit ${memory} Speicher: Median aus ${runs} Läufen à ${seconds} s.`,
  versions: 'Versionen',
  lower: 'Niedriger ist besser',
  higher: 'Höher ist besser',
  failed: 'Fehlgeschlagen',
  best: 'Bestwert',
  cases: {
    counter: {
      title: 'Zähler',
      body: 'Eine Zahl und zwei Buttons im Leerlauf: wie schnell eine App startet und was ihr Offenhalten kostet.',
    },
    quotes: {
      title: 'Live-Kurse',
      body: 'Eine Tabelle mit 40 Kursen, von denen sich zehn in jedem Display-Frame ändern – je vier Texte.',
    },
    list: {
      title: 'Scroll-Liste',
      body: '10.000 Zeilen aus einem Punkt und zwei Texten, in jedem Display-Frame um eine Zeile weiter gescrollt.',
    },
  },
  metrics: {
    startup: 'Startzeit',
    memory: 'Speicher',
    size: 'Binärgröße',
    cpu: 'CPU',
    fps: 'Bilder pro Sekunde',
  },
  app: 'App',
  compositor: 'Compositor',
  cpuNote: 'Prozent eines Kerns. Native Views überlassen das Zeichnen dem System-Compositor, während ein GPU-Renderer im eigenen Prozess zeichnet; jeder Balken addiert daher die Compositor-CPU zur App-CPU.',
  fpsNote: 'Updates folgen dem Display: eine App, die mithält, bleibt bei der Bildwiederholrate. Fyne zeichnet 60 Bilder pro Sekunde, und die System-Webview animiert mit 60, unabhängig vom Display.',
  features: {
    title: 'Wo jedes Framework läuft.',
    yes: 'Ja',
    no: 'Nein',
  },
  method: {
    title: 'Wie gemessen wird.',
    items: [
      'Jede App gibt beim ersten Frame eine Zeile aus und danach jede Sekunde die aktualisierten Frames. Der Runner misst die Zeit bis zur ersten Zeile und liest CPU-Zeit sowie Speicher vom System.',
      'Die Startzeit reicht vom Start der Binärdatei bis zum ersten Frame. Der Speicher wird am Laufende gelesen: Physical Footprint unter macOS, PSS unter Linux, Working Set unter Windows. Unter macOS zählen die WebKit-Prozesse einer System-Webview zur App, die sie gestartet hat.',
      'Die CPU wird nach 3 s Einschwingzeit 10 s lang im Wechsel gemessen. Der Compositor ist WindowServer (macOS), DWM (Windows) bzw. Wayland-Compositor + Xwayland (Linux) abzüglich des Leerlaufs.',
      'Release-Builds: go build -ldflags "-s -w" sowie cargo build --release ohne Symbole. GPUI Kit und GPUI Fast teilen denselben Quellcode gegen Upstream-GPUI bzw. GPUI Fast. Fyne wird mit -tags release und Wails mit -tags production gebaut, wie es fyne package -release und wails3 build tun.',
    ],
    source: 'Apps und Messprogramm',
  },
  fair: {
    title: 'Etwas nicht fair?',
    body: 'Diese Apps wurden von den NSGo-Autoren geschrieben. Falls eine nicht idiomatisch ist, eine ältere Version nutzt oder eine Messung ein Framework bevorteilt, eröffne ein Issue und wir messen erneut.',
    action: 'In einem Issue melden',
  },
};

const pt: BenchmarkCopy = {
  title: 'Benchmarks',
  description: 'NSGo contra MyGo, Fyne, Wails, NSH, GPUI Kit e GPUI Fast: os mesmos três apps medidos em macOS, Windows e Linux, e onde cada framework roda.',
  heading: 'Os mesmos apps, sete frameworks.',
  lead: 'Três pequenos apps, escritos para cada framework do jeito recomendado, compilados em modo release e medidos um após o outro na mesma máquina.',
  desktops: 'Desktop',
  missing: 'Ainda não medido neste desktop.',
  machine: (date, cpu, memory, runs, seconds) => `Medido em ${date} em ${cpu} com ${memory} de memória: mediana de ${runs} execuções de ${seconds} s cada.`,
  versions: 'Versões',
  lower: 'Menor é melhor',
  higher: 'Maior é melhor',
  failed: 'Falhou',
  best: 'Melhor',
  cases: {
    counter: {
      title: 'Contador',
      body: 'Uma contagem e dois botões em repouso: quão rápido o app abre e quanto custa mantê-lo aberto.',
    },
    quotes: {
      title: 'Cotações em tempo real',
      body: 'Uma tabela de 40 cotações, dez mudando a cada quadro da tela, com quatro textos cada.',
    },
    list: {
      title: 'Lista rolável',
      body: '10.000 linhas com um ponto e dois textos, roladas uma linha a mais a cada quadro.',
    },
  },
  metrics: {
    startup: 'Inicialização',
    memory: 'Memória',
    size: 'Tamanho do binário',
    cpu: 'CPU',
    fps: 'Quadros por segundo',
  },
  app: 'App',
  compositor: 'Compositor',
  cpuNote: 'Porcentagem de um núcleo. Views nativas delegam o desenho ao compositor do sistema, enquanto um renderizador GPU desenha no próprio processo, então cada barra soma a CPU do compositor à do app.',
  fpsNote: 'As atualizações acompanham a tela: um app que acompanha mantém a taxa de atualização. Fyne desenha 60 quadros por segundo e a webview do sistema anima a 60, seja qual for a tela.',
  features: {
    title: 'Onde cada um roda.',
    yes: 'Sim',
    no: 'Não',
  },
  method: {
    title: 'Como é medido.',
    items: [
      'Cada app imprime uma linha no primeiro quadro e depois a cada segundo quantos quadros atualizou. O runner mede o tempo até a primeira linha e lê o tempo de CPU e a memória do sistema.',
      'A inicialização vai do lançamento do binário ao primeiro quadro. A memória é lida ao final: physical footprint no macOS, PSS no Linux e working set no Windows. No macOS, os processos do WebKit de uma webview do sistema contam com o app que os iniciou.',
      'A CPU é medida por 10 s após 3 s de estabilização, alternando os apps. O compositor é WindowServer no macOS, DWM no Windows e compositor Wayland + Xwayland no Linux, descontando o consumo ocioso.',
      'Builds de release: go build -ldflags "-s -w" e cargo build --release sem símbolos. GPUI Kit e GPUI Fast compartilham o mesmo código, compilado contra GPUI e contra GPUI Fast. Fyne é compilado com -tags release e Wails com -tags production, como fazem fyne package -release e wails3 build.',
    ],
    source: 'Os apps e o runner',
  },
  fair: {
    title: 'Algo injusto?',
    body: 'Estes apps foram escritos pelos autores do NSGo. Se algum não estiver no estilo idiomático do seu framework, usar versão antiga ou a medição favorecer algum, abra uma issue com o que ajustar e mediremos novamente.',
    action: 'Relatar em uma issue',
  },
};

const ru: BenchmarkCopy = {
  title: 'Бенчмарки',
  description: 'Сравнение NSGo с MyGo, Fyne, Wails, NSH, GPUI Kit и GPUI Fast: три одинаковых приложения на macOS, Windows и Linux, а также список поддерживаемых платформ.',
  heading: 'Одни и те же приложения, семь фреймворков.',
  lead: 'Три небольших приложения, написанные идиоматично для каждого фреймворка, собранные в release-режиме и измеренные подряд на одной машине.',
  desktops: 'Десктоп',
  missing: 'На этой ОС ещё не измерялось.',
  machine: (date, cpu, memory, runs, seconds) => `Измерено ${date} на ${cpu} (${memory} ОЗУ): медиана ${runs} запусков по ${seconds} с.`,
  versions: 'Версии',
  lower: 'Меньше — лучше',
  higher: 'Больше — лучше',
  failed: 'Ошибка',
  best: 'Лучший',
  cases: {
    counter: {
      title: 'Счётчик',
      body: 'Число и две кнопки в простое: как быстро открывается приложение и сколько ресурсов потребляет в фоне.',
    },
    quotes: {
      title: 'Потоковые котировки',
      body: 'Таблица из 40 котировок, десять из которых обновляются каждый кадр дисплея (по четыре текста).',
    },
    list: {
      title: 'Прокручиваемый список',
      body: '10 000 строк с точкой и двумя текстами, прокручиваемых на строку дальше каждый кадр.',
    },
  },
  metrics: {
    startup: 'Запуск',
    memory: 'Память',
    size: 'Размер бинарника',
    cpu: 'CPU',
    fps: 'Кадров в секунду',
  },
  app: 'Приложение',
  compositor: 'Композитор',
  cpuNote: 'Процент одного ядра. Нативные представления отдают отрисовку системному композитору, тогда как GPU-рендерер рисует в своём процессе, поэтому на графиках CPU композитора прибавлен к CPU приложения.',
  fpsNote: 'Обновления синхронизированы с экраном: успевающее приложение держит частоту обновления дисплея. Fyne рисует 60 кадров в секунду, а системный WebView анимирует с частотой 60 независимо от экрана.',
  features: {
    title: 'Где работает каждый фреймворк.',
    yes: 'Да',
    no: 'Нет',
  },
  method: {
    title: 'Методика измерений.',
    items: [
      'Каждое приложение выводит строку на первом кадре, затем каждую секунду число обновлённых кадров. Раннер замеряет время до первой строки и считывает CPU и память процесса из системы.',
      'Запуск измеряется от старта бинарника до первого кадра. Память фиксируется в конце прогона: physical footprint на macOS, PSS на Linux, working set на Windows. В macOS процессы WebKit системного WebView учитываются вместе с запустившим их приложением.',
      'CPU измеряется 10 секунд после 3 секунд прогрева по очереди для каждого приложения. Композитор — WindowServer на macOS, DWM на Windows и Wayland + Xwayland на Linux за вычетом простоя.',
      'Release-сборки: go build -ldflags "-s -w" и cargo build --release без отладочных символов. GPUI Kit и GPUI Fast используют общий исходник против upstream GPUI и GPUI Fast. Fyne собирается с -tags release, Wails — с -tags production, как это делают fyne package -release и wails3 build.',
    ],
    source: 'Приложения и раннер',
  },
  fair: {
    title: 'Заметили неточность?',
    body: 'Эти приложения написаны авторами NSGo. Если какое-то написано неидиоматично, использует старую версию или замер даёт кому-то преимущество, откройте issue — мы исправим и перемерим.',
    action: 'Сообщить в issue',
  },
};

const it: BenchmarkCopy = {
  title: 'Benchmark',
  description: 'NSGo a confronto con MyGo, Fyne, Wails, NSH, GPUI Kit e GPUI Fast: le stesse tre app misurate su macOS, Windows e Linux, e dove gira ogni framework.',
  heading: 'Le stesse app, sette framework.',
  lead: 'Tre piccole app, scritte per ciascun framework nel modo idiomatico, compilate in release e misurate una dopo l’altra sulla stessa macchina.',
  desktops: 'Desktop',
  missing: 'Non ancora misurato su questo desktop.',
  machine: (date, cpu, memory, runs, seconds) => `Misurato il ${date} su ${cpu} con ${memory} di memoria: mediana di ${runs} esecuzioni da ${seconds} s ciascuna.`,
  versions: 'Versioni',
  lower: 'Più basso è meglio',
  higher: 'Più alto è meglio',
  failed: 'Fallito',
  best: 'Migliore',
  cases: {
    counter: {
      title: 'Contatore',
      body: 'Un numero e due pulsanti a riposo: quanto velocemente si apre un’app e quanto costa tenerla aperta.',
    },
    quotes: {
      title: 'Quotazioni in tempo reale',
      body: 'Una tabella di 40 quotazioni, dieci delle quali cambiano a ogni fotogramma, con quattro testi ciascuna.',
    },
    list: {
      title: 'Elenco scorrevole',
      body: '10.000 righe con un punto e due testi, fatte scorrere di una riga a ogni fotogramma.',
    },
  },
  metrics: {
    startup: 'Avvio',
    memory: 'Memoria',
    size: 'Dimensione binario',
    cpu: 'CPU',
    fps: 'Fotogrammi al secondo',
  },
  app: 'App',
  compositor: 'Compositore',
  cpuNote: 'Percentuale di un core. Le viste native affidano il disegno al compositore di sistema, mentre un renderer GPU disegna nel proprio processo, quindi ogni barra somma la CPU del compositore a quella dell’app.',
  fpsNote: 'Gli aggiornamenti seguono il display: un’app che tiene il passo resta alla frequenza di aggiornamento. Fyne disegna 60 fotogrammi al secondo e la webview di sistema anima a 60, qualunque sia il display.',
  features: {
    title: 'Dove gira ciascuno.',
    yes: 'Sì',
    no: 'No',
  },
  method: {
    title: 'Come viene misurato.',
    items: [
      'Ogni app stampa una riga al primo fotogramma, poi ogni secondo quanti fotogrammi ha aggiornato. Il runner misura fino alla prima riga e legge tempo CPU e memoria dal sistema.',
      'L’avvio va dal lancio del binario al primo fotogramma. La memoria è letta a fine esecuzione: physical footprint su macOS, PSS su Linux, working set su Windows. Su macOS i processi WebKit di una webview di sistema contano con l’app che li ha avviati.',
      'La CPU è misurata per 10 s dopo 3 s di assestamento, alternando le app. Il compositore è WindowServer su macOS, DWM su Windows e compositore Wayland + Xwayland su Linux, al netto del consumo a riposo.',
      'Build release: go build -ldflags "-s -w" e cargo build --release senza simboli. GPUI Kit e GPUI Fast condividono la stessa sorgente, compilata contro GPUI e contro GPUI Fast. Fyne è compilato con -tags release e Wails con -tags production, come fanno fyne package -release e wails3 build.',
    ],
    source: 'Le app e il runner',
  },
  fair: {
    title: 'Qualcosa di poco equo?',
    body: 'Queste app sono state scritte dagli autori di NSGo. Se una non segue lo stile del proprio framework, usa una versione vecchia o una misura ne favorisce uno, apri una issue con le modifiche e ripeteremo la misura.',
    action: 'Segnala in una issue',
  },
};

const vi: BenchmarkCopy = {
  title: 'Hiệu năng',
  description: 'So sánh NSGo với MyGo, Fyne, Wails, NSH, GPUI Kit và GPUI Fast: ba ứng dụng giống nhau đo trên macOS, Windows và Linux, cùng các nền tảng mỗi framework hỗ trợ.',
  heading: 'Cùng ứng dụng, bảy framework.',
  lead: 'Ba ứng dụng nhỏ, được viết đúng cách khuyến nghị của từng framework, biên dịch bản phát hành và đo lần lượt trên cùng một máy.',
  desktops: 'Máy tính',
  missing: 'Chưa đo trên hệ điều hành này.',
  machine: (date, cpu, memory, runs, seconds) => `Đo ngày ${date} trên ${cpu} với ${memory} bộ nhớ: trung vị của ${runs} lần chạy, mỗi lần ${seconds} giây.`,
  versions: 'Phiên bản',
  lower: 'Thấp hơn tốt hơn',
  higher: 'Cao hơn tốt hơn',
  failed: 'Thất bại',
  best: 'Tốt nhất',
  cases: {
    counter: {
      title: 'Bộ đếm',
      body: 'Một con số và hai nút ở trạng thái nghỉ: tốc độ mở ứng dụng và chi phí duy trì khi đang mở.',
    },
    quotes: {
      title: 'Bảng giá trực tiếp',
      body: 'Bảng 40 mã giá, mười mã thay đổi ở mỗi khung hình hiển thị, mỗi mã gồm bốn đoạn chữ.',
    },
    list: {
      title: 'Danh sách cuộn',
      body: '10.000 dòng gồm một dấu chấm và hai đoạn chữ, cuộn thêm một dòng ở mỗi khung hình.',
    },
  },
  metrics: {
    startup: 'Khởi động',
    memory: 'Bộ nhớ',
    size: 'Kích thước tệp nhị phân',
    cpu: 'CPU',
    fps: 'Khung hình mỗi giây',
  },
  app: 'Ứng dụng',
  compositor: 'Bộ tổng hợp',
  cpuNote: 'Phần trăm của một nhân CPU. Giao diện gốc giao phần vẽ cho bộ tổng hợp hệ thống trong khi bộ dựng GPU vẽ ngay trong tiến trình riêng, nên mỗi thanh cộng thêm CPU của bộ tổng hợp vào ứng dụng.',
  fpsNote: 'Cập nhật theo màn hình: ứng dụng theo kịp sẽ duy trì đúng tần số quét. Fyne vẽ 60 khung hình mỗi giây và WebView của hệ thống chạy hoạt ảnh ở 60, bất kể màn hình.',
  features: {
    title: 'Nơi mỗi framework chạy được.',
    yes: 'Có',
    no: 'Không',
  },
  method: {
    title: 'Cách đo lường.',
    items: [
      'Mỗi ứng dụng in một dòng ở khung hình đầu tiên, rồi mỗi giây in số khung hình đã cập nhật. Trình đo tính thời gian tới dòng đầu tiên và đọc CPU cùng bộ nhớ tiến trình từ hệ thống.',
      'Thời gian khởi động tính từ lúc chạy tệp nhị phân tới khung hình đầu tiên. Bộ nhớ đo cuối mỗi lượt: physical footprint trên macOS, PSS trên Linux, working set trên Windows. Trên macOS, các tiến trình WebKit của WebView hệ thống được tính cùng ứng dụng đã khởi chạy chúng.',
      'CPU đo trong 10 giây sau 3 giây ổn định, các ứng dụng chạy luân phiên. Bộ tổng hợp là WindowServer trên macOS, DWM trên Windows và Wayland + Xwayland trên Linux, trừ đi mức tiêu thụ lúc nghỉ.',
      'Bản phát hành: go build -ldflags "-s -w" và cargo build --release đã lược bỏ ký hiệu. GPUI Kit và GPUI Fast dùng chung mã nguồn, biên dịch với GPUI gốc và GPUI Fast. Fyne được build với -tags release và Wails với -tags production, giống fyne package -release và wails3 build.',
    ],
    source: 'Các ứng dụng và trình đo',
  },
  fair: {
    title: 'Thấy điểm chưa công bằng?',
    body: 'Các ứng dụng này do nhóm tác giả NSGo viết. Nếu một bản chưa viết đúng phong cách framework, dùng bản cũ hoặc cách đo thiên vị một framework nào, hãy mở issue để chúng tôi sửa và đo lại.',
    action: 'Báo cáo qua issue',
  },
};

const id: BenchmarkCopy = {
  title: 'Tolok ukur',
  description: 'Perbandingan NSGo melawan MyGo, Fyne, Wails, NSH, GPUI Kit, dan GPUI Fast: tiga aplikasi yang sama diukur di macOS, Windows, dan Linux, serta platform masing-masing framework.',
  heading: 'Aplikasi yang sama, tujuh framework.',
  lead: 'Tiga aplikasi kecil yang ditulis secara idiomatik untuk tiap framework, dibangun dalam mode rilis, lalu diukur bergantian di mesin yang sama.',
  desktops: 'Desktop',
  missing: 'Belum diukur di desktop ini.',
  machine: (date, cpu, memory, runs, seconds) => `Diukur pada ${date} di ${cpu} dengan memori ${memory}: median dari ${runs} kali jalan, masing-masing ${seconds} detik.`,
  versions: 'Versi',
  lower: 'Lebih rendah lebih baik',
  higher: 'Lebih tinggi lebih baik',
  failed: 'Gagal',
  best: 'Terbaik',
  cases: {
    counter: {
      title: 'Penghitung',
      body: 'Satu angka dan dua tombol dalam keadaan diam: seberapa cepat aplikasi terbuka dan berapa biaya mempertahankannya.',
    },
    quotes: {
      title: 'Kuotasi streaming',
      body: 'Tabel berisi 40 kuotasi, sepuluh di antaranya berubah setiap frame layar dengan empat teks masing-masing.',
    },
    list: {
      title: 'Daftar gulir',
      body: '10.000 baris berisi satu titik dan dua teks, digulir satu baris lebih jauh di setiap frame.',
    },
  },
  metrics: {
    startup: 'Waktu mulai',
    memory: 'Memori',
    size: 'Ukuran biner',
    cpu: 'CPU',
    fps: 'Frame per detik',
  },
  app: 'Aplikasi',
  compositor: 'Kompositor',
  cpuNote: 'Persentase dari satu inti CPU. Tampilan native menyerahkan penggambaran ke kompositor sistem, sedangkan perender GPU menggambar di prosesnya sendiri, sehingga tiap batang menjumlahkan CPU kompositor ke CPU aplikasi.',
  fpsNote: 'Pembaruan mengikuti layar: aplikasi yang mampu mengejar tetap berada pada refresh rate layar. Fyne menggambar 60 bingkai per detik dan webview sistem beranimasi pada 60, apa pun layarnya.',
  features: {
    title: 'Di mana masing-masing berjalan.',
    yes: 'Ya',
    no: 'Tidak',
  },
  method: {
    title: 'Cara pengukuran.',
    items: [
      'Setiap aplikasi mencetak satu baris pada frame pertama, lalu setiap detik jumlah frame yang diperbarui. Runner mencatat waktu hingga baris pertama dan membaca CPU serta memori proses dari sistem.',
      'Waktu mulai diukur dari peluncuran biner hingga frame pertama. Memori dibaca di akhir pengujian: physical footprint di macOS, PSS di Linux, dan working set di Windows. Di macOS, proses WebKit dari webview sistem dihitung bersama aplikasi yang memulainya.',
      'CPU diukur selama 10 detik setelah 3 detik stabilisasi secara bergantian. Kompositor adalah WindowServer di macOS, DWM di Windows, serta kompositor Wayland + Xwayland di Linux dikurangi penggunaan saat kosong.',
      'Build rilis: go build -ldflags "-s -w" serta cargo build --release tanpa simbol. GPUI Kit dan GPUI Fast berbagi kode sumber yang sama, dibangun terhadap GPUI hulu dan GPUI Fast. Fyne dibangun dengan -tags release dan Wails dengan -tags production, seperti fyne package -release dan wails3 build.',
    ],
    source: 'Aplikasi dan runner',
  },
  fair: {
    title: 'Ada yang kurang adil?',
    body: 'Aplikasi ini ditulis oleh pembuat NSGo. Jika ada yang belum ditulis secara idiomatik, memakai versi lama, atau pengukuran menguntungkan satu pihak, buka issue agar kami perbaiki dan ukur ulang.',
    action: 'Laporkan di issue',
  },
};

const tr: BenchmarkCopy = {
  title: 'Kıyaslamalar',
  description: 'NSGo’nun MyGo, Fyne, Wails, NSH, GPUI Kit ve GPUI Fast ile karşılaştırması: aynı üç uygulamanın macOS, Windows ve Linux ölçümleri ve her çerçevenin çalıştığı platformlar.',
  heading: 'Aynı uygulamalar, yedi çerçeve.',
  lead: 'Her çerçeve için önerilen biçimde yazılmış üç küçük uygulama; sürüm modunda derlendi ve aynı makinede art arda ölçüldü.',
  desktops: 'Masaüstü',
  missing: 'Bu masaüstünde henüz ölçülmedi.',
  machine: (date, cpu, memory, runs, seconds) => `${date} tarihinde ${cpu} (${memory} bellek) üzerinde ölçüldü: her biri ${seconds} sn süren ${runs} çalıştırmanın medyanı.`,
  versions: 'Sürümler',
  lower: 'Düşük olan daha iyi',
  higher: 'Yüksek olan daha iyi',
  failed: 'Başarısız',
  best: 'En iyi',
  cases: {
    counter: {
      title: 'Sayaç',
      body: 'Boşta bekleyen bir sayı ve iki düğme: uygulamanın ne kadar hızlı açıldığı ve açık kalma maliyeti.',
    },
    quotes: {
      title: 'Canlı fiyat akışı',
      body: 'Her ekran karesinde onu değişen, her biri dört metin içeren 40 satırlık fiyat tablosu.',
    },
    list: {
      title: 'Kaydırılan liste',
      body: 'Bir nokta ve iki metinden oluşan 10.000 satır, her karede bir satır aşağı kaydırılır.',
    },
  },
  metrics: {
    startup: 'Açılış',
    memory: 'Bellek',
    size: 'İkili dosya boyutu',
    cpu: 'CPU',
    fps: 'Saniye başına kare',
  },
  app: 'Uygulama',
  compositor: 'Birleştirici',
  cpuNote: 'Tek çekirdek yüzdesi. Yerel görünümler çizimi sistem birleştiricisine bırakırken GPU çiziciler kendi sürecinde çizer; bu nedenle her çubuk birleştirici CPU’sunu uygulamaya ekler.',
  fpsNote: 'Güncellemeler ekranı izler: yetişebilen uygulama yenileme hızında kalır. Fyne saniyede 60 kare çizer ve sistem web görünümü ekrandan bağımsız olarak 60’ta canlandırır.',
  features: {
    title: 'Her biri nerede çalışır.',
    yes: 'Evet',
    no: 'Hayır',
  },
  method: {
    title: 'Nasıl ölçülüyor.',
    items: [
      'Her uygulama ilk karede bir satır, ardından her saniye kaç kare güncellediğini yazar. Çalıştırıcı ilk satıra kadar geçen süreyi ve sistemden CPU süresi ile belleği okur.',
      'Açılış süresi ikili dosyanın başlatılmasından ilk kareye kadardır. Bellek çalıştırma sonunda okunur: macOS’ta physical footprint, Linux’ta PSS, Windows’ta working set. macOS’ta sistem web görünümünün WebKit süreçleri, onları başlatan uygulamayla birlikte sayılır.',
      'CPU, 3 saniyelik oturma süresinin ardından 10 saniye boyunca sırayla ölçülür. Birleştirici macOS’ta WindowServer, Windows’ta DWM, Linux’ta Wayland birleştiricisi ve Xwayland’dir (boşta tüketim düşülerek).',
      'Sürüm derlemeleri: go build -ldflags "-s -w" ve sembolleri temizlenmiş cargo build --release. GPUI Kit ve GPUI Fast aynı kaynağı kendi paketleriyle derler. Fyne -tags release, Wails -tags production ile derlenir; fyne package -release ve wails3 build de böyle yapar.',
    ],
    source: 'Uygulamalar ve ölçüm aracı',
  },
  fair: {
    title: 'Adil olmayan bir şey mi var?',
    body: 'Bu uygulamalar NSGo yazarları tarafından yazıldı. Biri kendi çerçevesine uygun yazılmadıysa, eski bir sürüm kullanıyorsa veya ölçüm bir çerçeveyi kayırıyorsa issue açın, düzeltip yeniden ölçelim.',
    action: 'Issue ile bildir',
  },
};

export const benchmarkCopies: Record<SiteLang, BenchmarkCopy> = {
  en,
  zh,
  'zh-tw': zhTw,
  ja,
  ko,
  es,
  fr,
  de,
  pt,
  ru,
  it,
  vi,
  id,
  tr,
};
