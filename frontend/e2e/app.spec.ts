import { test, expect } from "@playwright/test";

test.describe("Invoice Maker — E2E Smoke Tests", () => {
  test("login page loads correctly", async ({ page }) => {
    await page.goto("/");

    // Verify login page elements
    await expect(page.locator("h1")).toHaveText("Invoice Maker");
    await expect(page.locator("#email")).toBeVisible();
    await expect(page.locator("#password")).toBeVisible();
    await expect(page.getByRole("button", { name: "Login" })).toBeVisible();
    await expect(page.getByText("Register here")).toBeVisible();
  });

  test("login form shows error on empty submit", async ({ page }) => {
    await page.goto("/");

    await page.getByRole("button", { name: "Login" }).click();

    await expect(page.getByText("Please fill in all fields")).toBeVisible();
  });

  test("login form accepts input", async ({ page }) => {
    await page.goto("/");

    await page.fill("#email", "test@example.com");
    await page.fill("#password", "mypassword123");

    await expect(page.locator("#email")).toHaveValue("test@example.com");
    await expect(page.locator("#password")).toHaveValue("mypassword123");
  });

  test("navigates to register page", async ({ page }) => {
    await page.goto("/");

    await page.getByText("Register here").click();

    await expect(page.getByText("Create an account to get started")).toBeVisible();
    await expect(page.getByRole("button", { name: "Register" })).toBeVisible();
    await expect(page.getByText("Already have an account?")).toBeVisible();
  });

  test("navigates back to login from register", async ({ page }) => {
    await page.goto("/");

    await page.getByText("Register here").click();
    await page.getByText("Login here").click();

    await expect(page.locator("h1")).toHaveText("Invoice Maker");
  });
});
