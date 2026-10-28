// Small styled components: their looks are Tailwind classes, their
// variants classList entries.
import { type JSX, Text, View } from "nsgo";

export function Card(props: { title: string; subtitle?: string; children?: JSX.Element }) {
  return (
    <View class="gap-4 rounded-2xl border border-zinc-200 bg-white p-5 shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
      <View class="gap-1">
        <Text class="text-lg font-semibold text-zinc-900 dark:text-zinc-50">{props.title}</Text>
        <If when={props.subtitle}>
          <Text class="text-sm text-zinc-500 dark:text-zinc-400">{props.subtitle}</Text>
        </If>
      </View>
      {props.children}
    </View>
  );
}

export function Button(props: { label: string; onPress: () => void; variant?: "primary" | "ghost"; disabled?: boolean }) {
  const primary = () => props.variant !== "ghost";
  return (
    <View
      role="button"
      disabled={props.disabled}
      onPress={props.onPress}
      class="h-10 items-center justify-center rounded-lg px-4 transition-colors active:scale-95 disabled:opacity-40"
      classList={{
        "bg-indigo-600 hover:bg-indigo-500 active:bg-indigo-700": primary(),
        "bg-zinc-100 hover:bg-zinc-200 active:bg-zinc-300 dark:bg-zinc-800 dark:hover:bg-zinc-700": !primary(),
      }}
    >
      <Text
        class="text-sm font-medium"
        classList={{ "text-white": primary(), "text-zinc-700 dark:text-zinc-200": !primary() }}
      >
        {props.label}
      </Text>
    </View>
  );
}

export function Badge(props: { tone: "low" | "high" }) {
  const high = () => props.tone === "high";
  return (
    <View
      class="rounded-full px-2 py-0.5"
      classList={{ "bg-rose-100 dark:bg-rose-500/20": high(), "bg-zinc-100 dark:bg-zinc-800": !high() }}
    >
      <Text
        class="text-xs font-semibold uppercase tracking-wide"
        classList={{ "text-rose-600 dark:text-rose-300": high(), "text-zinc-500 dark:text-zinc-400": !high() }}
      >
        {props.tone}
      </Text>
    </View>
  );
}

export function Segmented<T extends string>(props: {
  options: { value: T; label: string }[];
  value: T;
  onChange: (value: T) => void;
}) {
  return (
    <View class="flex-row rounded-xl bg-zinc-100 p-1 dark:bg-zinc-800">
      <Each of={props.options} key={(o) => o.value}>
        {(option) => {
          const on = () => props.value === option().value;
          return (
            <View
              onPress={() => props.onChange(option().value)}
              class="flex-1 items-center rounded-lg py-1.5 transition-colors"
              classList={{ "bg-white shadow-sm dark:bg-zinc-700": on(), "hover:bg-black/5 dark:hover:bg-white/5": !on() }}
            >
              <Text
                class="text-sm font-medium"
                classList={{ "text-zinc-900 dark:text-white": on(), "text-zinc-500 dark:text-zinc-400": !on() }}
              >
                {option().label}
              </Text>
            </View>
          );
        }}
      </Each>
    </View>
  );
}
