import { useState, useEffect } from "react";
import { useAuth } from "./hooks/useAuth";
import { AuthLayout } from "./components/AuthLayout";
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
      <AuthLayout>
        <div className="backdrop-blur-2xl bg-white/90 rounded-3xl shadow-2xl shadow-black/10 p-8 sm:p-10 border border-white/20 text-center">
          <div className="flex items-center justify-center gap-3 text-gray-500">
            <svg
              className="animate-spin h-5 w-5"
              viewBox="0 0 24 24"
              fill="none"
              aria-hidden="true"
            >
              <circle
                className="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                strokeWidth="4"
              />
              <path
                className="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
              />
            </svg>
            <span>Loading...</span>
          </div>
        </div>
      </AuthLayout>
    );
  }

  if (!isAuthenticated) {
    return (
      <AuthLayout key={page}>
        {page === "register" ? (
          <RegisterPage
            onRegisterSuccess={() => setPage("dashboard")}
            onNavigateToLogin={() => setPage("login")}
            register={auth.register}
            loading={auth.loading}
            error={auth.error}
          />
        ) : (
          <LoginPage
            onLoginSuccess={() => setPage("dashboard")}
            onNavigateToRegister={() => setPage("register")}
            login={auth.login}
            loading={auth.loading}
            error={auth.error}
          />
        )}
      </AuthLayout>
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
