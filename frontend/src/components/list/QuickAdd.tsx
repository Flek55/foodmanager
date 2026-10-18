import { Plus } from "lucide-react";
import { useMemo, useRef, useState, type FormEvent } from "react";
import type { Schemas } from "../../api/client";
import { useAddItem } from "../../api/list";
import { toast } from "../../lib/toast";
import { formatQuantity, parseQuickAdd, searchable } from "../../lib/units";
import { newId } from "../../lib/uuid";

const MAX_SUGGESTIONS = 6;

/**
 * The input at the top of the list. Type "молоко 2л" + Enter to add; while
 * typing, products the family bought before are suggested (searched locally,
 * so it's instant and will work offline).
 */
export function QuickAdd({
  householdId,
  snapshot,
  storeId,
}: {
  householdId: string;
  snapshot: Schemas["ListSnapshot"];
  storeId: string | null;
}) {
  const [text, setText] = useState("");
  const [focused, setFocused] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const add = useAddItem(householdId);

  const parsed = parseQuickAdd(text);
  const emoji = useMemo(
    () => new Map(snapshot.categories.map((c) => [c.id, c.emoji ?? "•"])),
    [snapshot.categories],
  );
  const onList = useMemo(
    () => new Set(snapshot.items.filter((i) => !i.checkedAt).map((i) => i.productId)),
    [snapshot.items],
  );

  const suggestions = useMemo(() => {
    const q = searchable(parsed.name);
    if (!q) return [];
    return snapshot.products
      .filter((p) => searchable(p.name).includes(q))
      .sort((a, b) => {
        // Prefix matches first, then the most frequently bought.
        const ap = searchable(a.name).startsWith(q) ? 0 : 1;
        const bp = searchable(b.name).startsWith(q) ? 0 : 1;
        return ap - bp || b.useCount - a.useCount;
      })
      .slice(0, MAX_SUGGESTIONS);
  }, [parsed.name, snapshot.products]);

  const submit = (name: string, unitFallback?: Schemas["Unit"]) => {
    if (!name.trim()) return;
    add.mutate(
      {
        id: newId(),
        name,
        quantity: parsed.quantity,
        unit: parsed.quantity !== undefined ? (parsed.unit ?? unitFallback) : undefined,
        storeId: storeId ?? undefined,
      },
      {
        onSuccess: ({ item, created }) => {
          if (!created) toast(`«${item.name}» уже в списке — обновили`);
        },
        onError: (err) => toast(err.message),
      },
    );
    setText("");
    inputRef.current?.focus(); // keep the keyboard open for the next item
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    submit(parsed.name);
  };

  const showSuggestions = focused && suggestions.length > 0;

  return (
    <div className="relative">
      <form onSubmit={onSubmit} className="flex gap-2">
        <input
          ref={inputRef}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          placeholder="Что купить? Например, молоко 2 л"
          aria-label="Добавить в список"
          autoComplete="off"
          autoCapitalize="sentences"
          enterKeyHint="enter"
          className="h-13 min-w-0 flex-1 rounded-2xl border border-line bg-surface px-4 text-base shadow-sm outline-none transition placeholder:text-muted/70 focus:border-accent focus:ring-4 focus:ring-accent/15"
        />
        <button
          type="submit"
          disabled={!parsed.name}
          aria-label="Добавить"
          className="flex size-13 shrink-0 items-center justify-center rounded-2xl bg-accent text-accent-ink shadow-sm transition active:scale-95 disabled:opacity-40"
        >
          <Plus className="size-6" />
        </button>
      </form>

      {showSuggestions && (
        <ul className="absolute inset-x-0 top-full z-20 mt-2 overflow-hidden rounded-2xl border border-line bg-surface shadow-xl">
          {suggestions.map((p) => {
            const already = onList.has(p.id);
            return (
              <li key={p.id}>
                <button
                  type="button"
                  // mousedown + preventDefault: pick before the input blurs.
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => submit(p.name, p.defaultUnit)}
                  className="flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-line/40"
                >
                  <span className="text-lg" aria-hidden>
                    {p.categoryId ? emoji.get(p.categoryId) : "•"}
                  </span>
                  <span className="min-w-0 flex-1 truncate">{p.name}</span>
                  {parsed.quantity !== undefined && (
                    <span className="text-sm font-semibold text-muted">
                      {formatQuantity(parsed.quantity, parsed.unit ?? p.defaultUnit)}
                    </span>
                  )}
                  {already && <span className="rounded-md bg-accent/15 px-1.5 py-0.5 text-xs text-accent">в списке</span>}
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
