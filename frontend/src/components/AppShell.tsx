import { ChevronDown, Check, CloudOff, ListChecks, Plus, RefreshCw, Refrigerator, User, Users } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { NavLink, Outlet, useNavigate, useOutletContext } from "react-router";
import type { Schemas } from "../api/client";
import { useCurrentHousehold } from "../lib/currentHousehold";
import { useHouseholdEvents } from "../lib/events";
import { pluralize } from "../lib/format";
import { Toaster } from "../lib/toast";
import type { SyncStatus } from "../sync/engine";
import { useListSync } from "../sync/useListSync";

export type ShellContext = {
  me: Schemas["Me"];
  household: Schemas["HouseholdSummary"];
};

/** Layout for the main app: household switcher on top, tab bar at the bottom. */
export function AppShell() {
  const me = useOutletContext<Schemas["Me"]>();
  const [household, select] = useCurrentHousehold(me);

  if (!household) return null; // AuthGate guarantees at least one

  return <Shell me={me} household={household} onSelect={select} />;
}

// Split out so hooks that need a household (live events) run only once
// there is one.
function Shell({
  me,
  household,
  onSelect: select,
}: {
  me: Schemas["Me"];
  household: Schemas["HouseholdSummary"];
  onSelect: (id: string) => void;
}) {
  useHouseholdEvents(household.id);
  const { status } = useListSync(household.id);

  return (
    <div className="mx-auto flex min-h-dvh max-w-md flex-col">
      <header className="sticky top-0 z-10 flex items-center justify-between gap-2 bg-bg/85 px-4 pt-[max(0.75rem,env(safe-area-inset-top))] pb-2 backdrop-blur">
        <HouseholdSwitcher me={me} current={household} onSelect={select} />
        <SyncBadge status={status} />
      </header>

      <main className="flex-1 px-4 pb-28">
        <Outlet context={{ me, household } satisfies ShellContext} />
      </main>

      <nav className="fixed inset-x-0 bottom-0 z-10 border-t border-line bg-surface/90 pb-[env(safe-area-inset-bottom)] backdrop-blur">
        <div className="mx-auto grid max-w-md grid-cols-4">
          <Tab to="/" icon={<ListChecks className="size-6" />} label="Список" />
          <Tab to="/home" icon={<Refrigerator className="size-6" />} label="Дома" />
          <Tab to="/family" icon={<Users className="size-6" />} label="Семья" />
          <Tab to="/profile" icon={<User className="size-6" />} label="Профиль" />
        </div>
      </nav>
      <Toaster />
    </div>
  );
}

export function useShell() {
  return useOutletContext<ShellContext>();
}

/** Quiet when all is synced; otherwise says what's going on. */
function SyncBadge({ status }: { status: SyncStatus }) {
  const changes = (n: number) => `${n} ${pluralize(n, "изменение", "изменения", "изменений")}`;
  if (!status.online) {
    return (
      <span
        className="flex shrink-0 items-center gap-1.5 rounded-full bg-warn/15 px-3 py-1 text-xs font-semibold text-warn"
        title={status.pending ? `${changes(status.pending)} отправятся, когда появится связь` : undefined}
      >
        <CloudOff className="size-3.5" />
        Офлайн{status.pending > 0 && ` · ${status.pending}`}
      </span>
    );
  }
  if (status.pending > 0) {
    return (
      <span className="flex shrink-0 items-center gap-1.5 rounded-full bg-line/60 px-3 py-1 text-xs font-semibold text-muted">
        <RefreshCw className="size-3.5 animate-spin" />
        Отправка · {status.pending}
      </span>
    );
  }
  return null;
}

function Tab({ to, icon, label }: { to: string; icon: React.ReactNode; label: string }) {
  return (
    <NavLink
      to={to}
      end
      className={({ isActive }) =>
        `flex flex-col items-center gap-0.5 py-2.5 text-xs font-medium transition ${isActive ? "text-accent" : "text-muted"}`
      }
    >
      {icon}
      {label}
    </NavLink>
  );
}

function HouseholdSwitcher({
  me,
  current,
  onSelect,
}: {
  me: Schemas["Me"];
  current: Schemas["HouseholdSummary"];
  onSelect: (id: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();

  // Close the menu on any click outside of it.
  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((o) => !o)}
        className="-mx-2 flex items-center gap-1.5 rounded-xl px-2 py-1 text-2xl font-bold tracking-tight hover:bg-line/40"
        aria-expanded={open}
      >
        <span className="max-w-[16rem] truncate">{current.name}</span>
        <ChevronDown className={`size-5 text-muted transition ${open ? "rotate-180" : ""}`} />
      </button>

      {open && (
        <div className="absolute top-full left-0 mt-2 w-72 overflow-hidden rounded-2xl border border-line bg-surface shadow-xl">
          <ul className="py-1">
            {me.households.map((h) => (
              <li key={h.id}>
                <button
                  className="flex w-full items-center justify-between px-4 py-3 text-left hover:bg-line/40"
                  onClick={() => {
                    onSelect(h.id);
                    setOpen(false);
                  }}
                >
                  <span>
                    <span className="block font-medium">{h.name}</span>
                    <span className="text-xs text-muted">{h.role === "owner" ? "владелец" : "участник"}</span>
                  </span>
                  {h.id === current.id && <Check className="size-5 text-accent" />}
                </button>
              </li>
            ))}
          </ul>
          <button
            className="flex w-full items-center gap-2 border-t border-line px-4 py-3 text-accent hover:bg-line/40"
            onClick={() => navigate("/welcome")}
          >
            <Plus className="size-5" /> Создать или вступить в семью
          </button>
        </div>
      )}
    </div>
  );
}
