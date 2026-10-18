import type { StockStatus } from "../../sync/ops";

const OPTIONS: { value: StockStatus; label: string; active: string }[] = [
  { value: "ok", label: "Есть", active: "bg-accent text-accent-ink" },
  { value: "low", label: "Мало", active: "bg-warn text-white" },
  { value: "out", label: "Нет", active: "bg-danger text-white" },
];

/** Three-way "how much is left at home" switch. */
export function StockControl({
  value,
  onChange,
  size = "sm",
}: {
  value?: StockStatus;
  onChange: (status: StockStatus) => void;
  size?: "sm" | "lg";
}) {
  const pad = size === "lg" ? "px-4 py-2.5 text-[15px]" : "px-2.5 py-1 text-xs";
  return (
    <div role="radiogroup" aria-label="Сколько осталось" className="inline-flex shrink-0 rounded-full bg-line/60 p-0.5">
      {OPTIONS.map((o) => {
        const active = value === o.value;
        return (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => !active && onChange(o.value)}
            className={`rounded-full font-semibold transition ${pad} ${active ? `${o.active} shadow-sm` : "text-muted hover:text-ink"}`}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}
