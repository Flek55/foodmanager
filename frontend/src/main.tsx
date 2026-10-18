import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { isApiError } from "./api/client";
import { keys } from "./api/hooks";
import { App } from "./App";
import { captureInstallPrompt } from "./lib/install";
import { persistQueries, restoreQueries } from "./lib/persist";
import { toast } from "./lib/toast";
import { configureSync } from "./sync/engine";
import "./index.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Retry flaky network once, but never retry 4xx: they won't fix themselves.
      retry: (count, err) => count < 1 && !(isApiError(err) && err.status >= 400 && err.status < 500),
      refetchOnWindowFocus: true,
    },
  },
});

// Who am I and which households I'm in: restored from localStorage before
// the first render, so the app opens offline.
restoreQueries(queryClient);
persistQueries(queryClient);

configureSync({
  onDropped: (op, err) => {
    const what = op.kind === "add" ? `«${op.name}»` : "изменение";
    toast(`Не удалось сохранить ${what}: ${err.message}`);
  },
  // Session expired or removed from the household: re-check who we are;
  // AuthGate then sends the user to login or to another household.
  onAccessLost: () => void queryClient.invalidateQueries({ queryKey: keys.me }),
});

captureInstallPrompt();

if (import.meta.env.PROD && "serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch((err) => console.warn("service worker:", err));
  });
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
