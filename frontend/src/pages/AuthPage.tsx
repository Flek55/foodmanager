import { useQueryClient } from "@tanstack/react-query";
import { Send, ShoppingBasket } from "lucide-react";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Navigate, useSearchParams } from "react-router";
import { api, isApiError, unwrap, type Schemas } from "../api/client";
import { keys, useConfig, useLinkTelegramWebApp, useLogin, useMe, useRegister, useTelegramWebAppLogin } from "../api/hooks";
import { Button, Card, ErrorBanner, FullScreenSpinner, TextField } from "../components/ui";
import { isMiniApp, miniAppInitData, openTelegramLink } from "../lib/telegram";
import { toast } from "../lib/toast";

export function AuthPage() {
  const me = useMe();
  const config = useConfig();
  const [params] = useSearchParams();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [form, setForm] = useState({ email: "", password: "", displayName: "" });

  const login = useLogin();
  const register = useRegister();
  const linkWebApp = useLinkTelegramWebApp();
  const mutation = mode === "login" ? login : register;
  const miniApp = useMiniAppLogin(me.data === null);

  // Only allow same-site relative redirects, never "//evil.com".
  const nextRaw = params.get("next") ?? "/";
  const next = nextRaw.startsWith("/") && !nextRaw.startsWith("//") ? nextRaw : "/";

  if (me.isPending) return <FullScreenSpinner />;
  if (me.data) return <Navigate to={next} replace />;
  if (miniApp.state === "trying") return <FullScreenSpinner />; // Telegram auto-login in flight

  const fieldError = (field: string) =>
    isApiError(mutation.error) && mutation.error.field === field ? mutation.error.message : undefined;
  const generalError =
    mutation.error && !(isApiError(mutation.error) && mutation.error.field) ? mutation.error.message : null;

  // Inside Telegram, logging into an existing account also links this
  // Telegram to it, so next time the Mini App logs in by itself.
  const linkAfterLogin = () => {
    const initData = miniAppInitData();
    if (!initData) return;
    linkWebApp.mutate(initData, {
      onSuccess: () => toast("Telegram привязан к аккаунту"),
      onError: (err) => toast(err.message),
    });
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (mode === "login") login.mutate({ email: form.email, password: form.password }, { onSuccess: linkAfterLogin });
    else register.mutate(form, { onSuccess: linkAfterLogin });
  };

  const switchMode = () => {
    login.reset();
    register.reset();
    setMode((m) => (m === "login" ? "register" : "login"));
  };

  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [key]: e.target.value }));

  const bot = config.data?.telegramBot;

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

      {miniApp.state === "unlinked" && <MiniAppNewAccount onCreate={miniApp.create} creating={miniApp.creating} />}

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

      {bot && !isMiniApp && <TelegramLogin />}

      <p className="mt-6 text-center text-sm text-muted">
        {mode === "login" ? "Нет аккаунта?" : "Уже есть аккаунт?"}{" "}
        <button onClick={switchMode} className="font-semibold text-accent">
          {mode === "login" ? "Зарегистрироваться" : "Войти"}
        </button>
      </p>
    </main>
  );
}

/**
 * Inside the Telegram Mini App: try to log in with Telegram's signed data
 * right away. If this Telegram isn't linked to an account yet, the page
 * offers to create one — or to log in with a password, which links it.
 */
function useMiniAppLogin(loggedOut: boolean) {
  const webapp = useTelegramWebAppLogin();
  const [state, setState] = useState<"idle" | "trying" | "unlinked">(isMiniApp ? "trying" : "idle");
  const tried = useRef(false);

  useEffect(() => {
    const initData = miniAppInitData();
    if (!initData || !loggedOut || tried.current) return;
    tried.current = true;
    webapp.mutate(
      { initData },
      {
        onError: (err) => {
          setState(isApiError(err) && err.code === "telegram_not_linked" ? "unlinked" : "idle");
          if (!(isApiError(err) && err.code === "telegram_not_linked")) toast(err.message);
        },
      },
    );
  }, [loggedOut, webapp]);

  const create = () => {
    const initData = miniAppInitData();
    if (initData) webapp.mutate({ initData, create: true }, { onError: (err) => toast(err.message) });
  };
  return { state, create, creating: webapp.isPending };
}

function MiniAppNewAccount({ onCreate, creating }: { onCreate: () => void; creating: boolean }) {
  return (
    <Card className="mb-6">
      <p className="mb-1 font-semibold">Впервые здесь?</p>
      <p className="mb-4 text-sm text-muted">Создадим аккаунт по вашему Telegram — без email и пароля.</p>
      <Button block onClick={onCreate} loading={creating}>
        <Send className="size-4" /> Начать с Telegram
      </Button>
      <p className="mt-4 text-center text-xs text-muted">Уже есть аккаунт? Войдите ниже — Telegram привяжется к нему.</p>
    </Card>
  );
}

type LoginAttempt = Schemas["TelegramLoginStart"];

/**
 * "Войти через Telegram": opens the bot, which asks to confirm, while this
 * page polls the server until the confirmation arrives.
 */
function TelegramLogin() {
  const qc = useQueryClient();
  const [attempt, setAttempt] = useState<LoginAttempt | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);

  const start = async () => {
    setError(null);
    setStarting(true);
    try {
      const a = await unwrap(api.POST("/auth/telegram/start"));
      setAttempt(a);
      openTelegramLink(a.url);
    } catch (err) {
      setError(isApiError(err) ? err.message : "Не получилось начать вход");
    } finally {
      setStarting(false);
    }
  };

  useEffect(() => {
    if (!attempt) return;
    const timer = setInterval(async () => {
      if (Date.now() > new Date(attempt.expiresAt).getTime()) {
        setAttempt(null);
        setError("Время на подтверждение вышло. Попробуйте ещё раз.");
        return;
      }
      try {
        const { data, response } = await api.POST("/auth/telegram/complete", { body: { token: attempt.token } });
        if (response.status === 200 && data && "user" in data) {
          qc.setQueryData(keys.me, data); // AuthPage redirects once "me" is known
        } else if (response.status === 410) {
          setAttempt(null);
          setError("Вход отменён в Telegram.");
        }
      } catch {
        // Network hiccup: just try again on the next tick.
      }
    }, 2000);
    return () => clearInterval(timer);
  }, [attempt, qc]);

  return (
    <div className="mt-6">
      <div className="mb-4 flex items-center gap-3 text-xs text-muted">
        <span className="h-px flex-1 bg-line" /> или <span className="h-px flex-1 bg-line" />
      </div>
      {attempt ? (
        <Card className="text-center">
          <p className="font-semibold">Подтвердите вход в Telegram</p>
          <p className="mt-1 mb-4 text-sm text-muted">Бот спросит «Войти в приложение?» — нажмите «Да, это я».</p>
          <div className="flex justify-center gap-2">
            <Button variant="secondary" onClick={() => openTelegramLink(attempt.url)}>
              Открыть Telegram
            </Button>
            <Button variant="ghost" onClick={() => setAttempt(null)}>
              Отмена
            </Button>
          </div>
        </Card>
      ) : (
        <Button variant="secondary" block onClick={() => void start()} loading={starting}>
          <Send className="size-4" /> Войти через Telegram
        </Button>
      )}
      <ErrorBanner message={error} />
    </div>
  );
}
