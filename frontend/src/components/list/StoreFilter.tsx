import { Plus } from "lucide-react";
import { Link } from "react-router";
import type { Schemas } from "../../api/client";
import { Chip } from "../Sheet";

/**
 * "Where am I shopping?" Picking a store hides items meant for other stores
 * (items without a store are shown everywhere) and new items get that store.
 */
export function StoreFilter({
  stores,
  value,
  onChange,
}: {
  stores: Schemas["Store"][];
  value: string | null;
  onChange: (id: string | null) => void;
}) {
  return (
    <div className="-mx-4 flex gap-2 overflow-x-auto px-4 pb-1 [scrollbar-width:none]">
      <Chip active={value === null} onClick={() => onChange(null)}>
        Все магазины
      </Chip>
      {stores.map((s) => (
        <Chip key={s.id} active={value === s.id} onClick={() => onChange(s.id)}>
          {s.name}
        </Chip>
      ))}
      <Link
        to="/catalog"
        className="inline-flex shrink-0 items-center gap-1 rounded-full border border-dashed border-line px-3.5 py-1.5 text-sm font-medium text-muted hover:text-ink"
      >
        <Plus className="size-4" /> {stores.length === 0 ? "Добавить магазин" : "Магазин"}
      </Link>
    </div>
  );
}
