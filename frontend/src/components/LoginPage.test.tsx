import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LoginPage } from "./LoginPage";

describe("LoginPage", () => {
  const defaultProps = {
    onLoginSuccess: vi.fn(),
    onNavigateToRegister: vi.fn(),
    login: vi.fn(),
    loading: false,
    error: null,
  };

  it("should render login form", () => {
    render(<LoginPage {...defaultProps} />);

    expect(screen.getByText("Invoice Maker")).toBeInTheDocument();
    expect(screen.getByLabelText("Email")).toBeInTheDocument();
    expect(screen.getByLabelText("Password")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Login" })).toBeInTheDocument();
  });

  it("should show error when fields are empty", async () => {
    render(<LoginPage {...defaultProps} />);

    fireEvent.click(screen.getByRole("button", { name: "Login" }));

    expect(await screen.findByText("Please fill in all fields")).toBeInTheDocument();
  });

  it("should call login with email and password", async () => {
    const login = vi.fn().mockResolvedValue(undefined);
    render(<LoginPage {...defaultProps} login={login} />);

    await userEvent.type(screen.getByLabelText("Email"), "test@example.com");
    await userEvent.type(screen.getByLabelText("Password"), "password123");
    fireEvent.click(screen.getByRole("button", { name: "Login" }));

    expect(login).toHaveBeenCalledWith("test@example.com", "password123");
  });

  it("should show loading state", () => {
    render(<LoginPage {...defaultProps} loading={true} />);

    expect(screen.getByRole("button", { name: "Logging in..." })).toBeDisabled();
  });

  it("should show server error", () => {
    render(<LoginPage {...defaultProps} error="Invalid credentials" />);

    expect(screen.getByText("Invalid credentials")).toBeInTheDocument();
  });

  it("should navigate to register page", async () => {
    const onNavigate = vi.fn();
    render(<LoginPage {...defaultProps} onNavigateToRegister={onNavigate} />);

    fireEvent.click(screen.getByText("Register here"));

    expect(onNavigate).toHaveBeenCalledOnce();
  });
});
