import { Plus, X } from "lucide-react"
import { useState } from "react"

import type { Locale } from "~/lib/i18n"
import { cn } from "~/lib/utils"

const labels = {
  en: {
    greeting: "Hello, NSGo!",
    addPrompt: (draft: string) => `Add “${draft}”?`,
    placeholder: "Something to do",
    add: "Add",
    done: "Done",
    count: (n: number) => `${n} to do`,
    empty: "All done.",
    initial: ["Read ARCHITECTURE.md", "Port a subsystem"],
    pressed: (n: number) => `Pressed ${n} times`,
    press: "Press me",
  },
  zh: {
    greeting: "你好，NSGo！",
    addPrompt: (draft: string) => `添加“${draft}”？`,
    placeholder: "要做点什么",
    add: "添加",
    done: "完成",
    count: (n: number) => `${n} 项待办`,
    empty: "全部完成。",
    initial: ["阅读 ARCHITECTURE.md", "移植一个子系统"],
    pressed: (n: number) => `已按 ${n} 次`,
    press: "按我",
  },
} satisfies Record<Locale, unknown>

interface Todo {
  id: number
  text: string
}

/** A recreation of examples/hello, which the hero page captions as a mock. */
export function HelloWindow({ locale }: { locale: Locale }) {
  const t = labels[locale]
  const [todos, setTodos] = useState<Todo[]>(() =>
    t.initial.map((text, id) => ({ id, text }))
  )
  const [draft, setDraft] = useState("")
  const [next, setNext] = useState(t.initial.length)

  const add = () => {
    const text = draft.trim()
    if (!text) return
    setTodos((all) => [...all, { id: next, text }])
    setNext((n) => n + 1)
    setDraft("")
  }

  return (
    <div className="flex h-full flex-col gap-3 bg-fd-muted/40 p-5 text-left sm:p-6">
      <p className="text-xl font-semibold tracking-tight sm:text-2xl">
        {draft.trim() ? t.addPrompt(draft.trim()) : t.greeting}
      </p>
      <form
        onSubmit={(event) => {
          event.preventDefault()
          add()
        }}
        className="flex gap-2"
      >
        <input
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder={t.placeholder}
          aria-label={t.placeholder}
          className="h-9 min-w-0 flex-1 rounded-lg border bg-fd-card px-3 text-sm outline-none placeholder:text-fd-muted-foreground focus:border-fd-primary/60"
        />
        <button
          type="submit"
          aria-label={t.add}
          className="inline-flex h-9 items-center gap-1 rounded-lg bg-fd-primary px-3.5 text-sm font-medium text-fd-primary-foreground transition-colors hover:bg-fd-primary/90"
        >
          <Plus className="size-4" />
          {t.add}
        </button>
      </form>
      <ul className="flex min-h-0 flex-1 flex-col gap-2 overflow-auto">
        {todos.map((todo) => (
          <li
            key={todo.id}
            className="flex items-center gap-2.5 rounded-lg border bg-fd-card px-3 py-2.5"
          >
            <span className="min-w-0 flex-1 truncate text-sm">{todo.text}</span>
            <button
              type="button"
              aria-label={`${t.done}: ${todo.text}`}
              title={t.done}
              onClick={() =>
                setTodos((all) => all.filter((item) => item.id !== todo.id))
              }
              className="inline-flex size-6 shrink-0 items-center justify-center rounded-md text-fd-muted-foreground transition-colors hover:bg-fd-muted hover:text-fd-foreground"
            >
              <X className="size-3.5" />
            </button>
          </li>
        ))}
        {todos.length === 0 ? (
          <li className="py-2 text-center text-sm text-fd-muted-foreground">
            {t.empty}
          </li>
        ) : null}
      </ul>
      <p className="text-xs text-fd-muted-foreground">
        {t.count(todos.length)}
      </p>
    </div>
  )
}

/** A recreation of the counter example beside its source. */
export function CounterWindow({ locale }: { locale: Locale }) {
  const t = labels[locale]
  const [count, setCount] = useState(0)
  return (
    <div className="flex h-full flex-col justify-center gap-3 bg-fd-muted/40 p-6">
      <p
        className={cn(
          "text-lg font-semibold tracking-tight transition-colors",
          count > 0 && "text-fd-primary"
        )}
      >
        {t.pressed(count)}
      </p>
      <button
        type="button"
        onClick={() => setCount((n) => n + 1)}
        className="inline-flex h-9 w-fit items-center rounded-lg bg-fd-primary px-4 text-sm font-medium text-fd-primary-foreground transition-colors hover:bg-fd-primary/90"
      >
        {t.press}
      </button>
    </div>
  )
}
