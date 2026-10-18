import { useQuery } from "@tanstack/react-query";
import { EyeOff, ShoppingCart } from "lucide-react";
import { api, unwrap, type Schemas } from "../../api/client";
import { formatAgo, formatDate, pluralize } from "../../lib/format";
import { toast } from "../../lib/toast";
import { formatQuantity } from "../../lib/units";
import { newId } from "../../lib/uuid";
import type { ListSync } from "../../sync/engine";
import { openItemOf } from "../../sync/ops";
import { Sheet } from "../Sheet";
import { Button } from "../ui";
import { StockControl } from "./StockControl";

/** Everything about one tracked product: stock, purchase history, actions. */
export function ProductSheet({
  sync,
  product,
  snapshot,
  userName,
  onClose,
}: {
  sync: ListSync;
  product: Schemas["Product"];
  snapshot: Schemas["ListSnapshot"];
  userName: string;
  onClose: () => void;
}) {
  const onList = !!openItemOf(snapshot, product.id);
  const storeNames = new Map(snapshot.stores.map((s) => [s.id, s.name]));

  // History isn't part of the offline snapshot (it only grows), so it's
  // fetched when the sheet opens and simply unavailable offline.
  const history = useQuery({
    queryKey: ["purchases", sync.householdId, product.id],
    queryFn: () =>
      unwrap(
        api.GET("/households/{householdId}/products/{productId}/purchases", {
          params: { path: { householdId: sync.householdId, productId: product.id } },
        }),
      ),
    // A brand-new product created offline doesn't exist on the server yet.
    retry: false,
  });

  const setStock = (status: Schemas["StockStatus"]) => {
    const { addedToList } = sync.setStock(product.id, status, userName);
    if (addedToList) toast(`«${product.name}» добавлен в список`);
  };

  return (
    <Sheet open onClose={onClose} title={product.name}>
      <div className="space-y-6">
        <div>
          <p className="mb-2 text-sm font-medium text-muted">Сколько осталось дома</p>
          <StockControl value={product.stockStatus} onChange={setStock} size="lg" />
          {product.stockUpdatedAt && (
            <p className="mt-2 text-xs text-muted">Обновлено {formatAgo(product.stockUpdatedAt)}</p>
          )}
        </div>

        <div>
          <p className="mb-2 text-sm font-medium text-muted">Покупки</p>
          {product.purchaseCount === 0 ? (
            <p className="text-sm text-muted">Ещё не покупали через список.</p>
          ) : (
            <p className="text-sm">
              Купили {product.purchaseCount} {pluralize(product.purchaseCount, "раз", "раза", "раз")}
              {product.lastPurchasedAt && <>, последний раз — {formatAgo(product.lastPurchasedAt)}</>}
            </p>
          )}
          {history.isError && product.purchaseCount > 0 && (
            <p className="mt-2 text-xs text-muted">История подгрузится, когда будет связь.</p>
          )}
          {history.data && history.data.length > 0 && (
            <ul className="mt-3 divide-y divide-line/60 rounded-xl border border-line">
              {history.data.map((p) => (
                <li key={p.id} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
                  <span>
                    {formatDate(p.boughtAt)}
                    {p.boughtBy && <span className="text-muted"> · {p.boughtBy}</span>}
                  </span>
                  <span className="truncate text-muted">
                    {[formatQuantity(p.quantity, p.unit), p.storeId && storeNames.get(p.storeId)].filter(Boolean).join(" · ")}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="flex flex-wrap gap-2">
          {onList ? (
            <span className="inline-flex h-12 items-center gap-2 rounded-xl bg-accent/10 px-4 text-[15px] font-semibold text-accent">
              <ShoppingCart className="size-4" /> Уже в списке
            </span>
          ) : (
            <Button
              variant="secondary"
              onClick={() => {
                sync.addItem({ id: newId(), name: product.name, by: userName });
                toast(`«${product.name}» добавлен в список`);
              }}
            >
              <ShoppingCart className="size-4" /> В список
            </Button>
          )}
          <Button
            variant="ghost"
            onClick={() => {
              sync.setStock(product.id, "untracked");
              onClose();
            }}
          >
            <EyeOff className="size-4" /> Не следить
          </Button>
        </div>
      </div>
    </Sheet>
  );
}
