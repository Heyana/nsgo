import type { Copy } from '../copy';
import type { ThemePreference } from '../hooks/useTheme';
import { Icon } from './Icon';

const options = [
  { value: 'system', icon: 'monitor' },
  { value: 'light', icon: 'sun' },
  { value: 'dark', icon: 'moon' },
] as const;

export function ThemeSwitch({
  preference,
  setPreference,
  copy,
}: {
  preference: ThemePreference;
  setPreference: (preference: ThemePreference) => void;
  copy: Copy;
}) {
  return (
    <div className="theme-switch" role="group" aria-label={copy.nav.theme.label}>
      {options.map((option) => {
        const label = copy.nav.theme[option.value];
        return (
          <button
            key={option.value}
            type="button"
            className="theme-switch__button"
            aria-pressed={preference === option.value}
            aria-label={label}
            title={label}
            onClick={() => setPreference(option.value)}
          >
            <Icon name={option.icon} size={14} />
          </button>
        );
      })}
    </div>
  );
}
