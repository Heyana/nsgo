import type { Copy } from '../copy';
import { links } from '../copy';
import { Icon, type IconName } from './Icon';

const icons: Record<string, IconName> = {
  ios: 'smartphone',
  android: 'android',
  macos: 'laptop',
  windows: 'windows',
  linux: 'terminal',
  web: 'globe',
};

export function PlatformsSection({ copy }: { copy: Copy }) {
  return (
    <section className="section section--band" id="platforms">
      <div className="container">
        <header className="section__head">
          <span className="kicker">{copy.platforms.kicker}</span>
          <h2>{copy.platforms.title}</h2>
          <p>{copy.platforms.lead}</p>
        </header>

        <div className="platforms">
          {copy.platforms.items.map((platform) => (
            <article className="platform" key={platform.id}>
              <div className="platform__top">
                <span className="platform__icon">
                  <Icon name={icons[platform.id] ?? 'globe'} size={17} />
                </span>
                <span className="platform__status">
                  <i aria-hidden="true" />
                  {copy.platforms.status}
                </span>
              </div>
              <h3>{platform.name}</h3>
              <p className="platform__stack">{platform.stack}</p>
              <p className="platform__body">{platform.body}</p>
              <div className="platform__command">
                <span>nsgo build -target</span>
                <code>{platform.command}</code>
              </div>
            </article>
          ))}
        </div>

        <p className="section__footnote">
          <span>{copy.platforms.footnote}</span>
          <a href={links.embedding} target="_blank" rel="noopener noreferrer">
            {copy.platforms.footnoteLink}
          </a>
        </p>
      </div>
    </section>
  );
}
