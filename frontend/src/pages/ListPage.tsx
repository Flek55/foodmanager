import { ChevronDown, CloudOff, PartyPopper, Plus } from "lucide-react";
import { useMemo, useState } from "react";
import type { Schemas } from "../api/client";
import { useShell } from "../components/AppShell";
import { ItemRow } from "../components/list/ItemRow";
import { ItemSheet } from "../components/list/ItemSheet";
import { QuickAdd } from "../components/list/QuickAdd";
import { StoreFilter } from "../components/list/StoreFilter";
import { Card } from "../components/ui";
import { usePersistentState } from "../lib/usePersistentState";
import { newId } from "../lib/uuid";
import type { ListSync } from "../sync/engine";
import { useListSync } from "../sync/useListSync";

type Item = Schemas["ListItem"];
type Snapshot = Schemas["ListSnapshot"];

export function ListPage() {
  const { me, household } = useShell();
  const { snapshot, loaded, status, sync } = useListSync(household.id);

  if (snapshot) return <ListView sync={sync} snapshot={snapshot} userName={me.user.displayName} />;
  if (loaded && !status.online) {
    return (
      <Card className="mt-1 py-10 text-center">
        <CloudOff className="mx-auto mb-3 size-8 text-muted" />
        <h2 className="font-semibold">Нет связи</h2>
        <p className="mx-auto mt-1 max-w-xs text-sm text-muted">
          Список этой семьи ещё ни разу не загружался на этом устройстве. Он появится, как только будет интернет.
        </p>
      </Card>
    );
  }
  return <ListSkeleton />;
}

function ListView({ sync, snapshot, userName }: { sync: ListSync; snapshot: Snapshot; userName: string }) {
  const [storeFilter, setStoreFilter] = usePersistentState<string | null>(`fm.store.${sync.householdId}`, null);
  const [editingId, setEditingId] = useState<string | null>(null);

  // A remembered store may have been deleted meanwhile.
  const activeStore = snapshot.stores.some((s) => s.id === storeFilter) ? storeFilter : null;
  const storeNames = useMemo(() => new Map(snapshot.stores.map((s) => [s.id, s.name])), [snapshot.stores]);

  const visible = snapshot.items.filter((i) => !activeStore || !i.storeId || i.storeId === activeStore);
  const pending = visible.filter((i) => !i.checkedAt);
  const bought = visible
    .filter((i) => i.checkedAt)
    .sort((a, b) => (b.checkedAt ?? "").localeCompare(a.checkedAt ?? ""));
  const groups = groupByCategory(pending, snapshot.categories);
  const editing = snapshot.items.find((i) => i.id === editingId);

  const row = (item: Item) => (
    <ItemRow
      key={item.id}
      item={item}
      storeName={!activeStore && item.storeId ? storeNames.get(item.storeId) : undefined}
      onToggle={() => sync.setChecked(item.id, !item.checkedAt)}
      onEdit={() => setEditingId(item.id)}
    />
  );

  return (
    <div className="space-y-4 pt-1">
      <QuickAdd sync={sync} snapshot={snapshot} storeId={activeStore} userName={userName} />
      <StoreFilter stores={snapshot.stores} value={activeStore} onChange={setStoreFilter} />

      {pending.length === 0 ? (
        <EmptyState sync={sync} snapshot={snapshot} storeId={activeStore} hadItems={bought.length > 0} userName={userName} />
      ) : (
        <div className="space-y-3">
          {groups.map((g) => (
            <section key={g.key} className="rounded-(--radius-card) border border-line bg-surface px-3 py-2">
              <h3 className="flex items-center gap-2 px-1 pt-1 pb-0.5 text-xs font-semibold tracking-wide text-muted uppercase">
                <span className="text-base" aria-hidden>
                  {g.emoji}
                </span>
                {g.name}
                <span className="ml-auto font-normal normal-case">{g.items.length}</span>
              </h3>
              <ul className="divide-y divide-line/60">{g.items.map(row)}</ul>
            </section>
          ))}
        </div>
      )}

      {bought.length > 0 && (
        <details className="group rounded-(--radius-card) border border-line bg-surface/60 px-3 py-2">
          <summary className="flex cursor-pointer list-none items-center gap-2 px-1 py-1.5 text-sm font-semibold text-muted select-none">
            <ChevronDown className="size-4 transition group-open:rotate-180" />
            В корзине · {bought.length}
            <button
              onClick={(e) => {
                e.preventDefault();
                sync.clearChecked();
              }}
              className="ml-auto rounded-lg px-2 py-1 text-xs font-semibold text-danger hover:bg-danger/10"
            >
              Убрать купленное
            </button>
          </summary>
          <ul className="divide-y divide-line/60">{bought.map(row)}</ul>
        </details>
      )}

      {editing && (
        <ItemSheet key={editing.id} sync={sync} item={editing} snapshot={snapshot} onClose={() => setEditingId(null)} />
      )}
    </div>
  );
}

type Group = { key: string; name: string; emoji: string; items: Item[] };

/** Groups items by category in the family's category order (= aisle order). */
function groupByCategory(items: Item[], categories: Schemas["Category"][]): Group[] {
  const byId = new Map<string, Group>();
  for (const c of categories) byId.set(c.id, { key: c.id, name: c.name, emoji: c.emoji ?? "•", items: [] });
  const none: Group = { key: "none", name: "Без категории", emoji: "•", items: [] };

  for (const item of items) {
    (item.categoryId ? (byId.get(item.categoryId) ?? none) : none).items.push(item);
  }
  const collator = new Intl.Collator("ru");
  return [...categories.map((c) => byId.get(c.id)!), none]
    .filter((g) => g.items.length > 0)
    .map((g) => ({ ...g, items: g.items.sort((a, b) => collator.compare(a.name, b.name)) }));
}

/** Empty list: celebrate, and offer one-tap re-adding of usual purchases. */
function EmptyState({
  sync,
  snapshot,
  storeId,
  hadItems,
  userName,
}: {
  sync: ListSync;
  snapshot: Snapshot;
  storeId: string | null;
  hadItems: boolean;
  userName: string;
}) {
  const onList = new Set(snapshot.items.filter((i) => !i.checkedAt).map((i) => i.productId));
  const usual = snapshot.products
    .filter((p) => p.useCount > 0 && !onList.has(p.id))
    .sort((a, b) => b.useCount - a.useCount)
    .slice(0, 10);

  return (
    <Card className="py-8 text-center">
      <span className="mx-auto mb-3 flex size-14 items-center justify-center rounded-2xl bg-accent/15 text-accent">
        <PartyPopper className="size-7" />
      </span>
      <h2 className="text-lg font-bold">{hadItems ? "Всё куплено!" : "Список пуст"}</h2>
      <p className="mx-auto mt-1 max-w-xs text-sm text-muted">
        {snapshot.products.length === 0
          ? "Напишите в поле выше, что нужно купить. Можно сразу с количеством: «яйца 10»."
          : "Добавьте, что закончилось, — вся семья сразу увидит."}
      </p>
      {usual.length > 0 && (
        <div className="mt-5">
          <p className="mb-2 text-xs font-semibold tracking-wide text-muted uppercase">Часто покупаете</p>
          <div className="flex flex-wrap justify-center gap-2">
            {usual.map((p) => (
              <button
                key={p.id}
                onClick={() => sync.addItem({ id: newId(), name: p.name, storeId: storeId ?? undefined, by: userName })}
                className="inline-flex items-center gap-1 rounded-full border border-line bg-bg px-3 py-1.5 text-sm font-medium hover:border-accent hover:text-accent"
              >
                <Plus className="size-3.5" /> {p.name}
              </button>
            ))}
          </div>
        </div>
      )}
    </Card>
  );
}

function ListSkeleton() {
  return (
    <div className="animate-pulse space-y-4 pt-1" aria-label="Загрузка">
      <div className="h-13 rounded-2xl bg-line/60" />
      <div className="h-8 w-2/3 rounded-full bg-line/60" />
      {[0, 1].map((i) => (
        <div key={i} className="h-36 rounded-(--radius-card) bg-line/40" />
      ))}
    </div>
  );
}
