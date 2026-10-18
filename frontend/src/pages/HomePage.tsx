import { CloudOff, Plus, Refrigerator, ShoppingCart } from "lucide-react";
import { useState, type FormEvent } from "react";
import type { Schemas } from "../api/client";
import { useShell } from "../components/AppShell";
import { ProductSheet } from "../components/stock/ProductSheet";
import { StockControl } from "../components/stock/StockControl";
import { Card } from "../components/ui";
import { formatAgo } from "../lib/format";
import { toast } from "../lib/toast";
import type { ListSync } from "../sync/engine";
import { openItemOf, type StockStatus } from "../sync/ops";
import { useListSync } from "../sync/useListSync";

type Product = Schemas["Product"];
type Snapshot = Schemas["ListSnapshot"];

/** "Дома": what the family keeps at home and how much of it is left. */
export function HomePage() {
  const { me, household } = useShell();
  const { snapshot, loaded, status, sync } = useListSync(household.id);

  if (snapshot) return <HomeView sync={sync} snapshot={snapshot} userName={me.user.displayName} />;
  if (loaded && !status.online) {
    return (
      <Card className="mt-1 py-10 text-center">
        <CloudOff className="mx-auto mb-3 size-8 text-muted" />
        <p className="text-muted">Данные появятся, как только будет интернет.</p>
      </Card>
    );
  }
  return null;
}

const SECTIONS: { status: StockStatus; title: string; tone: string }[] = [
  { status: "out", title: "Закончилось", tone: "text-danger" },
  { status: "low", title: "Заканчивается", tone: "text-warn" },
  { status: "ok", title: "Есть дома", tone: "text-muted" },
];

function HomeView({ sync, snapshot, userName }: { sync: ListSync; snapshot: Snapshot; userName: string }) {
  const [openId, setOpenId] = useState<string | null>(null);
  const tracked = snapshot.products.filter((p) => p.stockStatus);
  const open = snapshot.products.find((p) => p.id === openId);

  // Category order, then name: the same order as aisles on the list.
  const categoryOrder = new Map(snapshot.categories.map((c, i) => [c.id, i]));
  const collator = new Intl.Collator("ru");
  const sorted = (list: Product[]) =>
    [...list].sort(
      (a, b) =>
        (categoryOrder.get(a.categoryId ?? "") ?? 999) - (categoryOrder.get(b.categoryId ?? "") ?? 999) ||
        collator.compare(a.name, b.name),
    );

  const setStock = (product: Product, status: StockStatus) => {
    const { addedToList } = sync.setStock(product.id, status, userName);
    if (addedToList) toast(`«${product.name}» добавлен в список`);
  };

  return (
    <div className="space-y-4 pt-1">
      <TrackInput sync={sync} userName={userName} />

      {tracked.length === 0 ? (
        <EmptyHome sync={sync} snapshot={snapshot} userName={userName} />
      ) : (
        SECTIONS.map(({ status, title, tone }) => {
          const products = sorted(tracked.filter((p) => p.stockStatus === status));
          if (products.length === 0) return null;
          return (
            <section key={status} className="rounded-(--radius-card) border border-line bg-surface px-3 py-2">
              <h3 className={`flex items-center px-1 pt-1 pb-0.5 text-xs font-semibold tracking-wide uppercase ${tone}`}>
                {title}
                <span className="ml-auto font-normal text-muted normal-case">{products.length}</span>
              </h3>
              <ul className="divide-y divide-line/60">
                {products.map((p) => (
                  <StockRow
                    key={p.id}
                    product={p}
                    snapshot={snapshot}
                    onOpen={() => setOpenId(p.id)}
                    onChange={(s) => setStock(p, s)}
                  />
                ))}
              </ul>
            </section>
          );
        })
      )}

      {open && (
        <ProductSheet
          key={open.id}
          sync={sync}
          product={open}
          snapshot={snapshot}
          userName={userName}
          onClose={() => setOpenId(null)}
        />
      )}
    </div>
  );
}

function StockRow({
  product,
  snapshot,
  onOpen,
  onChange,
}: {
  product: Product;
  snapshot: Snapshot;
  onOpen: () => void;
  onChange: (s: StockStatus) => void;
}) {
  const emoji = snapshot.categories.find((c) => c.id === product.categoryId)?.emoji ?? "•";
  const onList = !!openItemOf(snapshot, product.id);
  return (
    <li className="flex items-center gap-2 py-2">
      <button onClick={onOpen} className="flex min-w-0 flex-1 items-center gap-3 rounded-xl px-1 py-1 text-left active:bg-line/40">
        <span className="text-lg" aria-hidden>
          {emoji}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate font-medium">{product.name}</span>
          <span className="flex items-center gap-1.5 truncate text-xs text-muted">
            {onList && (
              <span className="inline-flex items-center gap-1 font-semibold text-accent">
                <ShoppingCart className="size-3" /> в списке
              </span>
            )}
            {onList && product.lastPurchasedAt && "·"}
            {product.lastPurchasedAt && <>куплено {formatAgo(product.lastPurchasedAt)}</>}
          </span>
        </span>
      </button>
      <StockControl value={product.stockStatus} onChange={onChange} />
    </li>
  );
}

function TrackInput({ sync, userName }: { sync: ListSync; userName: string }) {
  const [name, setName] = useState("");
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    const { alreadyTracked } = sync.trackProduct(name, "ok", userName);
    if (alreadyTracked) toast(`За «${name.trim()}» уже следим`);
    setName("");
  };
  return (
    <form onSubmit={onSubmit} className="flex gap-2">
      <input
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="За чем следить? Например, кофе"
        aria-label="Следить за товаром"
        autoComplete="off"
        className="h-13 min-w-0 flex-1 rounded-2xl border border-line bg-surface px-4 text-base shadow-sm outline-none transition placeholder:text-muted/70 focus:border-accent focus:ring-4 focus:ring-accent/15"
      />
      <button
        type="submit"
        disabled={!name.trim()}
        aria-label="Следить"
        className="flex size-13 shrink-0 items-center justify-center rounded-2xl bg-accent text-accent-ink shadow-sm transition active:scale-95 disabled:opacity-40"
      >
        <Plus className="size-6" />
      </button>
    </form>
  );
}

/** Nothing tracked yet: explain, and offer what the family buys most. */
function EmptyHome({ sync, snapshot, userName }: { sync: ListSync; snapshot: Snapshot; userName: string }) {
  const frequent = snapshot.products
    .filter((p) => !p.stockStatus && (p.purchaseCount > 0 || p.useCount > 1))
    .sort((a, b) => b.purchaseCount - a.purchaseCount || b.useCount - a.useCount)
    .slice(0, 12);

  return (
    <Card className="py-8 text-center">
      <span className="mx-auto mb-3 flex size-14 items-center justify-center rounded-2xl bg-accent/15 text-accent">
        <Refrigerator className="size-7" />
      </span>
      <h2 className="text-lg font-bold">Что есть дома</h2>
      <p className="mx-auto mt-1 max-w-xs text-sm text-muted">
        Добавьте то, что всегда должно быть дома. Отметили «Мало» или «Нет» — товар сам попадёт в список покупок. Купили —
        снова «Есть».
      </p>
      {frequent.length > 0 && (
        <div className="mt-5">
          <p className="mb-2 text-xs font-semibold tracking-wide text-muted uppercase">Вы часто покупаете</p>
          <div className="flex flex-wrap justify-center gap-2">
            {frequent.map((p) => (
              <button
                key={p.id}
                onClick={() => sync.setStock(p.id, "ok", userName)}
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
