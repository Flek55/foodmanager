import { Trash2 } from "lucide-react";
import { useState, type FormEvent } from "react";
import type { Schemas } from "../../api/client";
import { formatDate } from "../../lib/format";
import { UNITS } from "../../lib/units";
import type { ListSync } from "../../sync/engine";
import { Chip, Sheet } from "../Sheet";
import { Button, TextField } from "../ui";

type Item = Schemas["ListItem"];

/** Start/stop watching this product on the "Дома" screen. Applies at once. */
function TrackToggle({ sync, product }: { sync: ListSync; product?: Schemas["Product"] }) {
  if (!product) return null;
  const tracked = product.stockStatus !== undefined;
  return (
    <label className="flex cursor-pointer items-center justify-between gap-3 rounded-xl border border-line px-4 py-3">
      <span>
        <span className="block text-[15px] font-medium">Следить за запасом дома</span>
        <span className="block text-xs text-muted">Появится во вкладке «Дома»: отмечайте «мало» и «нет»</span>
      </span>
      <input
        type="checkbox"
        checked={tracked}
        onChange={(e) => sync.setStock(product.id, e.target.checked ? "ok" : "untracked")}
        className="size-5 shrink-0 accent-(--color-accent)"
      />
    </label>
  );
}

const MAX_NAME = 80;
const MAX_TEXT = 200;

/**
 * Edit everything about an item. Mounted with key={item.id}, so the form
 * state starts fresh for each item. Saving is optimistic and works offline:
 * the sheet closes at once; if the server later rejects the change, the
 * sync engine rolls it back and shows a toast.
 */
export function ItemSheet({
  sync,
  item,
  snapshot,
  onClose,
}: {
  sync: ListSync;
  item: Item;
  snapshot: Schemas["ListSnapshot"];
  onClose: () => void;
}) {
  const [form, setForm] = useState({
    name: item.name,
    description: item.description ?? "",
    note: item.note ?? "",
    quantity: item.quantity !== undefined ? String(item.quantity).replace(".", ",") : "",
    unit: item.unit ?? ("pcs" as Schemas["Unit"]),
    categoryId: item.categoryId,
    storeId: item.storeId,
  });

  const qtyNumber = form.quantity.trim() ? Number(form.quantity.replace(",", ".")) : undefined;
  const qtyInvalid = qtyNumber !== undefined && (!Number.isFinite(qtyNumber) || qtyNumber <= 0);
  const name = form.name.trim();
  // Same limits as the server, checked here because offline there's no
  // server to ask.
  const nameError = !name ? "Укажите название" : name.length > MAX_NAME ? `Не длиннее ${MAX_NAME} символов` : undefined;
  const tooLong = (s: string) => (s.trim().length > MAX_TEXT ? `Не длиннее ${MAX_TEXT} символов` : undefined);
  const invalid = !!nameError || qtyInvalid || !!tooLong(form.note) || !!tooLong(form.description);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (invalid) return;
    sync.updateItem(item.id, {
      name,
      description: form.description.trim() || undefined,
      note: form.note.trim() || undefined,
      quantity: qtyNumber,
      unit: qtyNumber !== undefined ? form.unit : undefined,
      categoryId: form.categoryId,
      storeId: form.storeId,
    });
    onClose();
  };

  const onDelete = () => {
    sync.deleteItem(item.id);
    onClose();
  };

  return (
    <Sheet open onClose={onClose} title="Позиция">
      <form onSubmit={onSubmit} className="space-y-5">
        <TextField
          label="Название"
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
          error={form.name !== item.name ? nameError : undefined}
        />

        <div>
          <span className="mb-1.5 block text-sm font-medium text-muted">Сколько</span>
          <div className="flex items-center gap-2">
            <input
              value={form.quantity}
              onChange={(e) => setForm({ ...form, quantity: e.target.value })}
              inputMode="decimal"
              placeholder="—"
              aria-label="Количество"
              aria-invalid={qtyInvalid}
              className={`h-11 w-20 rounded-xl border bg-surface px-3 text-center text-base outline-none focus:border-accent ${
                qtyInvalid ? "border-danger" : "border-line"
              }`}
            />
            <div className="flex flex-wrap gap-1.5">
              {UNITS.map((u) => (
                <Chip key={u.code} active={form.unit === u.code} onClick={() => setForm({ ...form, unit: u.code })}>
                  {u.label}
                </Chip>
              ))}
            </div>
          </div>
        </div>

        <TextField
          label="Комментарий к покупке"
          value={form.note}
          onChange={(e) => setForm({ ...form, note: e.target.value })}
          placeholder="по акции, без лактозы…"
          error={tooLong(form.note)}
        />

        <div>
          <span className="mb-1.5 block text-sm font-medium text-muted">Категория</span>
          <div className="flex flex-wrap gap-1.5">
            {snapshot.categories.map((c) => (
              <Chip
                key={c.id}
                active={form.categoryId === c.id}
                onClick={() => setForm({ ...form, categoryId: form.categoryId === c.id ? undefined : c.id })}
              >
                {c.emoji} {c.name}
              </Chip>
            ))}
          </div>
        </div>

        {snapshot.stores.length > 0 && (
          <div>
            <span className="mb-1.5 block text-sm font-medium text-muted">Магазин</span>
            <div className="flex flex-wrap gap-1.5">
              <Chip active={!form.storeId} onClick={() => setForm({ ...form, storeId: undefined })}>
                Любой
              </Chip>
              {snapshot.stores.map((s) => (
                <Chip key={s.id} active={form.storeId === s.id} onClick={() => setForm({ ...form, storeId: s.id })}>
                  {s.name}
                </Chip>
              ))}
            </div>
          </div>
        )}

        <TextField
          label="Описание товара"
          hint="Запомнится для этого товара: бренд, жирность, вкус"
          value={form.description}
          onChange={(e) => setForm({ ...form, description: e.target.value })}
          placeholder="Простоквашино 3,2%"
          error={tooLong(form.description)}
        />

        <TrackToggle sync={sync} product={snapshot.products.find((p) => p.id === item.productId)} />

        <p className="text-xs text-muted">
          {item.addedBy ? `Добавил(а) ${item.addedBy}` : "Добавлено"} {formatDate(item.createdAt)}
          {item.checkedAt && ` · куплено${item.checkedBy ? `: ${item.checkedBy}` : ""}`}
        </p>

        <div className="flex gap-2">
          <Button type="submit" block disabled={invalid}>
            Сохранить
          </Button>
          <Button type="button" variant="danger" onClick={onDelete} aria-label="Удалить из списка">
            <Trash2 className="size-5" />
          </Button>
        </div>
      </form>
    </Sheet>
  );
}
