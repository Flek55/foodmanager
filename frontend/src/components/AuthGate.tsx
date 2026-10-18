import { Navigate, Outlet, useLocation } from "react-router";
import { useMe } from "../api/hooks";
import { ErrorBanner, FullScreenSpinner } from "./ui";

/**
 * Route guard: renders child routes only for a logged-in user who belongs to
 * at least one household. Otherwise it redirects to login or onboarding,
 * remembering where the user wanted to go (?next=...).
 */
export function AuthGate({ requireHousehold = true }: { requireHousehold?: boolean }) {
  const me = useMe();
  const location = useLocation();

  if (me.isPending) return <FullScreenSpinner />;
  if (me.isError) {
    return (
      <div className="mx-auto max-w-md p-4 pt-16">
        <ErrorBanner message={me.error.message} />
      </div>
    );
  }
  if (!me.data) {
    const next = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?next=${next}`} replace />;
  }
  if (requireHousehold && me.data.households.length === 0) {
    return <Navigate to="/welcome" replace />;
  }
  return <Outlet context={me.data} />;
}
