import type { Copy } from '../copy';
import type { Theme } from '../hooks/useTheme';
import { Icon } from './Icon';
import { Logo } from './Logo';

export function Footer({ copy, theme }: { copy: Copy; theme: Theme }) {
  return (
    <footer className="footer">
      <div className="container">
        <div className="footer__grid">
          <div className="footer__brand">
            <a className="footer__brand-link" href="#top">
              <Logo theme={theme} size={24} />
              <span>NSGo</span>
            </a>
            <p className="footer__tagline">{copy.footer.tagline}</p>
          </div>

          {copy.footer.columns.map((column) => (
            <div className="footer__column" key={column.title}>
              <h4>{column.title}</h4>
              <ul>
                {column.links.map((link) => {
                  const external = !link.href.startsWith('#');
                  return (
                    <li key={link.label}>
                      <a
                        href={link.href}
                        {...(external
                          ? { target: '_blank', rel: 'noopener noreferrer' }
                          : {})}
                      >
                        {link.label}
                      </a>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </div>

        <div className="footer__bottom">
          <span>{copy.footer.bottom}</span>
          <a href="#top" className="footer__top">
            {copy.footer.top}
            <Icon name="arrow-up" size={12} />
          </a>
        </div>
      </div>
    </footer>
  );
}
