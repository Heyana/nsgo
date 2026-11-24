// Live previews: code blocks of the docs that the site shows running
// above themselves, marked in their info string.
//
//   ```go preview=button/basic height=240
//       a demo of examples/previews, the component previews' one app,
//       named in its launch parameter demo
//   ```go program height=480
//       a whole app.go, which scripts/build-live.mjs builds on its own
//
// height is the preview's in CSS pixels. Both run in one app,
// public/live/ (scripts/build-live.mjs), which takes the demo or the
// program's id in its launch parameters; the id is the hash of the
// program's code, so a page's languages share a program whose code they
// share.
import { createHash } from 'node:crypto';

export const defaultHeight = 160;

// parseLive reads a code block's mark from its meta, or returns null for
// a block without one.
export function parseLive(meta) {
  const fields = Object.fromEntries(
    (meta ?? '')
      .split(/\s+/)
      .filter(Boolean)
      .map((field) => {
        const at = field.indexOf('=');
        return at < 0 ? [field, ''] : [field.slice(0, at), field.slice(at + 1)];
      }),
  );
  const height = Number(fields.height) || defaultHeight;
  if (fields.preview) return { kind: 'preview', demo: fields.preview, height };
  if ('program' in fields) return { kind: 'program', height };
  return null;
}

// programId names a program's build after its code.
export function programId(code) {
  return createHash('sha256').update(code).digest('hex').slice(0, 12);
}

// The boxes' words while their app loads and when it fails, by the page's
// language.
const status = {
  en: { loading: 'Loading the preview…', failed: 'The preview could not start.' },
  zh: { loading: '正在加载预览…', failed: '预览无法启动。' },
  'zh-tw': { loading: '正在載入預覽…', failed: '預覽無法啟動。' },
  ja: { loading: 'プレビューを読み込み中…', failed: 'プレビューを起動できませんでした。' },
  ko: { loading: '미리보기를 불러오는 중…', failed: '미리보기를 시작할 수 없습니다.' },
  es: { loading: 'Cargando la vista previa…', failed: 'No se pudo iniciar la vista previa.' },
  fr: { loading: 'Chargement de l’aperçu…', failed: 'Impossible de démarrer l’aperçu.' },
  de: { loading: 'Vorschau wird geladen…', failed: 'Die Vorschau konnte nicht gestartet werden.' },
  pt: { loading: 'Carregando a prévia…', failed: 'Não foi possível iniciar a prévia.' },
  ru: { loading: 'Загрузка предпросмотра…', failed: 'Не удалось запустить предпросмотр.' },
};

// livePreviews puts an empty box before each marked block, which
// public/live/boxes.js, built from src/scripts/live.ts with the app, mounts
// the block's app in; the box loads that script, which runs once a page
// however many boxes load it. A Sätteri mdast plugin.
export function livePreviews({ base }) {
  return (context) => {
    const file = context.fileURL?.pathname ?? '';
    const lang = file.match(/\/docs\/public\/([a-z]{2}(?:-[a-z]{2})?)\//)?.[1] ?? 'en';
    const words = status[lang] ?? status.en;
    return {
      name: 'nsgo-live-previews',
      code(node, ctx) {
        if (node.lang !== 'go') return;
        const live = parseLive(node.meta);
        if (!live) return;
        const param = live.kind === 'preview' ? `data-demo="${live.demo}"` : `data-program="${programId(node.value)}"`;
        ctx.insertBefore(node, {
          type: 'html',
          value: `<div class="live live--${live.kind}" data-live="${base}live/" ${param} data-loading="${words.loading}" data-failed="${words.failed}" style="--live-height: ${live.height}px"></div><script type="module" src="${base}live/boxes.js"></script>`,
        });
      },
    };
  };
}
