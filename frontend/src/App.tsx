import { useQuery } from "@tanstack/react-query";
import { api } from "./api/client";

// Stage 0 screen: proves the chain browser → Vite → Go → Postgres works.
// It will be replaced by the real shopping list in stage 2.
export function App() {
  const health = useQuery({
    queryKey: ["health"],
    queryFn: async () => {
      const { data, error } = await api.GET("/health");
      if (error || !data) throw new Error("backend unavailable");
      return data;
    },
    refetchInterval: 10_000,
  });

  const state = health.isPending ? "pending" : health.data?.status === "ok" ? "ok" : "down";

  return (
    <main className="mx-auto flex min-h-dvh max-w-md flex-col px-4 pt-[max(2rem,env(safe-area-inset-top))] pb-8">
      <header className="mb-8">
        <p className="text-sm font-medium tracking-wide text-muted uppercase">Семейные покупки</p>
        <h1 className="mt-1 text-4xl font-bold tracking-tight">Foodmanager</h1>
      </header>

      <section className="rounded-(--radius-card) border border-line bg-surface p-5 shadow-sm">
        <div className="flex items-center gap-3">
          <StatusDot state={state} />
          <div>
            <p className="font-semibold">
              {state === "pending" && "Проверяем связь…"}
              {state === "ok" && "Сервер на связи"}
              {state === "down" && "Сервер недоступен"}
            </p>
            <p className="text-sm text-muted">
              {health.data
                ? `БД: ${health.data.database === "ok" ? "работает" : "недоступна"} · версия ${health.data.version}`
                : "Запусти бэкенд: make run"}
            </p>
          </div>
        </div>
      </section>

      <ul className="mt-6 space-y-3 text-[15px]">
        {roadmap.map((step, i) => (
          <li key={step} className="flex items-start gap-3">
            <span
              className={`mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-bold ${
                i === 0 ? "bg-accent text-accent-ink" : "border border-line text-muted"
              }`}
            >
              {i}
            </span>
            <span className={i === 0 ? "" : "text-muted"}>{step}</span>
          </li>
        ))}
      </ul>
    </main>
  );
}

const roadmap = [
  "Каркас проекта",
  "Аккаунты, семьи, инвайт-коды",
  "Общий список покупок",
  "Офлайн и установка на телефон",
  "Деплой",
  "Запасы дома",
  "Telegram-бот",
  "Штрихкоды",
  "Прогноз «пора купить»",
];

function StatusDot({ state }: { state: "pending" | "ok" | "down" }) {
  const color = { pending: "bg-muted", ok: "bg-accent", down: "bg-danger" }[state];
  return (
    <span className="relative flex size-3">
      {state === "ok" && <span className={`absolute inline-flex size-full animate-ping rounded-full opacity-60 ${color}`} />}
      <span className={`relative inline-flex size-3 rounded-full ${color}`} />
    </span>
  );
}
