import { Check, ChevronRight, Copy, Crown, LogOut, Pencil, Share2, UserMinus, X } from "lucide-react";
import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router";
import type { Schemas } from "../api/client";
import {
  useCreateInvite,
  useHousehold,
  useInvites,
  useRemoveMember,
  useRenameHousehold,
  useRevokeInvite,
} from "../api/hooks";
import { useShell } from "../components/AppShell";
import { Avatar, Button, Card, ErrorBanner, TextField } from "../components/ui";
import { formatDate, pluralize } from "../lib/format";

export function FamilyPage() {
  const { me, household } = useShell();
  const detail = useHousehold(household.id);
  const isOwner = household.role === "owner";

  return (
    <div className="space-y-4 pt-2">
      {isOwner && <RenameCard householdId={household.id} name={household.name} />}
      <InviteCard householdId={household.id} />
      <Link
        to="/catalog"
        className="flex items-center justify-between rounded-(--radius-card) border border-line bg-surface p-5 hover:bg-line/20"
      >
        <span>
          <span className="block font-semibold">Магазины и категории</span>
          <span className="text-sm text-muted">Где покупаете и в каком порядке идут отделы</span>
        </span>
        <ChevronRight className="size-5 text-muted" />
      </Link>
      <Card>
        <h2 className="mb-3 text-lg font-semibold">
          {detail.data
            ? `${detail.data.members.length} ${pluralize(detail.data.members.length, "участник", "участника", "участников")}`
            : "Участники"}
        </h2>
        <ErrorBanner message={detail.error?.message} />
        <ul className="divide-y divide-line">
          {detail.data?.members.map((m) => (
            <MemberRow key={m.userId} member={m} householdId={household.id} isMe={m.userId === me.user.id} canRemove={isOwner} />
          ))}
        </ul>
      </Card>
      <LeaveButton householdId={household.id} userId={me.user.id} name={household.name} />
    </div>
  );
}

function RenameCard({ householdId, name }: { householdId: string; name: string }) {
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(name);
  const rename = useRenameHousehold(householdId);

  if (!editing) {
    return (
      <button
        onClick={() => {
          setValue(name);
          setEditing(true);
        }}
        className="inline-flex items-center gap-1.5 text-sm font-medium text-muted hover:text-ink"
      >
        <Pencil className="size-4" /> Переименовать семью
      </button>
    );
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    rename.mutate(value, { onSuccess: () => setEditing(false) });
  };

  return (
    <Card>
      <form onSubmit={onSubmit} className="space-y-3">
        <TextField label="Название семьи" value={value} onChange={(e) => setValue(e.target.value)} autoFocus error={rename.error?.message} />
        <div className="flex gap-2">
          <Button type="submit" loading={rename.isPending} disabled={!value.trim()}>
            Сохранить
          </Button>
          <Button type="button" variant="ghost" onClick={() => setEditing(false)}>
            Отмена
          </Button>
        </div>
      </form>
    </Card>
  );
}

function InviteCard({ householdId }: { householdId: string }) {
  const invites = useInvites(householdId);
  const create = useCreateInvite(householdId);
  const revoke = useRevokeInvite(householdId);
  const latest = invites.data?.[0];

  return (
    <Card className="bg-gradient-to-br from-accent/10 to-transparent">
      <h2 className="text-lg font-semibold">Пригласить в семью</h2>
      <p className="mt-1 mb-4 text-sm text-muted">
        Отправьте ссылку или продиктуйте код. Он действует неделю, по нему могут вступить до 10 человек.
      </p>

      {latest ? (
        <InviteCode invite={latest} />
      ) : (
        <Button block loading={create.isPending} onClick={() => create.mutate({})}>
          Получить код приглашения
        </Button>
      )}
      <ErrorBanner message={create.error?.message ?? revoke.error?.message} />

      {invites.data && invites.data.length > 0 && (
        <details className="mt-4 text-sm">
          <summary className="cursor-pointer text-muted select-none">
            Действующие коды ({invites.data.length})
          </summary>
          <ul className="mt-2 space-y-1">
            {invites.data.map((inv) => (
              <li key={inv.code} className="flex items-center justify-between rounded-lg px-2 py-1.5 hover:bg-line/30">
                <span>
                  <span className="font-mono font-semibold">{inv.code}</span>
                  <span className="ml-2 text-muted">
                    {inv.uses}/{inv.maxUses} · до {formatDate(inv.expiresAt)} · {inv.createdBy}
                  </span>
                </span>
                <button
                  onClick={() => revoke.mutate(inv.code)}
                  className="rounded-md p-1 text-muted hover:text-danger"
                  aria-label={`Отозвать код ${inv.code}`}
                >
                  <X className="size-4" />
                </button>
              </li>
            ))}
          </ul>
          <button onClick={() => create.mutate({})} className="mt-2 font-medium text-accent" disabled={create.isPending}>
            + Новый код
          </button>
        </details>
      )}
    </Card>
  );
}

function InviteCode({ invite }: { invite: Schemas["Invite"] }) {
  const [copied, setCopied] = useState(false);
  const link = `${window.location.origin}/join/${invite.code}`;
  const canShare = typeof navigator.share === "function";

  const share = async () => {
    // On phones this opens the native share sheet (Telegram, WhatsApp, ...).
    try {
      await navigator.share({ title: "Приглашение в семью", text: `Вступай в нашу семью для покупок: код ${invite.code}`, url: link });
    } catch {
      // User closed the sheet: nothing to do.
    }
  };

  const copy = async () => {
    await navigator.clipboard.writeText(link);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div>
      <div className="mb-3 rounded-xl border border-dashed border-accent/50 bg-surface py-4 text-center font-mono text-3xl font-bold tracking-[0.2em] text-ink">
        {invite.code}
      </div>
      <div className="grid grid-cols-2 gap-2">
        {canShare && (
          <Button onClick={share}>
            <Share2 className="size-4" /> Отправить
          </Button>
        )}
        <Button variant="secondary" onClick={copy} className={canShare ? "" : "col-span-2"}>
          {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
          {copied ? "Скопировано" : "Копировать ссылку"}
        </Button>
      </div>
    </div>
  );
}

function MemberRow({
  member,
  householdId,
  isMe,
  canRemove,
}: {
  member: Schemas["Member"];
  householdId: string;
  isMe: boolean;
  canRemove: boolean;
}) {
  const remove = useRemoveMember(householdId);

  const onRemove = () => {
    if (confirm(`Исключить ${member.displayName} из семьи?`)) remove.mutate(member.userId);
  };

  return (
    <li className="flex items-center gap-3 py-3">
      <Avatar name={member.displayName} />
      <div className="min-w-0 flex-1">
        <p className="truncate font-medium">
          {member.displayName}
          {isMe && <span className="text-muted"> (вы)</span>}
        </p>
        <p className="flex items-center gap-1 text-xs text-muted">
          {member.role === "owner" && <Crown className="size-3.5 text-warn" />}
          {member.role === "owner" ? "владелец" : "участник"} · с {formatDate(member.joinedAt)}
        </p>
      </div>
      {canRemove && !isMe && (
        <button onClick={onRemove} className="rounded-lg p-2 text-muted hover:bg-danger/10 hover:text-danger" aria-label="Исключить">
          <UserMinus className="size-5" />
        </button>
      )}
    </li>
  );
}

function LeaveButton({ householdId, userId, name }: { householdId: string; userId: string; name: string }) {
  const leave = useRemoveMember(householdId);
  const navigate = useNavigate();

  const onLeave = () => {
    if (!confirm(`Выйти из семьи «${name}»? Если вы последний участник, семья будет удалена.`)) return;
    leave.mutate(userId, { onSuccess: () => navigate("/") });
  };

  return (
    <>
      <ErrorBanner message={leave.error?.message} />
      <Button variant="danger" block onClick={onLeave} loading={leave.isPending}>
        <LogOut className="size-4" /> Выйти из семьи
      </Button>
    </>
  );
}
