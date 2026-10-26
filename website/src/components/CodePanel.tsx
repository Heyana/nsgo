import { useState, type KeyboardEvent } from 'react';
import { Highlight } from 'prism-react-renderer';
import type { Copy } from '../copy';
import { codeTheme, snippets } from '../snippets';
import { CopyButton } from './CopyButton';

export function CodePanel({ copy }: { copy: Copy }) {
  const [active, setActive] = useState(0);
  const snippet = snippets[active] ?? snippets[0];

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!(event.target instanceof HTMLElement) || !event.target.closest('[role="tab"]')) {
      return;
    }
    const count = snippets.length;
    let next: number;
    if (event.key === 'ArrowRight') next = (active + 1) % count;
    else if (event.key === 'ArrowLeft') next = (active - 1 + count) % count;
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = count - 1;
    else return;
    event.preventDefault();
    setActive(next);
    document.getElementById(`code-tab-${snippets[next].id}`)?.focus();
  };

  return (
    <div className="code-panel">
      <div className="code-panel__bar">
        <div
          className="code-panel__tabs"
          role="tablist"
          aria-label={copy.code.kicker}
          onKeyDown={onKeyDown}
        >
          {snippets.map((item, index) => {
            const labels = copy.code.tabs[index] ?? copy.code.tabs[0];
            return (
              <button
                key={item.id}
                id={`code-tab-${item.id}`}
                type="button"
                role="tab"
                className="code-panel__tab"
                aria-selected={index === active}
                aria-controls="code-panel"
                tabIndex={index === active ? 0 : -1}
                onClick={() => setActive(index)}
              >
                {labels.file}
              </button>
            );
          })}
        </div>
        <CopyButton text={snippet.code} copy={copy} showLabel />
      </div>

      <div
        id="code-panel"
        role="tabpanel"
        aria-labelledby={`code-tab-${snippet.id}`}
        tabIndex={0}
      >
        <Highlight code={snippet.code} language="go" theme={codeTheme}>
          {({ className, style, tokens, getLineProps, getTokenProps }) => (
            <pre className={`code-panel__code ${className}`} style={style}>
              {tokens.map((line, lineIndex) => {
                const lineProps = getLineProps({ line });
                return (
                  <div
                    key={lineIndex}
                    {...lineProps}
                    className={`code-panel__line ${lineProps.className ?? ''}`}
                  >
                    {line.map((token, tokenIndex) => {
                      const tokenProps = getTokenProps({ token });
                      return <span key={tokenIndex} {...tokenProps} />;
                    })}
                  </div>
                );
              })}
            </pre>
          )}
        </Highlight>
      </div>

      <div className="code-panel__foot">
        <span>{copy.code.testNote}</span>
      </div>
    </div>
  );
}
