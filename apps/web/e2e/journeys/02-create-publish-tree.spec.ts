import { expect, test } from "@playwright/test";

import { API_URL, newAccount, registerThroughUi, waitForApi } from "../fixtures";
import { publishedTree } from "../tree-journey";

/**
 * Journeys 2 and 3 of the declared V1 list: create a tree and publish it.
 */

test.describe("create and publish a tree", () => {
  test("a researcher creates a public draft and publishes it as version 1", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("tree");
    await registerThroughUi(page, account);

    const created = await publishedTree(page, account);

    // The API agrees the tree is public and has exactly one published version
    // beside the new draft, so the assertions above are not the only evidence.
    const detail = (await (await page.request.get(`${API_URL}/api/v1/trees/${created.treeId}`)).json()) as {
      tree: { visibility: string; name: string };
      versions: { number: number; state: string }[];
    };
    expect(detail.tree.visibility).toBe("public");
    expect(detail.tree.name).toBe(created.treeName);
    expect(detail.versions.filter((version) => version.state === "published")).toHaveLength(1);
    expect(detail.versions.filter((version) => version.state === "draft")).toHaveLength(1);
    expect(detail.versions.find((version) => version.state === "published")?.number).toBe(1);

    // A published version is immutable, so the reader is told so in the UI.
    await page.goto(`/tree/${created.treeId}/versions/${created.publishedVersionId}`);
    // Both the canvas and the version strip say the sealed version is read-only.
    await expect(page.getByText("هذه النسخة للقراءة فقط")).toBeVisible();
    await expect(page.getByText("ليست تفسيراً منشوراً حالياً")).toBeVisible();
    await expect(page.getByRole("button", { name: /نشر المسودة/ })).toBeDisabled();
    await expect(page.getByRole("button", { name: /تحرير المسودة/ })).toBeDisabled();
    // Reading the published version marks it as the preview the reader opened.
    await expect(page.getByRole("list", { name: "نسخ الشجرة" }).getByText("نسخة معاينة")).toBeVisible();
  });

  test("the publish banner names the version that was published, not the draft it opened", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("publish-banner");
    await registerThroughUi(page, account);

    await page.goto("/tree");
    await page.getByRole("button", { name: "شجرة جديدة" }).click();
    await page.getByLabel("اسم الشجرة").fill(`شجرة ${account.displayName}`);
    await page.getByLabel("الظهور").selectOption("public");
    await page.getByRole("button", { name: /احفظ كمسودة/ }).click();
    // The draft that is about to be published is version 1, and the page says so
    // before anything is published.
    await expect(page.getByText("النسخة 1 من 1")).toBeVisible();

    await page.getByRole("button", { name: /نشر المسودة/ }).click();

    // The banner names the version that was published: 1. The version the API
    // selected in its response is the draft it opened, so reading the number from
    // there is how this used to announce "نُشرت النسخة 2" for a version 1 publish.
    const banner = page.locator(".tree-action-message");
    await expect(banner).toContainText("نُشرت النسخة 1");
    await expect(banner).not.toContainText("نُشرت النسخة 2");
    // And the page really has moved on to the new draft, so the assertion above is
    // not passing on a number that happens to be stale.
    await expect(page.getByText("النسخة 2 من 2")).toBeVisible();
  });

  test("a signed-out visitor cannot create a tree", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/tree");
    await page.getByRole("button", { name: "شجرة جديدة" }).click();
    await page.getByLabel("اسم الشجرة").fill("شجرة زائر");
    await page.getByRole("button", { name: /احفظ كمسودة/ }).click();

    // The refusal has to be visible: a silent no-op would read as success.
    await expect(page.getByRole("status")).toContainText(/سجّل? الدخول/);
    // And nothing was written: the tree selector still offers only what existed
    // before the attempt.
    await expect(page.getByRole("combobox", { name: "اختيار الشجرة" }).getByText("شجرة زائر")).toHaveCount(0);
  });
});
