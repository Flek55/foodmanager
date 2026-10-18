// Running as a Telegram Mini App.
//
// Telegram opens the app with launch parameters in the URL hash:
//   #tgWebAppData=<signed init data>&tgWebAppVersion=…&tgWebAppPlatform=…
// tgWebAppData proves to our server who the Telegram user is (the server
// checks its signature). We grab it on first load — before the router
// rewrites the URL — and keep it for this session.

const KEY = "fm.tgInitData";

function captureInitData(): string | null {
  try {
    const hash = new URLSearchParams(window.location.hash.slice(1));
    const fromHash = hash.get("tgWebAppData");
    if (fromHash) {
      sessionStorage.setItem(KEY, fromHash);
      return fromHash;
    }
    return sessionStorage.getItem(KEY);
  } catch {
    return null;
  }
}

const initData = captureInitData();

/** Signed init data when running inside Telegram, otherwise null. */
export function miniAppInitData(): string | null {
  return initData;
}

export const isMiniApp = initData !== null;

type TelegramWebApp = { ready: () => void; expand: () => void; openTelegramLink?: (url: string) => void };
declare global {
  interface Window {
    Telegram?: { WebApp?: TelegramWebApp };
  }
}

/**
 * Inside Telegram, loads Telegram's script and tells it the app is ready
 * (hides the loading placeholder, opens full height). Outside Telegram it
 * does nothing — no external requests for regular users.
 */
export function initMiniApp() {
  if (!isMiniApp) return;
  const script = document.createElement("script");
  script.src = "https://telegram.org/js/telegram-web-app.js";
  script.async = true;
  script.onload = () => {
    window.Telegram?.WebApp?.ready();
    window.Telegram?.WebApp?.expand();
  };
  document.head.appendChild(script);
}

/** Opens a t.me link: inside Telegram without leaving it, otherwise in a new tab/app. */
export function openTelegramLink(url: string) {
  const webApp = window.Telegram?.WebApp;
  if (webApp?.openTelegramLink) webApp.openTelegramLink(url);
  else window.open(url, "_blank", "noopener");
}
