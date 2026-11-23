import { randomUUID } from "node:crypto";
import { expect, test } from "@playwright/test";

import { API_URL, executeSql, grantPlatformRole, loginThroughApi, loginThroughUi, newAccount, registerThroughApi, waitForApi, WEB_ORIGIN } from "../fixtures";

/**
 * Journey 13c: the dossier claim's طعن arc (plan 031-D).
 *
 * A reader challenges a tribe's documented claim from the dossier itself; the
 * claim's own chip flips to `disputed` the moment the challenge commits (the
 * flip is the row, not a second state); the reader cannot close their thread;
 * the moderator settles it with a spelled outcome and the claim's chip comes
 * back. Relative assertions only.
 */
test("a challenge disputes a claim and the moderator settles it", async ({ page, request }) => {
  await waitForApi(request);
  await page.context().clearCookies();

  const owner = newAccount("challenge-owner");
  const challenger = newAccount("challenge-reader");
  const moderator = newAccount("challenge-moderator");
  for (const account of [owner, challenger, moderator]) {
    await registerThroughApi(request, account);
  }
  await grantPlatformRole(moderator.email, "moderator");

  const userOf = (email: string) => `(SELECT id FROM users WHERE email = '${email}')`;
  const tribeID = randomUUID();
  const placeID = randomUUID();
  const sourceID = randomUUID();
  const passageID = randomUUID();

  await executeSql(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
    VALUES ($1, 'قبيلة طعن الرحلة', 'قبيله طعن الرحله', 'public', ${userOf(owner.email)})`, [tribeID]);
  await executeSql(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, visibility, created_by)
    VALUES ($1, 'موضع طعن الرحلة', 'موضع طعن الرحله', 'region', 'public', ${userOf(owner.email)})`, [placeID]);
  await executeSql(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
    VALUES ($1, 'لوريمر طعن الرحلة', 'book', 'public', ${userOf(owner.email)})`, [sourceID]);
  await executeSql(`INSERT INTO source_passages (id, source_id, sequence_number, text_ar, normalized_text_ar)
    VALUES ($1, $2, 1, 'شاهد طعن الرحلة', 'شاهد طعن الرحلة')`, [passageID, sourceID]);

  const claimID = randomUUID();
  await executeSql(`INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, status, notes_ar, created_by)
    VALUES ($1, 'tribe', $2, 'originated_in', 'place', $3, 'documented', 'مطالبة الرحلة', ${userOf(owner.email)})`,
    [claimID, tribeID, placeID]);
  await executeSql(`INSERT INTO claim_evidence (id, claim_id, source_passage_id, relation, created_by)
    VALUES ($1, $2, $3, 'supports', ${userOf(owner.email)})`, [randomUUID(), claimID, passageID]);

  // The reader opens the dossier and challenges the claim from its own row.
  await loginThroughUi(page, challenger);
  await page.goto(`${WEB_ORIGIN}/search?view=index&kind=tribes&q=${encodeURIComponent("قبيلة طعن الرحلة")}`);
  await page.locator(".dictionary-index-item").filter({ hasText: "قبيلة طعن الرحلة" }).first().click();
  const claimRow = page.locator(".dictionary-related-row").filter({ hasText: "مطالبة الرحلة" });
  await expect(claimRow).toBeVisible();
  await expect(claimRow).toContainText("موثَّق");

  await claimRow.getByRole("button", { name: /طعن/ }).first().click();
  await claimRow.getByLabel("عنوان الخلاف").fill("طعن رحلة النسبة");
  await claimRow.getByLabel("سبب الطعن").fill("المصدر الناظر يقول غير هذا.");
  await claimRow.getByRole("button", { name: /افتح الخلاف/ }).click();

  // The chip flipped with the challenge's own transaction: the same row now
  // reads disputed, on this page and behind the API alike.
  await expect(claimRow).toContainText("متنازع عليه");
  const apiClaim = await request.get(`${API_URL}/api/v1/disputes`, {});
  expect(apiClaim.status()).toBe(200);
  const disputesBody = (await apiClaim.json()) as { items: Array<{ id: string; titleAr: string; status: string }> };
  const thread = disputesBody.items.find((item) => item.titleAr === "طعن رحلة النسبة");
  expect(thread, "the challenge's thread exists").toBeTruthy();
  expect(thread?.status).toBe("open");

  // The challenger cannot close their own thread: the adjudication is not
  // theirs. The moderator settles it and the chip comes back.
  const challengerCookie = await loginThroughApi(request, challenger.email, challenger.password);
  const selfClose = await request.post(`${API_URL}/api/v1/disputes/${thread!.id}/adjudicate`, {
    headers: { cookie: challengerCookie },
    data: { claimStatus: "documented", resolutionAr: "إغلاق ذاتي خاطئ" },
  });
  expect(selfClose.status()).toBe(403);

  const modCookie = await loginThroughApi(request, moderator.email, moderator.password);
  const adjudicated = await request.post(`${API_URL}/api/v1/disputes/${thread!.id}/adjudicate`, {
    headers: { cookie: modCookie },
    data: { claimStatus: "documented", resolutionAr: "الحسم بعد قراءة الشاهد المزدوج." },
  });
  expect(adjudicated.status(), await adjudicated.text()).toBe(200);
  const adjudicatedBody = (await adjudicated.json()) as { dispute: { status: string; resolutionAr?: string }; claims: Array<{ claimId: string; status: string }> };
  expect(adjudicatedBody.dispute.status).toBe("resolved");
  expect(adjudicatedBody.dispute.resolutionAr).toContain("الحسم بعد قراءة الشاهد");
  expect(adjudicatedBody.claims.find((claim) => claim.claimId === claimID)?.status).toBe("documented");

  // The same row, re-read: the settled state travels everywhere from the row.
  await page.reload();
  const settledRow = page.locator(".dictionary-related-row").filter({ hasText: "مطالبة الرحلة" });
  await expect(settledRow).toContainText("موثَّق");
});
