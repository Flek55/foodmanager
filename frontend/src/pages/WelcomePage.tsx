import { ArrowLeft, Home, Ticket } from "lucide-react";
import { useState, type FormEvent } from "react";
import { Link, useNavigate, useOutletContext } from "react-router";
import type { Schemas } from "../api/client";
import { useCreateHousehold } from "../api/hooks";
import { Button, Card, TextField } from "../components/ui";
import { setCurrentHousehold } from "../lib/currentHousehold";

/** Onboarding: create a new household or join an existing one by code. */
export function WelcomePage() {
  const me = useOutletContext<Schemas["Me"]>();
  const navigate = useNavigate();
  const create = useCreateHousehold();
  const [name, setName] = useState("");
  const [code, setCode] = useState("");

  const onCreate = (e: FormEvent) => {
    e.preventDefault();
    create.mutate(name, {
      onSuccess: (h) => {
        setCurrentHousehold(h.id);
        navigate("/family");
      },
    });
  };

  const onJoin = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = code.trim();
    if (trimmed) navigate(`/join/${encodeURIComponent(trimmed)}`);
  };

  const hasHouseholds = me.households.length > 0;

  return (
    <main className="mx-auto min-h-dvh max-w-md px-4 pt-[max(2rem,env(safe-area-inset-top))] pb-10">
      {hasHouseholds && (
        <Link to="/" className="mb-6 inline-flex items-center gap-1 text-sm font-medium text-muted">
          <ArrowLeft className="size-4" /> Назад
        </Link>
      )}
      <h1 className="text-3xl font-bold tracking-tight">
        {hasHouseholds ? "Ещё одна семья" : `Привет, ${me.user.displayName}!`}
      </h1>
      <p className="mt-2 mb-8 text-muted">
        Семья — это общий список покупок и запасы дома. Создайте свою или вступите по коду от близких.
      </p>

      <Card className="mb-4">
        <div className="mb-4 flex items-center gap-3">
          <span className="flex size-10 items-center justify-center rounded-xl bg-accent/15 text-accent">
            <Home className="size-5" />
          </span>
          <h2 className="text-lg font-semibold">Создать семью</h2>
        </div>
        <form onSubmit={onCreate} className="space-y-3">
          <TextField
            label="Название"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Ивановы, Дача, Квартира на Ленина"
            error={create.error?.message}
          />
          <Button type="submit" block loading={create.isPending} disabled={!name.trim()}>
            Создать
          </Button>
        </form>
      </Card>

      <Card>
        <div className="mb-4 flex items-center gap-3">
          <span className="flex size-10 items-center justify-center rounded-xl bg-warn/15 text-warn">
            <Ticket className="size-5" />
          </span>
          <h2 className="text-lg font-semibold">Вступить по коду</h2>
        </div>
        <form onSubmit={onJoin} className="space-y-3">
          <TextField
            label="Код приглашения"
            value={code}
            onChange={(e) => setCode(e.target.value.toUpperCase())}
            placeholder="K7QM-2XPA"
            autoCapitalize="characters"
            autoComplete="off"
            spellCheck={false}
            className="[&_input]:font-mono [&_input]:tracking-widest"
          />
          <Button type="submit" variant="secondary" block disabled={!code.trim()}>
            Продолжить
          </Button>
        </form>
      </Card>
    </main>
  );
}
