// Small design-system primitives. Screens compose these instead of styling
// raw <button>/<input>, so the look stays consistent and is changed here.

import { Loader2 } from "lucide-react";
import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from "react";
import { useId } from "react";

type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";

const buttonStyles: Record<ButtonVariant, string> = {
  primary: "bg-accent text-accent-ink hover:brightness-105 active:brightness-95 shadow-sm",
  secondary: "bg-surface text-ink border border-line hover:bg-line/40",
  ghost: "text-ink hover:bg-line/50",
  danger: "text-danger hover:bg-danger/10",
};

export function Button({
  variant = "primary",
  loading = false,
  block = false,
  className = "",
  children,
  disabled,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
  loading?: boolean;
  block?: boolean;
}) {
  return (
    <button
      {...props}
      disabled={disabled || loading}
      className={`inline-flex h-12 items-center justify-center gap-2 rounded-xl px-5 text-[15px] font-semibold transition select-none disabled:opacity-50 ${
        block ? "w-full" : ""
      } ${buttonStyles[variant]} ${className}`}
    >
      {loading && <Loader2 className="size-4 animate-spin" aria-hidden />}
      {children}
    </button>
  );
}

export function TextField({
  label,
  error,
  hint,
  className = "",
  ...props
}: InputHTMLAttributes<HTMLInputElement> & { label: string; error?: string; hint?: string }) {
  const id = useId();
  return (
    <div className={className}>
      <label htmlFor={id} className="mb-1.5 block text-sm font-medium text-muted">
        {label}
      </label>
      <input
        id={id}
        aria-invalid={!!error}
        {...props}
        className={`h-12 w-full rounded-xl border bg-surface px-4 text-base text-ink outline-none transition placeholder:text-muted/60 focus:border-accent focus:ring-4 focus:ring-accent/15 ${
          error ? "border-danger" : "border-line"
        }`}
      />
      {error ? (
        <p className="mt-1.5 text-sm text-danger">{error}</p>
      ) : hint ? (
        <p className="mt-1.5 text-sm text-muted">{hint}</p>
      ) : null}
    </div>
  );
}

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <section className={`rounded-(--radius-card) border border-line bg-surface p-5 ${className}`}>{children}</section>;
}

export function ErrorBanner({ message }: { message?: string | null }) {
  if (!message) return null;
  return (
    <div role="alert" className="rounded-xl bg-danger/10 px-4 py-3 text-sm text-danger">
      {message}
    </div>
  );
}

export function FullScreenSpinner() {
  return (
    <div className="flex min-h-dvh items-center justify-center">
      <Loader2 className="size-8 animate-spin text-muted" aria-label="Загрузка" />
    </div>
  );
}

export function Avatar({ name, className = "" }: { name: string; className?: string }) {
  // A stable pastel color per name, so people are easy to tell apart.
  let hash = 0;
  for (const ch of name) hash = (hash * 31 + ch.charCodeAt(0)) | 0;
  const hue = Math.abs(hash) % 360;
  return (
    <span
      aria-hidden
      className={`inline-flex size-10 shrink-0 items-center justify-center rounded-full text-sm font-bold text-[#1d1f1c] ${className}`}
      style={{ background: `hsl(${hue} 65% 78%)` }}
    >
      {name.trim().charAt(0).toUpperCase() || "?"}
    </span>
  );
}
