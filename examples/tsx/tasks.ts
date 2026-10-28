// The app's state: signals and what derives from them, shared by every
// component that imports them.
import { computed, signal } from "nsgo";

export type Priority = "low" | "high";
export type Filter = "all" | "active" | "done";
export type Task = { id: number; title: string; priority: Priority; done: boolean };

let nextId = 1;
const task = (title: string, priority: Priority = "low", done = false): Task => ({ id: nextId++, title, priority, done });

export const tasks = signal<Task[]>([
  task("Sketch the settings screen", "high"),
  task("Review the Tailwind classes"),
  task("Write the release notes", "low", true),
]);
export const filter = signal<Filter>("all");

export const remaining = computed(() => tasks().filter((t) => !t.done).length);
export const completed = computed(() => tasks().length - remaining());
export const visible = computed(() => {
  const f = filter();
  return tasks().filter((t) => f === "all" || (f === "done") === t.done);
});

/** Adds a task, reporting whether its title had any text. */
export function addTask(title: string, priority: Priority): boolean {
  const trimmed = title.trim();
  if (!trimmed) return false;
  tasks.update((list) => [...list, task(trimmed, priority)]);
  return true;
}

export function toggleTask(id: number) {
  tasks.update((list) => list.map((t) => (t.id === id ? { ...t, done: !t.done } : t)));
}

export function removeTask(id: number) {
  tasks.update((list) => list.filter((t) => t.id !== id));
}

export function clearCompleted() {
  tasks.update((list) => list.filter((t) => !t.done));
}
