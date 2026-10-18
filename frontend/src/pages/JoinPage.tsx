import { Users } from "lucide-react";
import { Link, useNavigate, useParams } from "react-router";
import { useAcceptInvite, useInvitePreview, useMe } from "../api/hooks";
import { Button, Card, ErrorBanner, FullScreenSpinner } from "../components/ui";
import { setCurrentHousehold } from "../lib/currentHousehold";
import { pluralize } from "../lib/format";

/**
 * /join/:code — opened from a shared invite link. Works without login: shows
 * where the code leads, then asks to sign in and comes back here after.
 */
export function JoinPage() {
  const { code = "" } = useParams();
  const navigate = useNavigate();
  const me = useMe();
  const preview = useInvitePreview(code);
  const accept = useAcceptInvite();

  if (preview.isPending || me.isPending) return <FullScreenSpinner />;

  const onAccept = () =>
    accept.mutate(code, {
      onSuccess: (h) => {
        setCurrentHousehold(h.id);
        navigate("/");
      },
    });

  return (
    <main className="mx-auto flex min-h-dvh max-w-md flex-col justify-center px-4 py-10">
      {preview.isError ? (
        <Card className="text-center">
          <h1 className="mb-2 text-xl font-bold">Код не подошёл</h1>
          <p className="mb-5 text-muted">{preview.error.message}. Попросите прислать новый.</p>
          <Link to="/welcome" className="font-semibold text-accent">
            Ввести другой код
          </Link>
        </Card>
      ) : (
        <Card className="text-center">
          <span className="mx-auto mb-4 flex size-14 items-center justify-center rounded-2xl bg-accent/15 text-accent">
            <Users className="size-7" />
          </span>
          <p className="text-muted">{preview.data.invitedBy} приглашает вас в семью</p>
          <h1 className="mt-1 text-2xl font-bold tracking-tight">{preview.data.householdName}</h1>
          <p className="mt-1 mb-6 text-sm text-muted">
            Уже {preview.data.memberCount}{" "}
            {pluralize(preview.data.memberCount, "участник", "участника", "участников")}
          </p>

          <ErrorBanner message={accept.error?.message} />

          {me.data ? (
            <Button block onClick={onAccept} loading={accept.isPending} className="mt-2">
              Вступить
            </Button>
          ) : (
            <Button block onClick={() => navigate(`/login?next=${encodeURIComponent(`/join/${code}`)}`)}>
              Войти, чтобы вступить
            </Button>
          )}
        </Card>
      )}
    </main>
  );
}
