import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { Copy } from '../copy';
import type { Theme } from '../hooks/useTheme';
import { Icon } from './Icon';
import { Logo } from './Logo';

type Status = 'idle' | 'loading' | 'ready' | 'error';
type LoadPhase = 'prepare' | 'download' | 'decompress' | 'compile' | 'start' | 'ready';

interface LoadState {
  phase: LoadPhase;
  percent: number;
  loaded: number;
  total: number;
}

interface Manifest {
  wasm: string;
  gzip: string;
  exec: string;
  resources: Record<string, string>;
  wasmBytes: number;
  gzipBytes: number;
}

interface GalleryMessage {
  type: 'nsgo-gallery';
  event: 'progress' | 'ready' | 'error';
  phase?: LoadPhase;
  percent?: number;
  loaded?: number;
  total?: number;
  message?: string;
}

interface Run {
  id: number;
  theme: Theme;
}

const canGzip = typeof DecompressionStream !== 'undefined';

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 MB';
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function galleryUrl(file: string, params?: Record<string, string>): string {
  const url = new URL(`gallery/${file}`, document.baseURI);
  for (const [key, value] of Object.entries(params ?? {})) url.searchParams.set(key, value);
  return url.href;
}

function isGalleryMessage(data: unknown): data is GalleryMessage {
  if (typeof data !== 'object' || data === null) return false;
  const message = data as Partial<GalleryMessage>;
  return (
    message.type === 'nsgo-gallery' &&
    (message.event === 'progress' || message.event === 'ready' || message.event === 'error')
  );
}

export function GalleryStage({ theme, copy }: { theme: Theme; copy: Copy }) {
  const [status, setStatus] = useState<Status>('idle');
  const [load, setLoad] = useState<LoadState>({
    phase: 'prepare',
    percent: 0,
    loaded: 0,
    total: 0,
  });
  const [error, setError] = useState('');
  const [run, setRun] = useState<Run | null>(null);
  const [manifest, setManifest] = useState<Manifest | null>(null);

  const stageRef = useRef<HTMLElement>(null);
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const statusRef = useRef(status);
  const themeRef = useRef(theme);
  const autoTried = useRef(false);
  const sequence = useRef(0);

  statusRef.current = status;
  themeRef.current = theme;

  const start = useCallback(() => {
    if (statusRef.current === 'loading' || statusRef.current === 'ready') return;
    autoTried.current = true;
    setError('');
    setLoad({ phase: 'prepare', percent: 0, loaded: 0, total: 0 });
    setStatus('loading');
    setRun({ id: ++sequence.current, theme: themeRef.current });
  }, []);

  const startRef = useRef(start);
  startRef.current = start;

  const cancel = () => {
    setRun(null);
    setStatus('idle');
    setError('');
  };

  useEffect(() => {
    let cancelled = false;
    fetch(galleryUrl('manifest.json'), { cache: 'no-cache' })
      .then((response) => (response.ok ? response.json() : null))
      .then((data: Manifest | null) => {
        if (!cancelled && data && typeof data.wasmBytes === 'number') setManifest(data);
      })
      .catch(() => {
        // The sizes are optional; loading works without the manifest.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const onMessage = (event: MessageEvent) => {
      if (event.origin !== window.location.origin) return;
      if (event.source !== iframeRef.current?.contentWindow) return;
      if (!isGalleryMessage(event.data)) return;
      const message = event.data;
      if (message.event === 'progress') {
        setStatus('loading');
        setLoad({
          phase: message.phase ?? 'prepare',
          percent: message.percent ?? 0,
          loaded: message.loaded ?? 0,
          total: message.total ?? 0,
        });
        return;
      }
      if (message.event === 'ready') {
        setLoad((current) => ({ ...current, phase: 'ready', percent: 100 }));
        setStatus('ready');
        return;
      }
      setError(message.message ?? '');
      setStatus('error');
    };
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
  }, []);

  useEffect(() => {
    if (!run) return;
    iframeRef.current?.contentWindow?.postMessage(
      { type: 'nsgo-gallery-theme', theme },
      window.location.origin,
    );
  }, [run, theme]);

  // If the loader page cannot start at all, the first message never comes.
  useEffect(() => {
    if (status !== 'loading' || load.total > 0) return;
    const timer = window.setTimeout(() => {
      if (statusRef.current !== 'loading') return;
      setError('');
      setStatus('error');
    }, 15000);
    return () => window.clearTimeout(timer);
  }, [status, load.total]);

  useEffect(() => {
    if (autoTried.current) return;
    const stage = stageRef.current;
    if (!stage) return;
    const connection = (
      navigator as Navigator & {
        connection?: { saveData?: boolean; effectiveType?: string };
      }
    ).connection;
    if (
      connection?.saveData ||
      connection?.effectiveType === 'slow-2g' ||
      connection?.effectiveType === '2g'
    ) {
      return;
    }
    if (!('IntersectionObserver' in window)) return;
    let timer = 0;
    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) return;
        observer.disconnect();
        timer = window.setTimeout(() => {
          autoTried.current = true;
          startRef.current();
        }, 700);
      },
      { threshold: 0.4 },
    );
    observer.observe(stage);
    return () => {
      observer.disconnect();
      window.clearTimeout(timer);
    };
  }, []);

  const frameSrc = useMemo(() => {
    if (!run) return null;
    return galleryUrl('loader.html', {
      embed: '1',
      theme: run.theme,
      run: String(run.id),
    });
  }, [run]);

  const fullSrc = galleryUrl('loader.html', { theme });

  const downloadBytes = manifest ? (canGzip ? manifest.gzipBytes : manifest.wasmBytes) : null;
  const idleMeta = copy.gallery.idleMeta(
    downloadBytes ? formatBytes(downloadBytes) : null,
    manifest && canGzip ? formatBytes(manifest.wasmBytes) : null,
  );

  const phase = load.phase;
  const percent = status === 'ready' ? 100 : load.percent;
  const step =
    phase === 'prepare' || phase === 'download'
      ? 0
      : phase === 'decompress' || phase === 'compile'
        ? 1
        : 2;

  return (
    <section
      className="stage"
      id="gallery"
      ref={stageRef}
      aria-label={copy.gallery.idleKicker}
    >
      <div className="stage__window">
        <div className="stage__bar">
          <span className="stage__dots" aria-hidden="true">
            <i />
            <i />
            <i />
          </span>
          <span className="stage__file">{copy.gallery.title}</span>
          <span className={`stage__badge${status === 'ready' ? ' is-ready' : ''}`}>
            <i aria-hidden="true" />
            {status === 'ready' ? copy.gallery.ready : copy.gallery.wasm}
          </span>
        </div>

        <div className={`stage__screen${status === 'ready' ? ' is-ready' : ''}`}>
          {frameSrc && (
            <iframe
              key={run?.id}
              ref={iframeRef}
              src={frameSrc}
              title="NSGo gallery"
              allow="clipboard-write"
              loading="eager"
              referrerPolicy="same-origin"
              aria-hidden={status === 'ready' ? undefined : true}
              tabIndex={status === 'ready' ? 0 : -1}
              onLoad={() => {
                iframeRef.current?.contentWindow?.postMessage(
                  { type: 'nsgo-gallery-theme', theme: themeRef.current },
                  window.location.origin,
                );
              }}
            />
          )}

          {status === 'idle' && (
            <div className="stage__overlay stage__overlay--idle">
              <Logo theme={theme} size={42} />
              <span className="stage__kicker">{copy.gallery.idleKicker}</span>
              <p className="stage__title">{copy.gallery.idleTitle}</p>
              <p className="stage__body">{copy.gallery.idleBody}</p>
              <p className="stage__meta">{idleMeta}</p>
              <button type="button" className="btn btn--accent" onClick={start}>
                <Icon name="play" size={14} />
                {copy.gallery.start}
              </button>
              <p className="stage__note">{copy.gallery.startNote}</p>
            </div>
          )}

          {status === 'loading' && (
            <div className="stage__overlay stage__overlay--loading">
              <p className="stage__phase" aria-live="polite">
                {copy.gallery.phases[phase]}
              </p>
              <p className="stage__percent">
                {percent}
                <span>%</span>
              </p>
              <div
                className="stage__progress"
                role="progressbar"
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={percent}
                aria-label={copy.gallery.phases[phase]}
              >
                <span style={{ width: `${percent}%` }} />
              </div>
              <p className="stage__detail">
                {load.total > 0 && load.loaded <= load.total
                  ? copy.gallery.loadingDetail(formatBytes(load.loaded), formatBytes(load.total))
                  : load.loaded > 0
                    ? formatBytes(load.loaded)
                    : copy.gallery.phases[phase]}
              </p>
              <ol className="stage__steps" aria-label={copy.gallery.stepsLabel}>
                {copy.gallery.steps.map((label, index) => (
                  <li
                    key={label}
                    aria-current={index === step ? 'step' : undefined}
                    className={
                      index === step
                        ? 'is-current'
                        : index < step
                          ? 'is-done'
                          : undefined
                    }
                  >
                    <i aria-hidden="true" />
                    {label}
                  </li>
                ))}
              </ol>
              <p className="stage__note">{copy.gallery.loadingNote}</p>
              <button type="button" className="stage__cancel" onClick={cancel}>
                {copy.gallery.cancel}
              </button>
            </div>
          )}

          {status === 'error' && (
            <div className="stage__overlay stage__overlay--error" role="alert">
              <span className="stage__error-mark" aria-hidden="true">
                <Icon name="bolt" size={20} />
              </span>
              <p className="stage__title">{copy.gallery.errorTitle}</p>
              <p className="stage__body">{copy.gallery.errorBody}</p>
              {error && <p className="stage__error-detail">{error}</p>}
              <div className="stage__error-actions">
                <button type="button" className="btn btn--accent" onClick={start}>
                  {copy.gallery.retry}
                </button>
                <button type="button" className="btn btn--secondary" onClick={cancel}>
                  {copy.gallery.cancel}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="stage__caption">
        <span>{copy.gallery.caption}</span>
        {status === 'ready' ? (
          <a href={fullSrc} target="_blank" rel="noopener noreferrer">
            {copy.gallery.open}
            <Icon name="external" size={13} />
          </a>
        ) : (
          <span className="stage__caption-note">{copy.gallery.wasm}</span>
        )}
      </div>
    </section>
  );
}
