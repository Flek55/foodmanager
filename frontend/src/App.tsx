import { Navigate, Route, Routes } from "react-router";
import { AppShell } from "./components/AppShell";
import { Toaster } from "./lib/toast";
import { AuthGate } from "./components/AuthGate";
import { AuthPage } from "./pages/AuthPage";
import { CatalogPage } from "./pages/CatalogPage";
import { FamilyPage } from "./pages/FamilyPage";
import { HomePage } from "./pages/HomePage";
import { JoinPage } from "./pages/JoinPage";
import { ListPage } from "./pages/ListPage";
import { ProfilePage } from "./pages/ProfilePage";
import { WelcomePage } from "./pages/WelcomePage";

// Route map. Nesting = layout: AuthGate checks login, AppShell draws the
// header and tab bar, and the innermost route fills the middle.
export function App() {
  return (
    <>
      <AppRoutes />
      {/* Global, so toasts also show on pages outside the app shell (login). */}
      <Toaster />
    </>
  );
}

function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<AuthPage />} />
      <Route path="/join/:code" element={<JoinPage />} />

      <Route element={<AuthGate requireHousehold={false} />}>
        <Route path="/welcome" element={<WelcomePage />} />
      </Route>

      <Route element={<AuthGate />}>
        <Route element={<AppShell />}>
          <Route index element={<ListPage />} />
          <Route path="/home" element={<HomePage />} />
          <Route path="/family" element={<FamilyPage />} />
          <Route path="/catalog" element={<CatalogPage />} />
          <Route path="/profile" element={<ProfilePage />} />
        </Route>
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
