import {
  colorScheme,
  computed,
  effect,
  mount,
  safeAreaInsets,
  Scroll,
  setColorScheme,
  signal,
  Text,
  View,
} from "nsgo";
import { Input } from "nsgo/base";
import { Badge, Button, Card, Segmented } from "./components";
import {
  addTask,
  clearCompleted,
  completed,
  type Filter,
  filter,
  remaining,
  removeTask,
  type Task,
  tasks,
  toggleTask,
  visible,
} from "./tasks";

function Header() {
  const dark = () => colorScheme() === "dark";
  return (
    <View class="flex-row items-center justify-between">
      <View class="gap-1">
        <Text class="text-xs font-semibold uppercase tracking-widest text-indigo-600 dark:text-indigo-400">NSGo · TSX</Text>
        <Text class="text-3xl font-bold tracking-tight text-zinc-900 dark:text-white">Today</Text>
      </View>
      <View
        testID="appearance"
        onPress={() => setColorScheme(dark() ? "light" : "dark")}
        class="flex-row items-center gap-2 rounded-full border border-zinc-200 bg-white px-3 py-1.5 shadow-sm hover:bg-zinc-50 active:scale-95 dark:border-zinc-700 dark:bg-zinc-900 dark:hover:bg-zinc-800"
      >
        <View class="size-3 rounded-full" classList={{ "bg-amber-400": !dark(), "bg-indigo-400": dark() }} />
        <Text class="text-sm font-medium text-zinc-700 dark:text-zinc-200">{dark() ? "Dark" : "Light"}</Text>
      </View>
    </View>
  );
}

function Stats() {
  const stats = () => [
    { label: "Total", value: tasks().length, tone: "text-zinc-900 dark:text-white" },
    { label: "Left", value: remaining(), tone: "text-indigo-600 dark:text-indigo-400" },
    { label: "Done", value: completed(), tone: "text-emerald-600 dark:text-emerald-400" },
  ];
  return (
    <View class="grid grid-cols-3 gap-3">
      <Each of={stats()} key={(s) => s.label}>
        {(stat) => (
          <View class="gap-1 rounded-xl bg-white p-4 shadow-sm ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
            <Text class={`text-2xl font-bold ${stat().tone}`}>{stat().value}</Text>
            <Text class="text-xs font-medium uppercase tracking-wide text-zinc-500">{stat().label}</Text>
          </View>
        )}
      </Each>
    </View>
  );
}

function TaskRow(props: { task: Task }) {
  return (
    <View class="group flex-row items-center gap-3 border-t border-zinc-100 py-3 first:border-t-0 dark:border-zinc-800">
      <View
        testID={`check-${props.task.id}`}
        role="checkbox"
        checked={props.task.done}
        onPress={() => toggleTask(props.task.id)}
        class="size-5 items-center justify-center rounded-md border-2 border-zinc-300 checked:border-indigo-600 checked:bg-indigo-600 dark:border-zinc-600"
      >
        <If when={props.task.done}>
          <Text class="text-xs font-bold text-white">✓</Text>
        </If>
      </View>
      <Text
        testID={`title-${props.task.id}`}
        class="flex-1 truncate text-zinc-800 dark:text-zinc-100"
        classList={{ "line-through text-zinc-400 dark:text-zinc-500": props.task.done }}
      >
        {props.task.title}
      </Text>
      <Badge tone={props.task.priority} />
      <View
        testID={`delete-${props.task.id}`}
        onPress={() => removeTask(props.task.id)}
        class="rounded-md px-2 py-1 opacity-30 group-hover:opacity-100 hover:bg-rose-50 dark:hover:bg-rose-500/10"
      >
        <Text class="text-sm text-rose-600">Delete</Text>
      </View>
    </View>
  );
}

function Tasks() {
  const draft = signal("");
  const urgent = signal(false);
  const add = () => {
    if (addTask(draft(), urgent() ? "high" : "low")) {
      draft.set("");
      urgent.set(false);
    }
  };
  const filters: { value: Filter; label: string }[] = [
    { value: "all", label: "All" },
    { value: "active", label: "Active" },
    { value: "done", label: "Done" },
  ];
  return (
    <Card title="Tasks" subtitle={`${remaining()} left · ${completed()} done`}>
      <View class="flex-row items-center gap-2">
        <Input
          testID="draft"
          value={draft()}
          onChange={draft.set}
          onSubmit={add}
          placeholder="Add a task…"
          returnKeyType="done"
          class="h-10 flex-1 rounded-lg border border-zinc-200 bg-zinc-50 px-3 text-zinc-900 focus:border-indigo-500 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-100"
        />
        <View
          testID="urgent"
          onPress={() => urgent.set(!urgent())}
          class="size-10 items-center justify-center rounded-lg"
          classList={{ "bg-rose-500": urgent(), "bg-zinc-100 dark:bg-zinc-800": !urgent() }}
        >
          <Text class="font-bold" classList={{ "text-white": urgent(), "text-zinc-500": !urgent() }}>
            !
          </Text>
        </View>
        <Button label="Add" onPress={add} disabled={draft().trim() === ""} />
      </View>
      <Segmented options={filters} value={filter()} onChange={(v) => filter.set(v)} />
      <If when={visible().length > 0} else={<Text class="py-6 text-center text-sm text-zinc-400">Nothing here.</Text>}>
        <View>
          <Each of={visible()} key={(t) => t.id}>
            {(t) => <TaskRow task={t()} />}
          </Each>
        </View>
      </If>
      <If when={completed() > 0}>
        <Button label="Clear completed" variant="ghost" onPress={clearCompleted} />
      </If>
    </Card>
  );
}

const SESSION = 25 * 60;

function Focus() {
  const left = signal(SESSION);
  const running = signal(false);
  // A second at a time while running; the interval goes when it stops.
  effect(() => {
    if (!running()) return;
    const id = setInterval(() => {
      left.update((s) => s - 1);
      if (left() === 0) running.set(false);
    }, 1000);
    return () => clearInterval(id);
  });
  const two = (n: number) => String(n).padStart(2, "0");
  const progress = computed(() => 1 - left() / SESSION);
  const state = () => (running() ? "Running" : left() === SESSION ? "Ready" : "Paused");
  return (
    <Card title="Focus" subtitle="A 25 minute session">
      <View class="items-center gap-2 py-2">
        <Text testID="clock" class="text-6xl font-bold tracking-tight text-zinc-900 dark:text-white">
          {two(Math.floor(left() / 60))}:{two(left() % 60)}
        </Text>
        <View class="flex-row items-center gap-2">
          <View
            class="size-2 rounded-full"
            classList={{ "animate-pulse bg-emerald-500": running(), "bg-zinc-300 dark:bg-zinc-600": !running() }}
          />
          <Text class="text-xs font-medium uppercase tracking-widest text-zinc-500">{state()}</Text>
        </View>
      </View>
      <View class="h-2 overflow-hidden rounded-full bg-zinc-100 dark:bg-zinc-800">
        <View class="h-full rounded-full bg-linear-to-r from-indigo-500 to-fuchsia-500" style={{ width: `${progress() * 100}%` }} />
      </View>
      <View class="flex-row gap-2">
        <View class="flex-1">
          <Button label={running() ? "Pause" : "Start"} onPress={() => running.set(!running())} />
        </View>
        <View class="flex-1">
          <Button
            label="Reset"
            variant="ghost"
            disabled={left() === SESSION}
            onPress={() => {
              running.set(false);
              left.set(SESSION);
            }}
          />
        </View>
      </View>
    </Card>
  );
}

function App() {
  // Inline styles merge over classes side by side: the safe area adds to p-6's top and bottom.
  const insets = safeAreaInsets();
  return (
    <Scroll class="flex-1 bg-zinc-50 dark:bg-zinc-950">
      <View class="gap-6 p-6" style={{ paddingTop: 24 + insets().top, paddingBottom: 24 + insets().bottom }}>
        <Header />
        <Stats />
        {/* One column on phones; from md: (768 points) the timer goes beside the tasks. */}
        <View class="gap-6 md:flex-row md:items-start">
          <View class="md:flex-1">
            <Tasks />
          </View>
          <View class="md:w-80">
            <Focus />
          </View>
        </View>
      </View>
    </Scroll>
  );
}

mount(App);
