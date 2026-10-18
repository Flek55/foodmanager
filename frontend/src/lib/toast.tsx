import { useSyncExternalStore } from "react";

// Minimal toast: one short message at a time, auto-hides after 2.5s.

type Toast = { id: number; text: string } | null;
let current: Toast = null;
let nextId = 1;
const listeners = new Set<() => void>();

export function toast(text: string) {
  const id = nextId++;
  current = { id, text };
  listeners.forEach((l) => l());
  setTimeout(() => {
    if (current?.id === id) {
      current = null;
      listeners.forEach((l) => l());
    }
  }, 2500);
}

function subscribe(cb: () => void) {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

export function Toaster() {
  const t = useSyncExternalStore(subscribe, () => current);
  if (!t) return null;
  return (
    // At the top, under the header: bottom sheets slide up from below, so a
    // bottom toast would cover their buttons.
    <div
      className="pointer-events-none fixed inset-x-0 top-[calc(env(safe-area-inset-top)+4.25rem)] z-50 flex justify-center px-4"
      role="status"
    >
      <div key={t.id} className="animate-fade-in rounded-full bg-ink px-4 py-2.5 text-sm font-medium text-bg shadow-lg">
        {t.text}
      </div>
    </div>
  );
}
