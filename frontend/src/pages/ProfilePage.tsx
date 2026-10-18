import { LogOut, Share, Smartphone } from "lucide-react";
import { isIOS, isStandalone, useInstallPrompt } from "../lib/install";
import { useNavigate } from "react-router";
import { useLogout } from "../api/hooks";
import { useShell } from "../components/AppShell";
import { Avatar, Button, Card } from "../components/ui";

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

      <InstallCard />

      <Card>
        <h2 className="mb-1 font-semibold">Скоро здесь</h2>
        <p className="text-sm text-muted">Вход через Google и Telegram, уведомления в Telegram-боте.</p>
      </Card>

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
