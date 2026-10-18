import { useQueryClient } from "@tanstack/react-query";
import { LogOut, Send, Share, Smartphone } from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import type { Schemas } from "../api/client";
import {
  keys,
  useConfig,
  useCreateTelegramLink,
  useLinkTelegramWebApp,
  useLogout,
  useTelegramSettings,
  useUnlinkTelegram,
} from "../api/hooks";
import { useShell } from "../components/AppShell";
import { Avatar, Button, Card } from "../components/ui";
import { isIOS, isStandalone, useInstallPrompt } from "../lib/install";
import { isMiniApp, miniAppInitData, openTelegramLink } from "../lib/telegram";
import { toast } from "../lib/toast";

/** Offers to install the app on the home screen, the right way per platform. */
function InstallCard() {
  const { canPrompt, install } = useInstallPrompt();
  if (isStandalone()) return null; // already running as an installed app

  if (canPrompt) {
    return (
      <Card className="flex items-center gap-4">
        <Smartphone className="size-8 shrink-0 text-accent" />
        <div className="min-w-0 flex-1">
          <p className="font-semibold">Установить на телефон</p>
          <p className="text-sm text-muted">Как обычное приложение: иконка на экране, работает без интернета.</p>
        </div>
        <Button onClick={() => void install()}>Установить</Button>
      </Card>
    );
  }
  if (isIOS()) {
    return (
      <Card>
        <p className="mb-2 flex items-center gap-2 font-semibold">
          <Smartphone className="size-5 text-accent" /> Установить на iPhone
        </p>
        <ol className="list-decimal space-y-1 pl-5 text-sm text-muted">
          <li>
            Откройте эту страницу в Safari и нажмите «Поделиться» <Share className="inline size-4 align-text-bottom" />
          </li>
          <li>Выберите «На экран «Домой»»</li>
          <li>Запускайте с иконки — список будет работать и без интернета</li>
        </ol>
      </Card>
    );
  }
  return null;
}

/** Linking Telegram: notifications, adding by message, the bot. */
function TelegramCard({ me }: { me: Schemas["Me"] }) {
  const qc = useQueryClient();
  const config = useConfig();
  const createLink = useCreateTelegramLink();
  const linkWebApp = useLinkTelegramWebApp();
  const unlink = useUnlinkTelegram();
  const settings = useTelegramSettings();
  const [waiting, setWaiting] = useState(false);
  const bot = config.data?.telegramBot;
  const tg = me.telegram;

  // After opening the bot, re-check "me" until the link shows up.
  useEffect(() => {
    if (!waiting) return;
    if (tg) {
      setWaiting(false);
      toast("Telegram подключён");
      return;
    }
    const timer = setInterval(() => void qc.invalidateQueries({ queryKey: keys.me }), 3000);
    const stop = setTimeout(() => setWaiting(false), 3 * 60_000);
    return () => {
      clearInterval(timer);
      clearTimeout(stop);
    };
  }, [waiting, tg, qc]);

  if (!bot) return null;

  if (!tg) {
    const connect = () => {
      const initData = miniAppInitData();
      if (initData) {
        // Inside Telegram we already know who this is: link directly.
        linkWebApp.mutate(initData, { onSuccess: () => toast("Telegram подключён"), onError: (e) => toast(e.message) });
        return;
      }
      createLink.mutate(undefined, {
        onSuccess: (link) => {
          openTelegramLink(link.url);
          setWaiting(true);
        },
        onError: (e) => toast(e.message),
      });
    };
    return (
      <Card>
        <p className="mb-1 flex items-center gap-2 font-semibold">
          <Send className="size-5 text-accent" /> Telegram
        </p>
        <p className="mb-4 text-sm text-muted">
          Бот пришлёт, что добавили в список и что закончилось дома. А покупки можно писать ему сообщением.
        </p>
        <Button block onClick={connect} loading={createLink.isPending || linkWebApp.isPending}>
          {waiting ? "Ждём подтверждения в Telegram…" : "Подключить Telegram"}
        </Button>
      </Card>
    );
  }

  const toggle = (key: "notifyAdded" | "notifyStock") =>
    settings.mutate({ notifyAdded: tg.notifyAdded, notifyStock: tg.notifyStock, [key]: !tg[key] });

  return (
    <Card>
      <p className="mb-1 flex items-center gap-2 font-semibold">
        <Send className="size-5 text-accent" /> Telegram подключён
      </p>
      <p className="mb-3 text-sm text-muted">
        {tg.username ? `@${tg.username}` : tg.firstName} ·{" "}
        <a href={`https://t.me/${bot}`} target="_blank" rel="noreferrer" className="font-medium text-accent">
          открыть бота
        </a>
      </p>
      <div className="divide-y divide-line/60 rounded-xl border border-line">
        <SettingRow label="Что добавили в список" checked={tg.notifyAdded} onChange={() => toggle("notifyAdded")} />
        <SettingRow label="Что заканчивается дома" checked={tg.notifyStock} onChange={() => toggle("notifyStock")} />
      </div>
      <Button
        variant="ghost"
        className="mt-3"
        loading={unlink.isPending}
        onClick={() => confirm("Отключить Telegram? Уведомления перестанут приходить.") && unlink.mutate()}
      >
        Отключить
      </Button>
    </Card>
  );
}

function SettingRow({ label, checked, onChange }: { label: string; checked: boolean; onChange: () => void }) {
  return (
    <label className="flex cursor-pointer items-center justify-between gap-3 px-4 py-3 text-[15px]">
      {label}
      <input type="checkbox" checked={checked} onChange={onChange} className="size-5 accent-(--color-accent)" />
    </label>
  );
}

export function ProfilePage() {
  const { me } = useShell();
  const logout = useLogout();
  const navigate = useNavigate();

  return (
    <div className="space-y-4 pt-2">
      <Card className="flex items-center gap-4">
        <Avatar name={me.user.displayName} className="size-14 text-lg" />
        <div className="min-w-0">
          <p className="truncate text-lg font-semibold">{me.user.displayName}</p>
          {me.user.email && <p className="truncate text-sm text-muted">{me.user.email}</p>}
        </div>
      </Card>

      <TelegramCard me={me} />

      {!isMiniApp && <InstallCard />}

      <Button
        variant="secondary"
        block
        loading={logout.isPending}
        onClick={() => logout.mutate(undefined, { onSettled: () => navigate("/login") })}
      >
        <LogOut className="size-4" /> Выйти из аккаунта
      </Button>
    </div>
  );
}
