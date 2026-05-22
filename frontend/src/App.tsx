import { useState, useEffect } from "react";
import { useAuth } from "./hooks/useAuth";
import { LoginPage } from "./components/LoginPage";
import { RegisterPage } from "./components/RegisterPage";
import { ProtectedInvoiceDashboard } from "./components/ProtectedInvoiceDashboard";

type PageType = "login" | "register" | "dashboard";

export default function App() {
  const auth = useAuth();
  const { user, isAuthenticated, loading, logout } = auth;
  const [page, setPage] = useState<PageType>("login");

  useEffect(() => {
    if (isAuthenticated && !loading) {
      setPage("dashboard");
    }
  }, [isAuthenticated, loading]);

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-100 flex items-center justify-center">
        <div className="text-center">
          <p className="text-gray-600">Loading...</p>
        </div>
      </div>
    );
  }

  if (!isAuthenticated) {
    if (page === "register") {
      return (
        <RegisterPage
          onRegisterSuccess={() => setPage("dashboard")}
          onNavigateToLogin={() => setPage("login")}
          register={auth.register}
          loading={auth.loading}
          error={auth.error}
        />
      );
    }

    return (
      <LoginPage
        onLoginSuccess={() => setPage("dashboard")}
        onNavigateToRegister={() => setPage("register")}
        login={auth.login}
        loading={auth.loading}
        error={auth.error}
      />
    );
  }

  return (
    <ProtectedInvoiceDashboard
      user={user}
      onLogout={() => {
        logout();
        setPage("login");
      }}
    />
  );
}
