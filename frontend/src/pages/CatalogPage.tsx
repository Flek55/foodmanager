import { ArrowDown, ArrowLeft, ArrowUp, Check, Pencil, Trash2, X } from "lucide-react";
import { useState, type FormEvent } from "react";
import { Link } from "react-router";
import type { Schemas } from "../api/client";
import {
  useCreateCategory,
  useCreateStore,
  useDeleteCategory,
  useDeleteStore,
  useList,
  useRenameStore,
  useReorderCategories,
  useUpdateCategory,
} from "../api/list";
import { useShell } from "../components/AppShell";
import { Button, Card, ErrorBanner, FullScreenSpinner } from "../components/ui";
import { toast } from "../lib/toast";

/** Stores and categories of the current household. */
export function CatalogPage() {
  const { household } = useShell();
  const list = useList(household.id);

  return (
    <div className="space-y-4 pt-1">
      <Link to="/" className="inline-flex items-center gap-1 text-sm font-medium text-muted">
        <ArrowLeft className="size-4" /> К списку
      </Link>
      {list.isPending ? (
        <FullScreenSpinner />
      ) : list.isError ? (
        <ErrorBanner message={list.error.message} />
      ) : (
        <>
          <StoresCard householdId={household.id} stores={list.data.stores} />
          <CategoriesCard householdId={household.id} categories={list.data.categories} />
        </>
      )}
    </div>
  );
}

function StoresCard({ householdId, stores }: { householdId: string; stores: Schemas["Store"][] }) {
  const [name, setName] = useState("");
  const create = useCreateStore(householdId);
  const rename = useRenameStore(householdId);
  const remove = useDeleteStore(householdId);

  const onAdd = (e: FormEvent) => {
    e.preventDefault();
    create.mutate(name, { onSuccess: () => setName("") });
  };

  return (
    <Card>
      <h2 className="text-lg font-semibold">Магазины</h2>
      <p className="mt-1 mb-3 text-sm text-muted">
        Чтобы в «Ашане» видеть только то, что берут в «Ашане». Товары без магазина видны везде.
      </p>
      <ul className="divide-y divide-line/60">
        {stores.map((s) => (
          <EditableRow
            key={s.id}
            label={s.name}
            onSave={(n) => rename.mutate({ id: s.id, name: n }, { onError: (err) => toast(err.message) })}
            onDelete={() => {
              if (confirm(`Удалить магазин «${s.name}»? Товары останутся в списке.`)) remove.mutate(s.id);
            }}
          />
        ))}
      </ul>
      <form onSubmit={onAdd} className="mt-3 flex gap-2">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Пятёрочка, Аптека, Рынок…"
          aria-label="Новый магазин"
          className="h-11 min-w-0 flex-1 rounded-xl border border-line bg-surface px-3 outline-none focus:border-accent"
        />
        <Button type="submit" className="h-11" loading={create.isPending} disabled={!name.trim()}>
          Добавить
        </Button>
      </form>
      <ErrorBanner message={create.error?.message} />
    </Card>
  );
}

function CategoriesCard({ householdId, categories }: { householdId: string; categories: Schemas["Category"][] }) {
  const [form, setForm] = useState({ emoji: "", name: "" });
  const create = useCreateCategory(householdId);
  const update = useUpdateCategory(householdId);
  const remove = useDeleteCategory(householdId);
  const reorder = useReorderCategories(householdId);

  const move = (index: number, delta: -1 | 1) => {
    const ids = categories.map((c) => c.id);
    const target = index + delta;
    if (target < 0 || target >= ids.length) return;
    [ids[index], ids[target]] = [ids[target]!, ids[index]!];
    reorder.mutate(ids, { onError: (err) => toast(err.message) });
  };

  const onAdd = (e: FormEvent) => {
    e.preventDefault();
    create.mutate({ name: form.name, emoji: form.emoji || undefined }, { onSuccess: () => setForm({ emoji: "", name: "" }) });
  };

  return (
    <Card>
      <h2 className="text-lg font-semibold">Категории</h2>
      <p className="mt-1 mb-3 text-sm text-muted">
        Порядок категорий — это порядок в списке. Расставьте их так, как идёте по магазину.
      </p>
      <ul className="divide-y divide-line/60">
        {categories.map((c, i) => (
          <EditableRow
            key={c.id}
            label={c.name}
            showEmoji
            emoji={c.emoji}
            onSave={(name, emoji) =>
              update.mutate({ id: c.id, body: { name, emoji: emoji || undefined } }, { onError: (err) => toast(err.message) })
            }
            onDelete={() => {
              if (confirm(`Удалить категорию «${c.name}»? Её товары останутся без категории.`)) remove.mutate(c.id);
            }}
            extra={
              <>
                <IconButton label="Выше" disabled={i === 0} onClick={() => move(i, -1)}>
                  <ArrowUp className="size-4" />
                </IconButton>
                <IconButton label="Ниже" disabled={i === categories.length - 1} onClick={() => move(i, 1)}>
                  <ArrowDown className="size-4" />
                </IconButton>
              </>
            }
          />
        ))}
      </ul>
      <form onSubmit={onAdd} className="mt-3 flex gap-2">
        <input
          value={form.emoji}
          onChange={(e) => setForm({ ...form, emoji: e.target.value })}
          placeholder="🙂"
          aria-label="Эмодзи"
          className="h-11 w-14 rounded-xl border border-line bg-surface text-center text-lg outline-none focus:border-accent"
        />
        <input
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
          placeholder="Новая категория"
          aria-label="Название категории"
          className="h-11 min-w-0 flex-1 rounded-xl border border-line bg-surface px-3 outline-none focus:border-accent"
        />
        <Button type="submit" className="h-11" loading={create.isPending} disabled={!form.name.trim()}>
          Добавить
        </Button>
      </form>
      <ErrorBanner message={create.error?.message} />
    </Card>
  );
}

/** A row that turns into inputs when you tap the pencil. */
function EditableRow({
  label,
  showEmoji = false,
  emoji,
  onSave,
  onDelete,
  extra,
}: {
  label: string;
  showEmoji?: boolean;
  emoji?: string;
  onSave: (name: string, emoji?: string) => void;
  onDelete: () => void;
  extra?: React.ReactNode;
}) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(label);
  const [emo, setEmo] = useState(emoji ?? "");

  if (editing) {
    return (
      <li className="flex items-center gap-2 py-2">
        {showEmoji && (
          <input
            value={emo}
            onChange={(e) => setEmo(e.target.value)}
            aria-label="Эмодзи"
            className="h-10 w-12 rounded-lg border border-line bg-surface text-center outline-none focus:border-accent"
          />
        )}
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoFocus
          aria-label="Название"
          className="h-10 min-w-0 flex-1 rounded-lg border border-line bg-surface px-3 outline-none focus:border-accent"
        />
        <IconButton
          label="Сохранить"
          disabled={!name.trim()}
          onClick={() => {
            onSave(name, showEmoji ? emo : undefined);
            setEditing(false);
          }}
        >
          <Check className="size-4 text-accent" />
        </IconButton>
        <IconButton label="Отмена" onClick={() => setEditing(false)}>
          <X className="size-4" />
        </IconButton>
      </li>
    );
  }

  return (
    <li className="flex items-center gap-2 py-2">
      {showEmoji && (
        <span className="w-7 text-center text-lg" aria-hidden>
          {emoji || "•"}
        </span>
      )}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {extra}
      <IconButton
        label="Изменить"
        onClick={() => {
          setName(label);
          setEmo(emoji ?? "");
          setEditing(true);
        }}
      >
        <Pencil className="size-4" />
      </IconButton>
      <IconButton label="Удалить" onClick={onDelete}>
        <Trash2 className="size-4 text-danger" />
      </IconButton>
    </li>
  );
}

function IconButton({
  label,
  onClick,
  disabled,
  children,
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={label}
      className="rounded-lg p-2 text-muted hover:bg-line/50 hover:text-ink disabled:opacity-30"
    >
      {children}
    </button>
  );
}
