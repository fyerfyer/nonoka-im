import { test, expect } from "@playwright/test";
import { SHARED_STATE } from "./shared-state";

test.describe("workspace empty states", () => {
  test.use({ storageState: SHARED_STATE });
  test("nav rail reaches chat, agents, approvals and developer", async ({
    page,
  }) => {
    await page.goto("/chat");
    const rail = page.getByTestId("workspace-rail");

    await expect(
      page.getByRole("heading", { name: "No conversations yet" })
    ).toBeVisible();

    await rail.getByRole("link", { name: "Agents" }).click();
    await expect(page).toHaveURL(/\/agents$/);
    await expect(page.getByText("No agents yet")).toBeVisible();

    await rail.getByRole("link", { name: "Approvals" }).click();
    await expect(page).toHaveURL(/\/approvals$/);
    await expect(page.getByText("You're all caught up")).toBeVisible();

    await rail.getByRole("link", { name: "Developer" }).click();
    await expect(page).toHaveURL(/\/developer$/);
    await expect(page.getByText("No external bots yet.")).toBeVisible();

    await rail.getByRole("link", { name: "Chat" }).click();
    await expect(page).toHaveURL(/\/chat$/);
    await expect(page.getByText("Select a conversation")).toBeVisible();
  });

  test("non-admin sees the access boundary in the operator console", async ({
    page,
  }) => {
    await page.goto("/admin");
    await expect(
      page.getByText("Platform admin access required")
    ).toBeVisible();
  });
});
