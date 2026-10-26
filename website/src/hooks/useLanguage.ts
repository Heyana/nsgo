import { useEffect, useState } from 'react';
import { copies, type Copy, type Lang } from '../copy';

const STORAGE_KEY = 'nsgo-lang';

function systemLanguage(): Lang {
  const languages = navigator.languages?.length ? navigator.languages : [navigator.language];
  return languages.some((language) => language.toLowerCase().startsWith('zh')) ? 'zh' : 'en';
}

function readLanguage(): Lang {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === 'en' || stored === 'zh') return stored;
  } catch {
    // Storage can be unavailable; follow the browser.
  }
  return systemLanguage();
}

export function useLanguage(): { lang: Lang; copy: Copy; setLang: (lang: Lang) => void } {
  const [lang, setLang] = useState<Lang>(readLanguage);

  useEffect(() => {
    const copy = copies[lang];
    document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';
    document.title = copy.meta.title;
    document.querySelector('meta[name="description"]')?.setAttribute('content', copy.meta.description);
    document.querySelector('meta[property="og:title"]')?.setAttribute('content', copy.meta.title);
    document
      .querySelector('meta[property="og:description"]')
      ?.setAttribute('content', copy.meta.description);
    try {
      localStorage.setItem(STORAGE_KEY, lang);
    } catch {
      // The language still applies for this visit.
    }
  }, [lang]);

  return { lang, copy: copies[lang], setLang };
}
