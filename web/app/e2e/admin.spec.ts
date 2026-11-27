import { test, expect } from "@playwright/test";
import { E2E_PASSWORD, login } from "./helpers";

test.describe("operator console", () => {
  test("admin sees gateway, model, MCP and DLQ tabs", async ({ page }) => {
    const username = process.env.E2E_ADMIN_USERNAME;
    if (!username) {
      test.skip(
        true,
        "Set E2E_ADMIN_USERNAME/E2E_ADMIN_PASSWORD and AGENT_ADMIN_USER_IDS to exercise the admin console."
      );
      return;
    }
    await login(page, username, process.env.E2E_ADMIN_PASSWORD || E2E_PASSWORD);
    await page.goto("/admin?tab=overview");

    await expect(page.getByText("Gateway registry")).toBeVisible();
    await expect(page.getByText("Live gateways")).toBeVisible();

    await page.getByRole("button", { name: "Models" }).click();
    await expect(
      page.getByRole("heading", { name: "Model catalog" })
    ).toBeVisible();

    await page.getByRole("button", { name: "MCP" }).click();
    await expect(
      page.getByRole("heading", { name: "MCP servers" })
    ).toBeVisible();

    await page.getByRole("button", { name: "Skills" }).click();
    await expect(
      page.getByRole("heading", { name: "Registered skills" })
    ).toBeVisible();

    await page.getByRole("button", { name: "DLQ" }).click();
    await expect(
      page.getByText("Agent trigger dead letters")
    ).toBeVisible();
  });
});
