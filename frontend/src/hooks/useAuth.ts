import { useState, useEffect } from "react";
import {
  User,
  AuthResponse,
  UpdateProfileRequest,
  ChangePasswordRequest,
} from "../types/auth";
import { apiFetch, ApiError } from "../utils/api";

export interface UseAuthResult {
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  loading: boolean;
  error: string | null;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string) => Promise<void>;
  logout: () => void;
  updateProfile: (email: string) => Promise<User>;
  changePassword: (
    currentPassword: string,
    newPassword: string,
  ) => Promise<void>;
}

export function useAuth(): UseAuthResult {
  const [user, setUser] = useState<User | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Initialize from localStorage
  useEffect(() => {
    const savedToken = localStorage.getItem("auth_token");
    const savedUser = localStorage.getItem("auth_user");

    if (savedToken && savedUser) {
      try {
        setToken(savedToken);
        setUser(JSON.parse(savedUser));
      } catch (e) {
        localStorage.removeItem("auth_token");
        localStorage.removeItem("auth_user");
      }
    }

    setLoading(false);
  }, []);

  const login = async (email: string, password: string): Promise<void> => {
    setLoading(true);
    setError(null);

    try {
      const response = await apiFetch<AuthResponse>("/auth/login", {
        method: "POST",
        body: JSON.stringify({ email, password }),
      });

      setToken(response.token);
      setUser(response.user);
      localStorage.setItem("auth_token", response.token);
      localStorage.setItem("auth_user", JSON.stringify(response.user));
    } catch (err) {
      const message = err instanceof ApiError ? err.message : "Login failed";
      setError(message);
      throw err;
    } finally {
      setLoading(false);
    }
  };

  const register = async (email: string, password: string): Promise<void> => {
    setLoading(true);
    setError(null);

    try {
      const response = await apiFetch<AuthResponse>("/auth/register", {
        method: "POST",
        body: JSON.stringify({ email, password }),
      });

      setToken(response.token);
      setUser(response.user);
      localStorage.setItem("auth_token", response.token);
      localStorage.setItem("auth_user", JSON.stringify(response.user));
    } catch (err) {
      const message =
        err instanceof ApiError ? err.message : "Registration failed";
      setError(message);
      throw err;
    } finally {
      setLoading(false);
    }
  };

  const logout = (): void => {
    setUser(null);
    setToken(null);
    setError(null);
    localStorage.removeItem("auth_token");
    localStorage.removeItem("auth_user");
  };

  const updateProfile = async (email: string): Promise<User> => {
    setError(null);

    try {
      const updated = await apiFetch<User>("/auth/profile", {
        method: "PUT",
        body: JSON.stringify({ email } satisfies UpdateProfileRequest),
      });

      setUser(updated);
      localStorage.setItem("auth_user", JSON.stringify(updated));
      return updated;
    } catch (err) {
      const message =
        err instanceof ApiError ? err.message : "Failed to update profile";
      setError(message);
      throw err;
    }
  };

  const changePassword = async (
    currentPassword: string,
    newPassword: string,
  ): Promise<void> => {
    setError(null);

    try {
      await apiFetch<void>("/auth/change-password", {
        method: "PUT",
        body: JSON.stringify({
          current_password: currentPassword,
          new_password: newPassword,
        } satisfies ChangePasswordRequest),
      });
    } catch (err) {
      const message =
        err instanceof ApiError ? err.message : "Failed to change password";
      setError(message);
      throw err;
    }
  };

  return {
    user,
    token,
    isAuthenticated: !!token && !!user,
    loading,
    error,
    login,
    register,
    logout,
    updateProfile,
    changePassword,
  };
}
