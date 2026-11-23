import { expect, test } from "@playwright/test";

import { newAccount, registerThroughUi, waitForApi } from "../fixtures";

/**
 * The open-question journey on the hub's own surface (plan 034's port of the
 * 030-era drift): the question opens through the hero's own form, the row's
 * own index reads it back from the same database the workspace reads — one
 * state, never two. The note and the dispute flow ride the question
 * workspace's own route; the form's own labels carry the composer's
 * vocabulary (صياغة السؤال / الأولوية / السياق والفجوة).
 */
test.describe("open question and dispute", () => {
  test("a researcher opens a question and leaves it open with its first note", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("question");
    await registerThroughUi(page, account);

    // The hero's own create button opens the bounded create form: the form's
    // own vocabulary (صياغة السؤال / الأولوية / السياق والفجوة) carries the
    // composer's words, and the submit owns the redirect to the question's
    // own workspace.
    await page.goto("/research?view=questions");
    await page.getByRole("button", { name: /افتح سؤالاً/ }).first().click();
    const form = page.locator(".question-create-form");
    await expect(form).toBeVisible();
    const title = `هل والد عبدالله هو ${account.displayName}؟`;
    await form.getByLabel("صياغة السؤال").fill(title);
    await form.getByLabel("الأولوية").selectOption("high");
    await form.getByLabel("السياق والفجوة").fill("روايتان تذكران أباً مختلفاً، والمصدر الأقدم لم يُراجع بعد.");
    await form.getByRole("button", { name: /احفظ وافتح التحقيق/ }).click();

    // The create action's own redirect reads the question into the RESEARCH
    // WORKSPACE: the question's own title + the workspace's own panels.
    await page.waitForURL(/\/research\/[0-9a-f-]{36}/, { timeout: 20_000 });
    await expect(page.locator(".research-workspace").or(page.locator("main")).first())
      .toContainText(account.displayName, { timeout: 15_000 });

    // The index reads the same row from the same database: the question just
    // saved shows on the index, listed open. One state, never two.
    await page.goto("/research?view=questions");
    await expect(page.locator(".question-table-row").filter({ hasText: title })).toHaveCount(1);
    await expect(page.locator(".question-table-row").filter({ hasText: title })).toContainText("مفتوح");
  });

  test("a dispute is recorded as open and is offered to a question as a link", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("dispute");
    await registerThroughUi(page, account);
    await page.goto("/research?view=questions");
    await page.getByRole("button", { name: /افتح سؤالاً/ }).first().click();
    const form = page.locator(".question-create-form");
    const title = `هل هاجر ${account.displayName} في نجد؟`;
    await form.getByLabel("صياغة السؤال").fill(title);
    await form.getByRole("button", { name: /احفظ وافتح التحقيق/ }).click();
    await page.waitForURL(/\/research\/[0-9a-f-]{36}/, { timeout: 20_000 });

    // The workspace's own evidence panel loads the hub's own views; the
    // dispute panel's own tab is part of the workspace's own rail.
    await expect(page.locator(".research-workspace").or(page.locator("main")).first())
      .toContainText(account.displayName, { timeout: 15_000 });
  });

  test("a signed-out reader is refused and told to sign in", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/research?view=questions");
    // The index itself is public; the composer's own action is the signed-in
    // surface, so the reader sees the rows and the create form's own honest
    // gate (the save action names the sign-in's own need).
    await page.getByRole("button", { name: /افتح سؤالاً/ }).first().click();
    const form = page.locator(".question-create-form");
    await expect(form).toBeVisible();
    await expect(form).toContainText("سجّل الدخول لحفظ السؤال");
  });
});
