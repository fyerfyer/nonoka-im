import { test, expect } from "@playwright/test";
import { sidebarWidth } from "./helpers";
import { SHARED_STATE } from "./shared-state";

test.describe("workspace shell", () => {
  test.use({ storageState: SHARED_STATE });
  test("sidebar drag, keyboard and reload persistence", async ({ page }) => {
    await page.goto("/chat");

    const separator = page.getByTestId("chat-sidebar-separator");
    await expect(separator).toBeVisible();
    const initial = await sidebarWidth(page);
    expect(initial).toBeGreaterThanOrEqual(280);
    expect(initial).toBeLessThanOrEqual(420);

    const box = await separator.boundingBox();
    if (!box) throw new Error("separator has no bounding box");
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    await page.mouse.move(box.x + box.width / 2 + 70, box.y + box.height / 2, {
      steps: 8,
    });
    await page.mouse.up();

    await expect
      .poll(() => sidebarWidth(page))
      .toBeGreaterThan(initial);
    const dragged = await sidebarWidth(page);
    expect(dragged).toBeLessThanOrEqual(420);

    await page.reload();
    await expect(page.getByTestId("conversation-list")).toBeVisible();
    await expect.poll(() => sidebarWidth(page)).toBe(dragged);

    await separator.focus();
    await page.keyboard.press("ArrowLeft");
    await expect
      .poll(() => sidebarWidth(page))
      .toBeLessThan(dragged);

    await page.keyboard.press("End");
    await expect.poll(() => sidebarWidth(page)).toBe(420);
    await page.keyboard.press("Home");
    await expect.poll(() => sidebarWidth(page)).toBe(280);
  });

  test("mobile uses single-pane navigation without the splitter", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/chat");

    await expect(page.getByTestId("mobile-tab-bar")).toBeVisible();
    await expect(page.getByTestId("workspace-rail")).toBeHidden();
    await expect(page.getByTestId("chat-sidebar-separator")).toBeHidden();

    await page
      .getByTestId("mobile-tab-bar")
      .getByRole("link", { name: "Agents" })
      .click();
    await expect(page).toHaveURL(/\/agents$/);
    await expect(page.getByText("No agents yet")).toBeVisible();
  });
});

test.describe("theme", () => {
  test.use({ storageState: SHARED_STATE });

  test("switches between light and dark and persists", async ({ page }) => {
    await page.goto("/chat");
    await page.getByRole("button", { name: "Change theme" }).click();
    await page.getByRole("menuitem", { name: "Dark" }).click();
    await expect(page.locator("html")).toHaveClass(/dark/);
    await page.reload();
    await expect(page.locator("html")).toHaveClass(/dark/);

    await page.getByRole("button", { name: "Change theme" }).click();
    await page.getByRole("menuitem", { name: "Light" }).click();
    await expect(page.locator("html")).toHaveClass(/light/);
  });
});
