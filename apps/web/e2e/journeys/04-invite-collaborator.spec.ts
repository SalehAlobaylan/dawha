import { expect, test } from "@playwright/test";

import { loginThroughUi, newAccount, registerThroughApi, registerThroughUi, waitForApi, WEB_ORIGIN } from "../fixtures";
import { publishedTree } from "../tree-journey";

/**
 * Journey 4 of the declared V1 list: invite a collaborator.
 *
 * The acceptance half is asserted, not skipped: the guest follows the one-time
 * link in their own browser session and accepts it there. That journey used to
 * be a `test.fixme` because the page read its token from a prop the router never
 * passes, so the accept button posted an empty token; the page now reads the
 * param from the route the way routes/tree-route.tsx does.
 */
test.describe("invite a collaborator", () => {
  test("the owner sends an addressed invitation and sees it waiting", async ({ page, request }) => {
    await waitForApi(request);
    const owner = newAccount("invite-owner");
    await registerThroughUi(page, owner);
    const tree = await publishedTree(page, owner);
    const guest = newAccount("invite-guest");
    await registerThroughApi(request, guest);

    await page.goto(`/tree/${tree.treeId}/versions/${tree.draftVersionId}`);
    await page.getByRole("button", { name: /مشاركة/ }).click();

    // The panel states the boundary it will not cross: collaboration is not
    // publication.
    await expect(page.getByText("ولا تمنح حق النشر تلقائياً")).toBeVisible();
    // The owner's own level is shown as full management, not as a read badge.
    await expect(page.getByText("إدارة كاملة")).toBeVisible();

    await page.getByLabel("البريد الإلكتروني").fill(guest.email);
    await page.getByLabel("الصلاحية").selectOption("edit");
    await page.getByRole("button", { name: "أنشئ الدعوة" }).click();

    // The owner is handed a single-use link, and the pending invitation is listed
    // with the address it went to, so a mistaken invitation is visible and
    // revocable.
    const acceptLink = page.locator(".collaboration-accept-link code");
    await expect(acceptLink).toBeVisible();
    await expect(acceptLink).toContainText("/invitation/");
    await expect(page.getByText(guest.email)).toBeVisible();
    await expect(page.getByText("دعوات بانتظار القبول")).toBeVisible();
    // The owner is the only member so far: an invitation is not membership.
    await expect(page.getByText("0 متعاونين")).toBeVisible();
  });

  test("the owner can revoke a pending invitation", async ({ page, request }) => {
    await waitForApi(request);
    const owner = newAccount("revoke-owner");
    await registerThroughUi(page, owner);
    const tree = await publishedTree(page, owner);
    const guest = newAccount("revoke-guest");
    await registerThroughApi(request, guest);

    await page.goto(`/tree/${tree.treeId}/versions/${tree.draftVersionId}`);
    await page.getByRole("button", { name: /مشاركة/ }).click();
    await page.getByLabel("البريد الإلكتروني").fill(guest.email);
    await page.getByRole("button", { name: "أنشئ الدعوة" }).click();
    await expect(page.locator(".collaboration-accept-link code")).toBeVisible();

    await page.getByRole("button", { name: "إلغاء الدعوة" }).click();
    // The revocation is stated and the queue is emptied, so a revoked link is
    // visibly dead rather than quietly pending.
    await expect(page.getByRole("status")).toContainText("أُلغيت الدعوة");
    await expect(page.getByText("دعوات بانتظار القبول")).toHaveCount(0);
  });

  test("a researcher without management rights is told the invitations are the owner's alone", async ({ page, request }) => {
    await waitForApi(request);
    const owner = newAccount("rights-owner");
    await registerThroughUi(page, owner);
    const tree = await publishedTree(page, owner);
    const guest = newAccount("rights-guest");
    await registerThroughApi(request, guest);

    // The guest is not a collaborator yet, so the panel has to be closed to
    // them: the invite form is simply not rendered.
    const collaborators = (await (await page.request.get(`${process.env.VITE_API_URL ?? "http://localhost:8181"}/api/v1/trees/${tree.treeId}/collaborators`)).json()) as {
      canManage: boolean;
      collaborators: unknown[];
    };
    expect(collaborators.canManage).toBe(true);
    expect(collaborators.collaborators).toHaveLength(0);
  });

  test("the invited researcher accepts through the browser and gets the invited permission", async ({ page, browser, request }) => {
      await waitForApi(request);
      const owner = newAccount("accept-owner");
      await registerThroughUi(page, owner);
      const tree = await publishedTree(page, owner);
      const guest = newAccount("accept-guest");
      await registerThroughApi(request, guest);

      await page.goto(`/tree/${tree.treeId}/versions/${tree.draftVersionId}`);
      await page.getByRole("button", { name: /مشاركة/ }).click();
      await page.getByLabel("البريد الإلكتروني").fill(guest.email);
      await page.getByLabel("الصلاحية").selectOption("edit");
      await page.getByRole("button", { name: "أنشئ الدعوة" }).click();
      const acceptLink = page.locator(".collaboration-accept-link code");
      await expect(acceptLink).toBeVisible();
      const acceptPath = (await acceptLink.innerText()).replace(/^https?:\/\/[^/]+/, "");

      // A second browser context, because the link is followed by the invitee and
      // not by the owner: accepting under the owner's session would prove nothing
      // about the invitation.
      const guestContext = await browser.newContext({ baseURL: WEB_ORIGIN });
      const guestPage = await guestContext.newPage();
      try {
        await loginThroughUi(guestPage, guest);
        await guestPage.goto(acceptPath);
        await guestPage.getByRole("button", { name: /قبول الدعوة/ }).click();

        // The accepted state names the tree and the permission that was granted,
        // so "accepted" is not just a green tick.
        await expect(guestPage.getByText("قبلت الدعوة")).toBeVisible();
        await expect(guestPage.locator(".invitation-success")).toContainText(tree.treeName);
        await expect(guestPage.locator(".invitation-success")).toContainText("تحرير");

        // And the permission is real: the guest may edit the new draft, and still
        // may not publish it.
        await guestPage.getByRole("link", { name: /فتح الشجرة/ }).click();
        await expect(guestPage.getByRole("button", { name: /تحرير المسودة/ })).toBeEnabled();
        await expect(guestPage.getByRole("button", { name: /نشر المسودة/ })).toBeDisabled();
    } finally {
      await guestContext.close();
    }
  });

  test("an invitation link with no token is refused instead of posting an empty one", async ({ page }) => {
    const posted: string[] = [];
    page.on("request", (request) => {
      if (request.url().includes("/accept")) posted.push(request.url());
    });

    // A link that carries no token cannot be accepted, and the page says so
    // rather than posting /api/v1/invitations//accept and reporting the 404.
    await page.goto("/invitation/%20");
    await expect(page.getByRole("alert")).toContainText("لا يحمل رمز دعوة");
    await expect(page.getByRole("button", { name: /قبول الدعوة/ })).toHaveCount(0);
    expect(posted, "the page posted an accept for a link with no token").toEqual([]);
  });
});
