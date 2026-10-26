import type { Copy } from '../copy';
import { CopyButton } from './CopyButton';
import { Icon } from './Icon';

export function ToolchainSection({ copy }: { copy: Copy }) {
  const commands = copy.toolchain.commands.map((item) => item.command).join('\n');

  return (
    <section className="section section--band" id="toolchain">
      <div className="container">
        <header className="section__head">
          <span className="kicker">{copy.toolchain.kicker}</span>
          <h2>{copy.toolchain.title}</h2>
          <p>{copy.toolchain.lead}</p>
        </header>

        <div className="toolchain">
          <div className="terminal">
            <div className="terminal__bar">
              <Icon name="terminal" size={14} />
              <span>{copy.toolchain.terminalTitle}</span>
              <CopyButton text={commands} copy={copy} label={copy.ui.copyCommand} />
            </div>
            <pre className="terminal__body">
              {copy.toolchain.commands.map((item) => (
                <span className="terminal__line" key={item.command}>
                  <span className="terminal__prompt" aria-hidden="true">
                    $
                  </span>
                  {item.command}
                  <span className="terminal__comment"># {item.note}</span>
                </span>
              ))}
            </pre>
          </div>

          <div className="steps">
            {copy.toolchain.steps.map((step) => (
              <article className="step" key={step.api}>
                <div className="step__top">
                  <span className="step__n">{step.n}</span>
                  <code className="step__api">{step.api}</code>
                </div>
                <h3 className="step__title">{step.title}</h3>
                <p className="step__body">{step.body}</p>
              </article>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
