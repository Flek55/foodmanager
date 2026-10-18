import { LogOut } from "lucide-react";
import { useNavigate } from "react-router";
import { useLogout } from "../api/hooks";
import { useShell } from "../components/AppShell";
import { Avatar, Button, Card } from "../components/ui";

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
