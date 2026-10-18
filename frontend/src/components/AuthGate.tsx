import { CloudOff } from "lucide-react";
import { Navigate, Outlet, useLocation } from "react-router";
import { isApiError } from "../api/client";
import { useMe } from "../api/hooks";
import { Button, ErrorBanner, FullScreenSpinner } from "./ui";

/**
 * Route guard: renders child routes only for a logged-in user who belongs to
 * at least one household. Otherwise it redirects to login or onboarding,
 * remembering where the user wanted to go (?next=...).
 *
 * Offline-friendly: if we have a saved "me" (persisted cache), it is used
 * even when refreshing it fails for lack of network.
 */
export function AuthGate({ requireHousehold = true }: { requireHousehold?: boolean }) {
  const me = useMe();
  const location = useLocation();

  if (me.data === undefined) {
    if (me.isError) return <GateError error={me.error} retry={() => void me.refetch()} />;
    return <FullScreenSpinner />;
  }
  if (me.data === null) {
    const next = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?next=${next}`} replace />;
  }
  if (requireHousehold && me.data.households.length === 0) {
    return <Navigate to="/welcome" replace />;
  }
  return <Outlet context={me.data} />;
}

function GateError({ error, retry }: { error: Error; retry: () => void }) {
  const offline = isApiError(error, 0);
  return (
    <div className="mx-auto flex min-h-dvh max-w-md flex-col justify-center gap-4 p-4 text-center">
      {offline ? (
        <>
          <CloudOff className="mx-auto size-10 text-muted" />
          <h1 className="text-xl font-bold">Нет связи</h1>
          <p className="text-muted">Для первого входа нужен интернет. Дальше приложение работает и без него.</p>
        </>
      ) : (
        <ErrorBanner message={error.message} />
      )}
      <Button variant="secondary" onClick={retry}>
        Попробовать снова
      </Button>
    </div>
  );
}
