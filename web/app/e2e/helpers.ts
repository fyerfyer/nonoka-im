import { expect, type Page } from "@playwright/test";

export const E2E_PASSWORD = "e2e-password-123";

export function uniqueUsername(prefix = "e2e"): string {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
}

export async function register(page: Page, username = uniqueUsername()) {
  await page.goto("/login");
  await page.getByTestId("auth-mode-register").click();
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password", { exact: true }).fill(E2E_PASSWORD);
  await page.getByLabel("Confirm password").fill(E2E_PASSWORD);
  await page
    .locator("form")
    .getByRole("button", { name: "Create account" })
    .click();
  await expect(page).toHaveURL(/\/chat$/);
  return username;
}

export async function login(
  page: Page,
  username: string,
  password = E2E_PASSWORD
) {
  await page.goto("/login");
  await page.getByTestId("auth-mode-login").click();
  await page.getByLabel("Username").fill(username);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.locator("form").getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/chat$/);
}

export async function logout(page: Page) {
  const rail = page.getByTestId("workspace-rail");
  await rail.getByRole("button", { name: "Account menu" }).click();
  await page.getByRole("menuitem", { name: "Log out" }).click();
  await expect(page).toHaveURL(/\/login$/);
}

export async function sidebarWidth(page: Page): Promise<number> {
  const box = await page.getByTestId("conversation-list").boundingBox();
  return box?.width ?? 0;
}
