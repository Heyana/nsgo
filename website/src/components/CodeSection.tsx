import type { Copy } from '../copy';
import { links } from '../copy';
import { CodePanel } from './CodePanel';
import { Icon } from './Icon';

export function CodeSection({ copy }: { copy: Copy }) {
  return (
    <section className="section" id="code">
      <div className="container code-layout">
        <div className="code-aside">
          <header className="section__head">
            <span className="kicker">{copy.code.kicker}</span>
            <h2>{copy.code.title}</h2>
            <p>{copy.code.lead}</p>
          </header>
          <ol className="code-aside__list">
            {copy.code.aside.map((item, index) => (
              <li className="code-aside__item" key={item.title}>
                <span className="code-aside__n">{String(index + 1).padStart(2, '0')}</span>
                <div>
                  <p className="code-aside__title">{item.title}</p>
                  <p className="code-aside__body">{item.body}</p>
                </div>
              </li>
            ))}
          </ol>
          <a
            className="code-aside__more"
            href={links.readme}
            target="_blank"
            rel="noopener noreferrer"
          >
            {copy.code.more}
            <Icon name="arrow" size={14} />
          </a>
        </div>
        <CodePanel copy={copy} />
      </div>
    </section>
  );
}
