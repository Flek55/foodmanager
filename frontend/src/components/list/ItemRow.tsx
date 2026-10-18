import { Check, ChevronRight } from "lucide-react";
import type { Schemas } from "../../api/client";
import { formatQuantity } from "../../lib/units";

/**
 * One list line. Tapping the line toggles "bought" — the action you do most
 * in a store, with a wet thumb, so the whole row is the target. Details
 * open from the chevron on the right.
 */
export function ItemRow({
  item,
  storeName,
  onToggle,
  onEdit,
}: {
  item: Schemas["ListItem"];
  storeName?: string;
  onToggle: () => void;
  onEdit: () => void;
}) {
  const checked = !!item.checkedAt;
  const qty = formatQuantity(item.quantity, item.unit);
  const details = [item.note, item.description, storeName].filter(Boolean).join(" · ");

  return (
    <li className="flex items-center">
      <button
        onClick={onToggle}
        className="flex min-w-0 flex-1 items-center gap-3 rounded-xl py-2.5 pr-1 pl-1 text-left transition active:bg-line/40"
        aria-pressed={checked}
      >
        <span
          className={`flex size-6 shrink-0 items-center justify-center rounded-full border-2 transition ${
            checked ? "border-accent bg-accent text-accent-ink" : "border-muted/50"
          }`}
        >
          {checked && <Check className="size-4" strokeWidth={3} />}
        </span>
        <span className="min-w-0 flex-1">
          <span className={`block truncate text-[16px] ${checked ? "text-muted line-through" : "font-medium"}`}>
            {item.name}
          </span>
          {details && <span className="block truncate text-sm text-muted">{details}</span>}
        </span>
        {qty && (
          <span
            className={`shrink-0 rounded-lg px-2 py-0.5 text-sm font-semibold tabular-nums ${
              checked ? "text-muted" : "bg-line/60"
            }`}
          >
            {qty}
          </span>
        )}
      </button>
      <button onClick={onEdit} className="shrink-0 rounded-lg p-2 text-muted/70 hover:text-ink" aria-label={`Изменить «${item.name}»`}>
        <ChevronRight className="size-5" />
      </button>
    </li>
  );
}
