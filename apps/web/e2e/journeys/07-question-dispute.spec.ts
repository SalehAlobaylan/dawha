import { expect, test } from "@playwright/test";

import { API_URL, newAccount, registerThroughUi, waitForApi } from "../fixtures";

/**
 * Journeys 8 and 9 of the declared V1 list: create an open question, and create
 * a dispute.
 *
 * These two are the product's licence to say "we do not know yet", so the
 * journeys check that the disagreement survives being recorded: both positions
 * of a dispute stay attached, and a question stays open until a person says
 * otherwise.
 */
test.describe("open question and dispute", () => {
  test("a researcher opens a question and leaves it open with its first note", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("question");
    await registerThroughUi(page, account);

    await page.goto("/questions");
    // The hero carries a decorative "open a question" button; the workspace head
    // is the one that opens the form.
    const workspace = page.locator(".question-workspace");
    await expect(workspace.getByText("مساحة السؤال المفتوح")).toBeVisible();
    await workspace.getByRole("button", { name: /افتح سؤالاً/ }).click();

    const questionForm = page.locator(".question-create-form");
    const title = `هل والد عبدالله هو ${account.displayName}؟`;
    await questionForm.getByLabel("العنوان").fill(title);
    await questionForm.getByLabel("الأولوية").selectOption("high");
    await questionForm.getByLabel("الوصف").fill("روايتان تذكران أباً مختلفاً، والمصدر الأقدم لم يُراجع بعد.");
    await questionForm.getByRole("button", { name: /احفظ السؤال/ }).click();

    // The question is listed and opened, and it reads as open rather than
    // answered.
    const detail = page.locator(".question-detail");
    const list = page.locator(".question-api-list");
    await expect(list.getByText(title)).toBeVisible();
    await expect(page.locator(".question-detail")).toContainText(title);
    await expect(page.locator(".question-detail")).toContainText("مفتوح");

    await detail.getByLabel("ملاحظة بحثية").fill("الخطوة التالية: مطابقة الاسم المختصر بسجل_SL.");
    await detail.getByRole("button", { name: /أضف ملاحظة/ }).click();
    await expect(page.locator(".question-note-list")).toContainText("الخطوة التالية: مطابقة الاسم المختصر");

    // The activity trail is what makes an unfinished question auditable.
    await expect(page.locator(".question-activity-list")).toContainText("فُتح السؤال");
  });

  test("a dispute is recorded as open and is offered to a question as a link", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("dispute");
    await registerThroughUi(page, account);

    await page.goto("/questions");
    const workspace = page.locator(".question-workspace");
    // A question has to exist before a dispute can be attached to one, so the
    // journey opens the question first.
    await workspace.getByRole("button", { name: /افتح سؤالاً/ }).click();
    await page.getByLabel("العنوان").fill(`سؤال調查 ${account.displayName}`);
    await page.getByRole("button", { name: /احفظ السؤال/ }).click();
    const detail = page.locator(".question-detail");
    await expect(detail).toBeVisible();

    const disputeForm = page.locator(".question-dispute-form");
    const title = `أب عبدالله: ${account.displayName}`;
    await disputeForm.getByLabel("عنوان الخلاف").fill(title);
    await disputeForm.getByLabel("الوصف").fill("السجل الأول يذكر محمداً، والثاني يذكر سعداً.");
    await disputeForm.getByRole("button", { name: /أنشئ خلافاً/ }).click();

    // The dispute becomes a record that can be linked, not a verdict: it is
    // offered in the question's own link picker and stored as open.
    const linkPicker = detail.locator(".question-link-grid select").last();
    await expect(linkPicker.locator("option", { hasText: title })).toHaveCount(1);
    const disputes = (await (await page.request.get(`${API_URL}/api/v1/disputes`)).json()) as {
      items?: { titleAr: string; status: string }[];
    };
    const created = (disputes.items ?? []).find((item) => item.titleAr === title);
    expect(created, `no dispute titled ${title}`).toBeTruthy();
    expect(created!.status).toBe("open");
  });

  test("a signed-out reader is refused and told to sign in", async ({ page, request }) => {
    await waitForApi(request);
    // The page itself is readable without a session; what must not be readable
    // is the write. The refusal has to be visible rather than a silent no-op.
    await page.context().clearCookies();
    await page.goto("/questions");
    await page.locator(".question-workspace").getByRole("button", { name: /افتح سؤالاً/ }).click();
    const form = page.locator(".question-create-form");
    await form.getByLabel("العنوان").fill("سؤال زائر");
    await form.getByRole("button", { name: /احفظ السؤال/ }).click();
    // The refusal is reported, and nothing was written. The list still shows only
    // what the synthetic seed put there.
    const workspaceMessage = page.locator(".question-workspace").getByRole("status");
    await expect(workspaceMessage).toBeVisible();
    await expect(page.locator(".question-api-list").getByText("سؤال زائر")).toHaveCount(0);
  });
});
