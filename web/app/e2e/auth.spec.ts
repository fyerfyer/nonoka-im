import { test, expect } from "@playwright/test";
import { login, logout, register, uniqueUsername } from "./helpers";

test.describe("authentication", () => {
  test("register, reload session and log out", async ({ page }) => {
    const username = uniqueUsername("auth");
    await register(page, username);

    await expect(page.getByTestId("workspace-rail")).toBeVisible();
    await expect(page.getByRole("heading", { name: "No conversations yet" })).toBeVisible();

    await page.reload();
    await expect(page).toHaveURL(/\/chat$/);
    await expect(page.getByTestId("workspace-rail")).toBeVisible();

    await logout(page);
    await expect(page.getByTestId("auth-mode-login")).toBeVisible();
  });

  test("can sign in again after logging out", async ({ page }) => {
    const username = uniqueUsername("relogin");
    await register(page, username);
    await logout(page);
    await login(page, username);
    await expect(page.getByTestId("workspace-rail")).toBeVisible();
  });
});
