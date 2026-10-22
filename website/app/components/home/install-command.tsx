import { Check, Copy } from "lucide-react"
import { useState } from "react"

/** The command that adds NSGo to a module, with a copy button. */
export function InstallCommand({
  command,
  copy,
  copied,
}: {
  command: string
  copy: string
  copied: string
}) {
  const [done, setDone] = useState(false)
  const label = done ? copied : copy
  return (
    <div className="inline-flex max-w-full items-center gap-3 rounded-full border bg-fd-card py-1.5 pr-1.5 pl-4 font-mono text-sm shadow-xs">
      <span className="text-fd-muted-foreground select-none">$</span>
      <code className="truncate">{command}</code>
      <button
        type="button"
        aria-label={label}
        title={label}
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(command)
            setDone(true)
            setTimeout(() => setDone(false), 2000)
          } catch {
            // Clipboard access can be denied; leave the button unchanged.
          }
        }}
        className="inline-flex size-8 shrink-0 items-center justify-center rounded-full text-fd-muted-foreground transition-colors hover:bg-fd-accent hover:text-fd-accent-foreground"
      >
        {done ? <Check className="size-4" /> : <Copy className="size-4" />}
      </button>
    </div>
  )
}
