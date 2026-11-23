// The Benchmarks page's words. Add English and Chinese together; Chinese
// headings and titles omit the trailing 。, body sentences keep it.

const en = {
  title: 'Benchmarks',
  description:
    'NSGo against MyGo, NSH, GPUI Kit and GPUI Fast: the same three apps measured on macOS, Windows and Linux, and where each framework runs.',
  heading: 'The same apps, five frameworks.',
  lead: 'Three small apps, written for each framework the way it is meant to be used, built as releases and measured one after another on the same machine.',
  desktops: 'Desktop',
  missing: 'Not measured on this desktop yet.',
  machine: (date: string, cpu: string, memory: string, runs: number, seconds: number) =>
    `Measured on ${date} on ${cpu} with ${memory} of memory: the median of ${runs} runs, ${seconds} s each.`,
  versions: 'Versions',
  lower: 'Lower is better',
  higher: 'Higher is better',
  failed: 'Failed',
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
  fpsNote: 'Updates follow the display: an app that keeps up stays at its refresh rate.',
  features: {
    title: 'Where each one runs.',
    yes: 'Yes',
    no: 'No',
  },
  method: {
    title: 'How it is measured.',
    items: [
      'Each app prints a line on its first frame, then every second how many frames it updated. The runner times the first line from the start and reads the process’s CPU time and memory from the system.',
      'Startup runs from launching the binary to the first frame. Memory is read at the end of a run: the physical footprint on macOS, the proportional set size on Linux, the working set on Windows.',
      'CPU is measured for 10 seconds after 3 to settle, the apps taking turns. The compositor is WindowServer on macOS, the Desktop Window Manager on Windows, and the Wayland compositor and Xwayland on Linux, less what it spends with none of the apps open.',
      'Release builds: go build -ldflags "-s -w", and cargo build --release with symbols stripped. GPUI Kit and GPUI Fast share one source, built against upstream GPUI and against GPUI Fast.',
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
  description: 'NSGo 与 MyGo、NSH、GPUI Kit、GPUI Fast 的对比：同样的三个应用在 macOS、Windows 和 Linux 上的测量结果，以及各框架支持的平台。',
  heading: '同样的应用，五个框架',
  lead: '三个小应用，按每个框架推荐的写法各实现一遍，以发布模式构建，在同一台机器上依次测量。',
  desktops: '桌面平台',
  missing: '还没有在这个平台上测量。',
  machine: (date: string, cpu: string, memory: string, runs: number, seconds: number) =>
    `${date} 测于 ${cpu}，内存 ${memory}：取 ${runs} 次运行的中位数，每次 ${seconds} 秒。`,
  versions: '版本',
  lower: '越低越好',
  higher: '越高越好',
  failed: '失败',
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
  cpuNote:
    '以单核为 100%。原生视图把绘制交给系统合成器，而 GPU 渲染在应用自己的进程里绘制，所以每条都把合成器的 CPU 加在应用的之上。',
  fpsNote: '更新跟随显示器：跟得上的应用保持在屏幕刷新率。',
  features: {
    title: '各自能跑在哪里',
    yes: '支持',
    no: '不支持',
  },
  method: {
    title: '测量方法',
    items: [
      '每个应用在第一帧打印一行，之后每秒打印更新了多少帧。测量程序从启动开始计时到第一行，并从系统读取进程的 CPU 时间和内存。',
      '启动时间是从启动二进制到第一帧。内存在每次运行结束时读取：macOS 为物理内存占用（footprint），Linux 为按比例分摊的常驻内存（PSS），Windows 为工作集。',
      'CPU 在稳定 3 秒后测量 10 秒，各应用轮流运行。合成器在 macOS 是 WindowServer，Windows 是桌面窗口管理器（DWM），Linux 是 Wayland 合成器和 Xwayland，并扣除未打开任何测试应用时它自身的占用。',
      '均为发布构建：go build -ldflags "-s -w"，以及去除符号的 cargo build --release。GPUI Kit 与 GPUI Fast 共用一份源码，分别基于上游 GPUI 和 GPUI Fast 构建。',
    ],
    source: '测试应用与测量程序',
  },
  fair: {
    title: '发现不公平的地方？',
    body: '这些应用由 NSGo 的作者编写。如果某个实现不符合它所在框架的推荐写法、用了旧版本，或者测量方式偏向了某个框架，欢迎在 Issue 中指出应该怎么改，我们会重新测量。',
    action: '在 Issue 中报告',
  },
};

export const benchmarkCopies = { en, zh };
