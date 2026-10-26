import { useEffect, useRef, useState } from 'react';
import type { Copy } from '../copy';
import { Icon } from './Icon';

async function copyText(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const field = document.createElement('textarea');
  field.value = text;
  field.setAttribute('readonly', '');
  field.style.position = 'fixed';
  field.style.opacity = '0';
  document.body.appendChild(field);
  field.select();
  document.execCommand('copy');
  field.remove();
}

export function CopyButton({
  text,
  copy,
  label,
  showLabel = false,
  className = '',
}: {
  text: string;
  copy: Copy;
  label?: string;
  showLabel?: boolean;
  className?: string;
}) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const onCopy = async () => {
    try {
      await copyText(text);
      setCopied(true);
      window.clearTimeout(timer.current);
      timer.current = window.setTimeout(() => setCopied(false), 1600);
    } catch {
      // A denied clipboard leaves the text selectable in place.
    }
  };

  const title = label ?? copy.ui.copy;
  return (
    <button
      type="button"
      className={`copy-button ${className}`}
      onClick={onCopy}
      title={copied ? copy.ui.copied : title}
      aria-label={copied ? copy.ui.copied : title}
    >
      <Icon name={copied ? 'check' : 'copy'} size={14} />
      {showLabel && <span>{copied ? copy.ui.copied : copy.ui.copy}</span>}
    </button>
  );
}
