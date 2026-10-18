import { ShoppingBasket } from "lucide-react";
import { useState, type FormEvent } from "react";
import { Navigate, useSearchParams } from "react-router";
import { isApiError } from "../api/client";
import { useLogin, useMe, useRegister } from "../api/hooks";
import { Button, ErrorBanner, FullScreenSpinner, TextField } from "../components/ui";

export function AuthPage() {
  const me = useMe();
  const [params] = useSearchParams();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [form, setForm] = useState({ email: "", password: "", displayName: "" });

  const login = useLogin();
  const register = useRegister();
  const mutation = mode === "login" ? login : register;

  // Only allow same-site relative redirects, never "//evil.com".
  const nextRaw = params.get("next") ?? "/";
  const next = nextRaw.startsWith("/") && !nextRaw.startsWith("//") ? nextRaw : "/";

  if (me.isPending) return <FullScreenSpinner />;
  if (me.data) return <Navigate to={next} replace />;

  const fieldError = (field: string) =>
    isApiError(mutation.error) && mutation.error.field === field ? mutation.error.message : undefined;
  const generalError =
    mutation.error && !(isApiError(mutation.error) && mutation.error.field) ? mutation.error.message : null;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (mode === "login") login.mutate({ email: form.email, password: form.password });
    else register.mutate(form);
  };

  const switchMode = () => {
    login.reset();
    register.reset();
    setMode((m) => (m === "login" ? "register" : "login"));
  };

  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [key]: e.target.value }));

  return (
    <main className="mx-auto flex min-h-dvh max-w-md flex-col justify-center px-4 py-10">
      <div className="mb-8">
        <div className="mb-5 inline-flex size-14 items-center justify-center rounded-2xl bg-accent text-accent-ink shadow-sm">
          <ShoppingBasket className="size-7" />
        </div>
        <h1 className="text-3xl font-bold tracking-tight">
          {mode === "login" ? "С возвращением" : "Давайте знакомиться"}
        </h1>
        <p className="mt-2 text-muted">
          {mode === "login"
            ? "Войдите, чтобы открыть общий список покупок."
            : "Один аккаунт — и вся семья видит, что купить."}
        </p>
      </div>

      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        {mode === "register" && (
          <TextField
            label="Как вас зовут"
            autoComplete="given-name"
            value={form.displayName}
            onChange={set("displayName")}
            error={fieldError("displayName")}
            placeholder="Маша"
          />
        )}
        <TextField
          label="Email"
          type="email"
          inputMode="email"
          autoComplete="email"
          value={form.email}
          onChange={set("email")}
          error={fieldError("email")}
          placeholder="masha@example.com"
        />
        <TextField
          label="Пароль"
          type="password"
          autoComplete={mode === "login" ? "current-password" : "new-password"}
          value={form.password}
          onChange={set("password")}
          error={fieldError("password")}
          hint={mode === "register" ? "Не короче 8 символов" : undefined}
        />

        <ErrorBanner message={generalError} />

        <Button type="submit" block loading={mutation.isPending}>
          {mode === "login" ? "Войти" : "Создать аккаунт"}
        </Button>
      </form>

      <p className="mt-6 text-center text-sm text-muted">
        {mode === "login" ? "Нет аккаунта?" : "Уже есть аккаунт?"}{" "}
        <button onClick={switchMode} className="font-semibold text-accent">
          {mode === "login" ? "Зарегистрироваться" : "Войти"}
        </button>
      </p>
    </main>
  );
}
