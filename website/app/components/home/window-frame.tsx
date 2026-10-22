import type { ReactNode } from "react"

import { cn } from "~/lib/utils"

/**
 * A desktop window drawn around content: traffic lights and a title above a
 * body, so a live example reads as the application window it would be.
 */
export function WindowFrame({
  title,
  className,
  children,
}: {
  title: string
  className?: string
  children: ReactNode
}) {
  return (
    <div
      className={cn(
        "overflow-hidden rounded-2xl border bg-fd-background shadow-[0_24px_80px_-24px_rgb(0_0_0/0.25),0_2px_8px_-2px_rgb(0_0_0/0.08)] dark:shadow-[0_24px_80px_-24px_rgb(0_0_0/0.7)]",
        className
      )}
    >
      <div className="relative flex h-10 items-center border-b bg-fd-card px-4">
        <div className="flex gap-2" aria-hidden>
          <span className="size-3 rounded-full bg-[#ff5f57]" />
          <span className="size-3 rounded-full bg-[#febc2e]" />
          <span className="size-3 rounded-full bg-[#28c840]" />
        </div>
        <p className="absolute inset-x-24 truncate text-center text-xs font-medium text-fd-muted-foreground">
          {title}
        </p>
      </div>
      {children}
    </div>
  )
}
