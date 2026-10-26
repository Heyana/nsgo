import { Fragment } from 'react';
import type { Copy } from '../copy';
import { Icon } from './Icon';

export function EngineSection({ copy }: { copy: Copy }) {
  return (
    <section className="section" id="engine">
      <div className="container">
        <header className="section__head">
          <span className="kicker">{copy.engine.kicker}</span>
          <h2>{copy.engine.title}</h2>
          <p>{copy.engine.lead}</p>
        </header>

        <div className="flow">
          {copy.engine.flow.map((node, index) => (
            <Fragment key={node.label}>
              <div className={`flow__node${index === 1 ? ' flow__node--batch' : ''}`}>
                <span className="flow__label">{node.label}</span>
                <span className="flow__title">{node.title}</span>
                <span className="flow__body">{node.body}</span>
              </div>
              {index < copy.engine.flow.length - 1 && (
                <span className="flow__arrow" aria-hidden="true">
                  <Icon name="arrow" size={18} />
                </span>
              )}
            </Fragment>
          ))}
        </div>
        <p className="flow__note">{copy.engine.flowNote}</p>

        <div className="layers">
          {copy.engine.layers.map((layer) => (
            <article className="layer" key={layer.n}>
              <span className="layer__n">{layer.n}</span>
              <h3>{layer.title}</h3>
              <p>{layer.body}</p>
            </article>
          ))}
        </div>
      </div>
    </section>
  );
}
