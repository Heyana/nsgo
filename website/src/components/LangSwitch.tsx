import type { Copy, Lang } from '../copy';

export function LangSwitch({
  lang,
  setLang,
  copy,
}: {
  lang: Lang;
  setLang: (lang: Lang) => void;
  copy: Copy;
}) {
  return (
    <button
      type="button"
      className="lang-switch"
      aria-label={copy.nav.language}
      title={copy.nav.language}
      onClick={() => setLang(lang === 'en' ? 'zh' : 'en')}
    >
      {copy.nav.languageShort}
    </button>
  );
}
