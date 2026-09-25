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
