import { expect, test } from "@playwright/test";

import { API_URL, newAccount, registerThroughApi, registerThroughUi, waitForApi } from "../fixtures";
import { publishedTree } from "../tree-journey";

/**
 * Journeys 5 and 6 of the declared V1 list: submit a public suggestion on a
 * published interpretation, and review it.
 *
 * A public suggestion is the one place a signed-out reader can change the
 * record, so both halves are covered: a stranger can submit, and only somebody
 * with a permission on the tree can decide.
 */
test.describe("submit and review a public suggestion", () => {
  test("a stranger proposes a correction and the owner decides it", async ({ page, browser, request }) => {
    await waitForApi(request);
    const owner = newAccount("suggest-owner");
    await registerThroughUi(page, owner);
    const tree = await publishedTree(page, owner);

    // A different browser, so the contributor is genuinely anonymous.
    const strangerContext = await browser.newContext();
    const strangerPage = await strangerContext.newPage();
    const correction = `الاسم مختصر في هذا الموضع: ${tree.firstPersonName} بن سعد، وليس "سعد بن عامر".`;
    try {
      await strangerPage.goto(`/tree/${tree.treeId}/versions/${tree.publishedVersionId}`);
      // The suggestion panel is only offered for a published version of a public
      // tree, and it names the person the proposal is about.
      const panel = strangerPage.locator(".suggestion-panel");
      await expect(panel).toBeVisible();
      // The panel states the boundary: a proposal never edits the version, it
      // waits for somebody with a permission on the tree.
      await expect(panel.getByText("لن تغيّر نسخة الشجرة")).toBeVisible();
      await strangerPage.getByPlaceholder("اكتب الفقرة أو التصحيح الذي تقترحه، مع أي سياق يساعد المراجع.").fill(correction);
      await strangerPage.getByRole("button", { name: /أرسل المقترح/ }).click();
      // The confirmation says the text arrived as written, which is the promise
      // the panel makes above the form.
      await expect(strangerPage.getByRole("status")).toContainText("وصل المقترح كما كتبته");
      await expect(strangerPage.getByRole("status")).toContainText("طابور المراجعة");
    } finally {
      await strangerContext.close();
    }

    // The owner opens the review queue and sees the proposal waiting.
    await page.goto(`/tree/${tree.treeId}/versions/${tree.publishedVersionId}`);
    const reviewSection = page.locator(".suggestion-review-section");
    await expect(reviewSection).toBeVisible();
    await expect(reviewSection.getByText("1 مقترحاً بانتظار القرار")).toBeVisible();
    await expect(reviewSection.getByText(correction)).toBeVisible();

    await reviewSection.getByLabel("ملاحظة القرار").fill("تم التحقق من السجل الممسوح.");
    await reviewSection.getByRole("button", { name: /^قبول$/ }).click();
    await expect(page.locator(".suggestion-panel-message")).toContainText("حُفظ قرار المراجعة في سجل المقترح");

    // The decision is kept with the note that explains it, and the queue no
    // longer offers it as pending.
    const suggestions = (await (await page.request.get(`${API_URL}/api/v1/trees/${tree.treeId}/suggestions?status=accepted`)).json()) as {
      items?: { textAr: string; status: string; reviews: { noteAr: string; decision: string }[] }[];
    };
    const decided = (suggestions.items ?? []).find((item) => item.textAr === correction);
    expect(decided, "the accepted suggestion is not readable").toBeTruthy();
    expect(decided!.status).toBe("accepted");
    expect(decided!.reviews[0]?.noteAr).toBe("تم التحقق من السجل الممسوح.");
    expect(decided!.reviews[0]?.decision).toBe("accepted");
    await expect(reviewSection.getByText("0 مقترحاً بانتظار القرار")).toBeVisible();
  });

  test("a signed-out reader sees the form but cannot decide anything", async ({ page, request }) => {
    await waitForApi(request);
    const owner = newAccount("suggest-gate-owner");
    await registerThroughUi(page, owner);
    const tree = await publishedTree(page, owner);

    // A second registered researcher with no permission on the tree may propose
    // but is offered no queue to review: the panel is not rendered for them.
    const outsider = newAccount("suggest-outsider");
    await registerThroughApi(request, outsider);
    const outsiderContext = await page.context().browser()!.newContext();
    const outsiderPage = await outsiderContext.newPage();
    try {
      await outsiderPage.goto("/login");
      await outsiderPage.getByLabel("البريد الإلكتروني").fill(outsider.email);
      await outsiderPage.getByLabel("كلمة المرور", { exact: true }).fill(outsider.password);
      await outsiderPage.getByRole("button", { name: /دخول إلى دَوْحة/ }).click();
      await outsiderPage.waitForURL((url) => !url.pathname.startsWith("/login"));
      await outsiderPage.goto(`/tree/${tree.treeId}/versions/${tree.publishedVersionId}`);

      await expect(outsiderPage.getByRole("button", { name: /أرسل المقترح/ })).toBeVisible();
      await expect(outsiderPage.locator(".suggestion-review-section")).toHaveCount(0);
      await expect(outsiderPage.getByText("طابور المراجعة")).toHaveCount(0);
    } finally {
      await outsiderContext.close();
    }
  });
});
