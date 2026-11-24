import type { Section } from 'nsgo-website/config';
import type { SiteLang } from '../locales';

interface SectionCopy {
  features: {
    eyebrow: string;
    title: string;
    items: [string, string][];
  };
  api: {
    eyebrow: string;
    title: string;
    text: string;
    items: [string, string][];
  };
  end: {
    title: string;
    text: string;
    getStarted: string;
    liveDemo: string;
  };
}

const FEATURE_ICONS = ['layers', 'activity', 'layout-dashboard', 'blocks', 'file-code', 'test-tube'] as const;
const API_ICONS = ['code', 'activity', 'puzzle'] as const;

const COPY: Record<SiteLang, SectionCopy> = {
  en: {
    features: {
      eyebrow: 'What you get',
      title: 'Everything an app needs, in one language.',
      items: [
        ['Truly native', 'Each platform draws with its own views, text, scrolling and accessibility.'],
        ['A reactive engine', 'Signals update only what reads them. Animation runs on the platform’s frame clock.'],
        ['Layout and styles you know', 'Flexbox and grid, CSS-like styles, Tailwind classes, light and dark themes, right-to-left languages.'],
        ['Components included', 'Unstyled in base, ready to use in ui: from buttons and menus to data tables, charts, a code editor and a terminal.'],
        ['Go or TSX', 'Components as Go values, or TSX with Tailwind classes, compiled ahead with no Node.'],
        ['Tests that act like users', 'Find a control by its text and press it, headless or in a real window.'],
      ],
    },
    api: {
      eyebrow: 'The API',
      title: 'A component is just Go.',
      text: 'Elements are structs, state is a signal, and a test presses the button the way a user would.',
      items: [
        ['No template language', 'A component is a function that returns elements.'],
        ['Fine-grained updates', 'Pressing changes the one text that reads the count.'],
        ['Embeds anywhere', 'Put an NSGo view inside an iOS, Android, desktop or web app.'],
      ],
    },
    end: {
      title: 'Start building with NSGo.',
      text: 'Follow the guide to install the tool and create your first app.',
      getStarted: 'Get started',
      liveDemo: 'Try the live demo',
    },
  },
  zh: {
    features: {
      eyebrow: '你能得到',
      title: '应用所需的一切，都在一门语言里',
      items: [
        ['真正的原生', '每个平台用自己的视图、文本、滚动和无障碍来绘制。'],
        ['响应式引擎', 'signal 只更新读取它的地方，动画运行在平台的帧时钟上。'],
        ['熟悉的布局与样式', 'flexbox 与 grid、类 CSS 样式、Tailwind 类名、浅色与深色主题、从右到左的语言。'],
        ['内置组件', 'base 里是无样式的，ui 里是开箱即用的：从按钮、菜单到数据表格、图表、代码编辑器和终端。'],
        ['Go 或 TSX', '用 Go 值写组件，或用 TSX 配合 Tailwind 类名，提前编译，不需要 Node。'],
        ['像用户一样测试', '按文本找到控件并按下它，无窗口或在真实窗口中运行。'],
      ],
    },
    api: {
      eyebrow: 'API',
      title: '组件就是 Go',
      text: '元素是结构体，状态是 signal，测试像用户一样按下按钮。',
      items: [
        ['没有模板语言', '组件就是返回元素的函数。'],
        ['细粒度更新', '按下按钮，只有读取计数的那段文本会变。'],
        ['随处嵌入', '把 NSGo 视图放进 iOS、Android、桌面或 Web 应用。'],
      ],
    },
    end: {
      title: '开始用 NSGo 构建',
      text: '跟着指南安装工具，创建你的第一个应用。',
      getStarted: '快速开始',
      liveDemo: '在线体验',
    },
  },
  'zh-tw': {
    features: {
      eyebrow: '你將獲得',
      title: '應用所需的一切，都在同一門語言裡',
      items: [
        ['真正的原生', '每個平台皆使用各自的原生視圖、文字、捲動與無障礙功能來繪製。'],
        ['響應式引擎', 'signal 只更新讀取它的位置，動畫跟隨平台的影格時鐘執行。'],
        ['熟悉的版面與樣式', 'flexbox 與 grid、類 CSS 樣式、Tailwind 類別、淺色與深色主題、由右至左語言。'],
        ['內建元件', 'base 提供無樣式基礎，ui 開箱即用：從按鈕、選單到資料表、圖表、程式碼編輯器與終端機。'],
        ['Go 或 TSX', '以 Go 值編寫元件，或搭配 TSX 與 Tailwind 類別預先編譯，無需 Node。'],
        ['像使用者一樣測試', '依文字尋找控制項並點擊它，支援無頭模式或真實視窗執行。'],
      ],
    },
    api: {
      eyebrow: 'API',
      title: '元件就是 Go',
      text: '元素是結構體，狀態是 signal，測試就像使用者一樣按下按鈕。',
      items: [
        ['沒有範本語言', '元件就是回傳元素的函式。'],
        ['細粒度更新', '按下按鈕後，只有讀取計數的那段文字會更新。'],
        ['隨處嵌入', '可將 NSGo 視圖嵌入 iOS、Android、桌面或 Web 應用中。'],
      ],
    },
    end: {
      title: '開始用 NSGo 打造應用',
      text: '跟著指南安裝工具，建立你的第一個應用。',
      getStarted: '快速開始',
      liveDemo: '線上體驗',
    },
  },
  ja: {
    features: {
      eyebrow: '特長',
      title: 'アプリに必要なすべてを、ひとつの言語で。',
      items: [
        ['真のネイティブ', '各プラットフォーム固有のビュー、テキスト、スクロール、アクセシビリティで描画します。'],
        ['リアクティブエンジン', 'シグナルは参照箇所だけを更新し、アニメーションはプラットフォームのフレームクロックで動作します。'],
        ['馴染みのあるレイアウトとスタイル', 'Flexbox・Grid、CSS ライクなスタイル、Tailwind クラス、ライト/ダークテーマ、RTL 言語に対応。'],
        ['充実したコンポーネント', 'base に無装飾、ui に即戦力を収録：ボタンやメニューからデータテーブル、チャート、コードエディタ、ターミナルまで。'],
        ['Go でも TSX でも', 'Go の値として書くことも、Node 不要の事前コンパイル付き TSX + Tailwind で書くこともできます。'],
        ['ユーザーのようにテスト', 'テキストでコントロールを見つけて押下。ヘッドレスでも実ウィンドウでも実行できます。'],
      ],
    },
    api: {
      eyebrow: 'API',
      title: 'コンポーネントは、ただの Go。',
      text: '要素は構造体、状態はシグナル、テストはユーザーと同じようにボタンを押します。',
      items: [
        ['テンプレート言語は不要', 'コンポーネントは要素を返すただの関数です。'],
        ['きめ細かな更新', 'ボタンを押しても、カウントを読むテキスト 1 箇所だけが更新されます。'],
        ['どこにでも埋め込み可能', 'iOS、Android、デスクトップ、Web アプリの中に NSGo ビューを配置できます。'],
      ],
    },
    end: {
      title: 'NSGo で開発を始めよう。',
      text: 'ガイドに沿ってツールをインストールし、最初のアプリを作りましょう。',
      getStarted: 'はじめる',
      liveDemo: 'ライブデモを試す',
    },
  },
  ko: {
    features: {
      eyebrow: '주요 특징',
      title: '앱에 필요한 모든 것을 하나의 언어로.',
      items: [
        ['진정한 네이티브', '각 플랫폼 고유의 뷰, 텍스트, 스크롤, 접근성으로 직접 그립니다.'],
        ['반응형 엔진', '시그널은 읽는 곳만 업데이트하고, 애니메이션은 플랫폼 프레임 클록에 맞춰 실행됩니다.'],
        ['익숙한 레이아웃과 스타일', 'Flexbox와 Grid, CSS 스타일, Tailwind 클래스, 라이트/다크 테마, RTL 언어를 지원합니다.'],
        ['풍부한 기본 컴포넌트', 'base는 무스타일, ui는 즉시 사용 가능: 버튼과 메뉴부터 데이터 테이블, 차트, 코드 에디터, 터미널까지.'],
        ['Go 또는 TSX', 'Go 값으로 컴포넌트를 작성하거나 Node 없이 사전 컴파일되는 TSX + Tailwind를 사용할 수 있습니다.'],
        ['사용자처럼 테스트', '텍스트로 컨트롤을 찾아 누릅니다. 헤드리스나 실제 창 모두에서 실행됩니다.'],
      ],
    },
    api: {
      eyebrow: 'API',
      title: '컴포넌트는 그저 Go입니다.',
      text: '요소는 구조체, 상태는 시그널이며 테스트는 사용자처럼 버튼을 누릅니다.',
      items: [
        ['템플릿 언어 없음', '컴포넌트는 요소를 반환하는 함수일 뿐입니다.'],
        ['세밀한 업데이트', '버튼을 눌러도 카운트를 읽는 텍스트 하나만 바뀝니다.'],
        ['어디서나 임베드', 'iOS, Android, 데스크톱, 웹 앱 안에 NSGo 뷰를 바로 넣을 수 있습니다.'],
      ],
    },
    end: {
      title: 'NSGo로 시작해 보세요.',
      text: '가이드를 따라 도구를 설치하고 첫 앱을 만들어 보세요.',
      getStarted: '시작하기',
      liveDemo: '라이브 데모 체험',
    },
  },
  es: {
    features: {
      eyebrow: 'Qué incluye',
      title: 'Todo lo que necesita una app, en un solo lenguaje.',
      items: [
        ['Verdaderamente nativo', 'Cada plataforma dibuja con sus propias vistas, texto, desplazamiento y accesibilidad.'],
        ['Motor reactivo', 'Las señales actualizan solo lo que las lee. Las animaciones siguen el reloj de fotogramas de la plataforma.'],
        ['Diseño y estilos conocidos', 'Flexbox y grid, estilos tipo CSS, clases Tailwind, temas claro y oscuro e idiomas de derecha a izquierda.'],
        ['Componentes incluidos', 'Sin estilos en base, listos para usar en ui: desde botones y menús hasta tablas de datos, gráficos, editor de código y terminal.'],
        ['Go o TSX', 'Componentes como valores Go o TSX con clases Tailwind, compilados de antemano sin Node.'],
        ['Pruebas como un usuario', 'Encuentra un control por su texto y púlsalo, sin ventana o en una ventana real.'],
      ],
    },
    api: {
      eyebrow: 'La API',
      title: 'Un componente es solo Go.',
      text: 'Los elementos son structs, el estado es una señal y una prueba pulsa el botón como lo haría un usuario.',
      items: [
        ['Sin lenguaje de plantillas', 'Un componente es una función que devuelve elementos.'],
        ['Actualizaciones precisas', 'Al pulsar solo cambia el texto que lee el contador.'],
        ['Incrústalo donde quieras', 'Coloca una vista NSGo dentro de una app de iOS, Android, escritorio o web.'],
      ],
    },
    end: {
      title: 'Empieza a construir con NSGo.',
      text: 'Sigue la guía para instalar la herramienta y crear tu primera app.',
      getStarted: 'Comenzar',
      liveDemo: 'Probar demo en vivo',
    },
  },
  fr: {
    features: {
      eyebrow: 'Ce que vous obtenez',
      title: 'Tout ce dont une app a besoin, dans un seul langage.',
      items: [
        ['Vraiment natif', 'Chaque plateforme dessine avec ses propres vues, texte, défilement et accessibilité.'],
        ['Moteur réactif', 'Les signaux ne mettent à jour que ce qui les lit. Les animations suivent l’horloge d’images de la plateforme.'],
        ['Mise en page et styles familiers', 'Flexbox et grid, styles proches de CSS, classes Tailwind, thèmes clair et sombre, langues de droite à gauche.'],
        ['Composants inclus', 'Sans style dans base, prêts à l’emploi dans ui : des boutons et menus aux tableaux, graphiques, éditeur de code et terminal.'],
        ['Go ou TSX', 'Composants en valeurs Go ou en TSX avec classes Tailwind, compilés à l’avance sans Node.'],
        ['Des tests comme un utilisateur', 'Trouvez un contrôle par son texte et cliquez dessus, sans fenêtre ou dans une vraie fenêtre.'],
      ],
    },
    api: {
      eyebrow: 'L’API',
      title: 'Un composant, c’est juste du Go.',
      text: 'Les éléments sont des structs, l’état est un signal et un test appuie sur le bouton comme le ferait un utilisateur.',
      items: [
        ['Aucun langage de template', 'Un composant est une fonction qui renvoie des éléments.'],
        ['Mises à jour fines', 'Cliquer ne modifie que le texte qui lit le compteur.'],
        ['Intégrable partout', 'Insérez une vue NSGo dans une app iOS, Android, bureau ou web.'],
      ],
    },
    end: {
      title: 'Commencez à construire avec NSGo.',
      text: 'Suivez le guide pour installer l’outil et créer votre première app.',
      getStarted: 'Démarrer',
      liveDemo: 'Essayer la démo',
    },
  },
  de: {
    features: {
      eyebrow: 'Was du bekommst',
      title: 'Alles, was eine App braucht, in einer Sprache.',
      items: [
        ['Wirklich nativ', 'Jede Plattform zeichnet mit eigenen Views, Text, Scrolling und Barrierefreiheit.'],
        ['Reaktive Engine', 'Signale aktualisieren nur, was sie liest. Animationen laufen im Frame-Takt der Plattform.'],
        ['Vertrautes Layout und Styling', 'Flexbox und Grid, CSS-ähnliche Styles, Tailwind-Klassen, helle und dunkle Themes sowie Rechts-nach-links-Sprachen.'],
        ['Komponenten inklusive', 'Ungestylt in base, sofort einsatzbereit in ui: von Buttons und Menüs bis zu Datentabellen, Diagrammen, Code-Editor und Terminal.'],
        ['Go oder TSX', 'Komponenten als Go-Werte oder TSX mit Tailwind-Klassen – vorab kompiliert, ganz ohne Node.'],
        ['Tests wie echte Nutzer', 'Finde ein Steuerelement über seinen Text und klicke es – headless oder im echten Fenster.'],
      ],
    },
    api: {
      eyebrow: 'Die API',
      title: 'Eine Komponente ist einfach Go.',
      text: 'Elemente sind Structs, Zustand ist ein Signal und ein Test drückt den Button genauso wie ein Nutzer.',
      items: [
        ['Keine Template-Sprache', 'Eine Komponente ist eine Funktion, die Elemente zurückgibt.'],
        ['Feingranulare Updates', 'Ein Klick ändert genau den einen Text, der den Zähler liest.'],
        ['Überall einbettbar', 'Setze eine NSGo-View in jede iOS-, Android-, Desktop- oder Web-App ein.'],
      ],
    },
    end: {
      title: 'Starte dein Projekt mit NSGo.',
      text: 'Folge dem Leitfaden, installiere das Tool und erstelle deine erste App.',
      getStarted: 'Loslegen',
      liveDemo: 'Live-Demo testen',
    },
  },
  pt: {
    features: {
      eyebrow: 'O que você ganha',
      title: 'Tudo o que um app precisa, em uma só linguagem.',
      items: [
        ['Verdadeiramente nativo', 'Cada plataforma desenha com suas próprias views, texto, rolagem e acessibilidade.'],
        ['Motor reativo', 'Signals atualizam apenas o que os lê. Animações acompanham o clock de quadros da plataforma.'],
        ['Layout e estilos familiares', 'Flexbox e grid, estilos parecidos com CSS, classes Tailwind, temas claro e escuro e idiomas da direita para a esquerda.'],
        ['Componentes incluídos', 'Sem estilo em base, prontos para uso em ui: de botões e menus a tabelas de dados, gráficos, editor de código e terminal.'],
        ['Go ou TSX', 'Componentes como valores Go ou TSX com classes Tailwind, compilados antecipadamente sem Node.'],
        ['Testes como um usuário', 'Encontre um controle pelo texto e pressione-o, em modo headless ou em uma janela real.'],
      ],
    },
    api: {
      eyebrow: 'A API',
      title: 'Um componente é só Go.',
      text: 'Elementos são structs, estado é um signal e um teste pressiona o botão do mesmo jeito que um usuário faria.',
      items: [
        ['Sem linguagem de templates', 'Um componente é uma função que retorna elementos.'],
        ['Atualizações granulares', 'Pressionar altera apenas o texto que lê o contador.'],
        ['Embuta em qualquer lugar', 'Coloque uma view NSGo dentro de um app iOS, Android, desktop ou web.'],
      ],
    },
    end: {
      title: 'Comece a criar com NSGo.',
      text: 'Siga o guia para instalar a ferramenta e criar seu primeiro app.',
      getStarted: 'Começar',
      liveDemo: 'Testar demo ao vivo',
    },
  },
  ru: {
    features: {
      eyebrow: 'Что вы получаете',
      title: 'Всё, что нужно приложению, на одном языке.',
      items: [
        ['По-настоящему нативно', 'Каждая платформа использует собственные views, текст, прокрутку и доступность.'],
        ['Реактивный движок', 'Сигналы обновляют только то, что их читает. Анимации идут в такт с кадрами платформы.'],
        ['Привычная вёрстка и стили', 'Flexbox и grid, CSS-подобные стили, классы Tailwind, светлая и тёмная темы, языки RTL.'],
        ['Готовые компоненты', 'Без стилей в base и готовые в ui: от кнопок и меню до таблиц, графиков, редактора кода и терминала.'],
        ['Go или TSX', 'Компоненты как значения Go или TSX с классами Tailwind — предварительная компиляция без Node.'],
        ['Тесты как у пользователя', 'Находите элемент по тексту и нажимайте его — в headless-режиме или в реальном окне.'],
      ],
    },
    api: {
      eyebrow: 'API',
      title: 'Компонент — это просто Go.',
      text: 'Элементы — структуры, состояние — сигнал, а тест нажимает кнопку так же, как пользователь.',
      items: [
        ['Без языка шаблонов', 'Компонент — это функция, возвращающая элементы.'],
        ['Точечные обновления', 'Нажатие обновляет только тот текст, который читает счётчик.'],
        ['Встраивается куда угодно', 'Поместите NSGo-представление внутрь iOS-, Android-, десктопного или веб-приложения.'],
      ],
    },
    end: {
      title: 'Начните разработку с NSGo.',
      text: 'Следуйте руководству, чтобы установить инструмент и создать первое приложение.',
      getStarted: 'Начать',
      liveDemo: 'Открыть демо',
    },
  },
};

export function buildHomeSections(repo: string, gallery: string, goApp: string): Record<SiteLang, Section[]> {
  return Object.fromEntries(
    (Object.entries(COPY) as [SiteLang, SectionCopy][]).map(([lang, c]) => [
      lang,
      [
        { type: 'component', component: './src/components/home/Screens.astro' },
        { type: 'component', component: './src/components/home/How.astro' },
        {
          type: 'features',
          id: 'features',
          eyebrow: c.features.eyebrow,
          title: c.features.title,
          items: c.features.items.map(([title, text], i) => ({ icon: FEATURE_ICONS[i], title, text })),
        },
        {
          type: 'split',
          eyebrow: c.api.eyebrow,
          title: c.api.title,
          text: c.api.text,
          items: c.api.items.map(([title, text], i) => ({ icon: API_ICONS[i], title, text })),
          code: { lang: 'go', code: goApp },
        },
        { type: 'component', component: './src/components/home/Workflow.astro' },
        {
          type: 'end',
          logo: 'nsgo',
          title: c.end.title,
          text: c.end.text,
          actions: [
            { label: c.end.getStarted, href: 'docs/getting-started/' },
            { label: c.end.liveDemo, href: gallery },
            { label: 'GitHub', href: repo, icon: 'github' },
          ],
        },
      ] satisfies Section[],
    ]),
  ) as Record<SiteLang, Section[]>;
}
