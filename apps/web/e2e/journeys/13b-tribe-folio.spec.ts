import { randomUUID } from "node:crypto";
import { expect, test } from "@playwright/test";

import { API_URL, WEB_ORIGIN, executeSql, grantPlatformRole, loginThroughApi, loginThroughUi, newAccount, registerThroughApi, waitForApi } from "../fixtures";

/**
 * Journey 13b: the tribal dossier's receipt folio (plan 031-A).
 *
 * One tribe, receipts through two sources: one public, one private. The plain
 * reader's folio shows only what they may read; the moderator opens the
 * needs-review band and sees the same records one level deeper. Everything is
 * asserted relatively: elements planted by this journey, beside the ids it
 * planted, never against a global count.
 */
test("the tribal dossier shows receipts with chips and keeps private sources invisible", async ({ page, request }) => {
  await waitForApi(request);
  await page.context().clearCookies();

  const owner = newAccount("tribe-folio-owner");
  const reader = newAccount("tribe-folio-reader");
  const moderator = newAccount("tribe-folio-moderator");
  await registerThroughApi(request, owner);
  await registerThroughApi(request, reader);
  await registerThroughApi(request, moderator);
  await grantPlatformRole(moderator.email, "moderator");

  const userOf = (email: string) => `(SELECT id FROM users WHERE email = '${email}')`;
  const tribeID = randomUUID();
  const placeID = randomUUID();
  const publicSource = randomUUID();
  const privateSource = randomUUID();
  const publicPassage = randomUUID();
  const privatePassage = randomUUID();

  await executeSql(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
    VALUES ($1, 'لوريمر ١٩٠٨ (رحلة ١٤)', 'book', 'public', ${userOf(owner.email)})`, [publicSource]);
  await executeSql(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by)
    VALUES ($1, 'مصدر رحلة ١٤ الخصوصي', 'book', 'private', ${userOf(owner.email)})`, [privateSource]);
  await executeSql(`INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, visibility, created_by)
    VALUES ($1, 'قبيلة الرحلة', 'قبيله الرحله', 'public', ${userOf(owner.email)})`, [tribeID]);
  await executeSql(`INSERT INTO tribe_aliases (tribe_id, value_ar, normalized_value_ar)
    VALUES ($1, 'أهل الرحلة', 'اهل الرحله')`, [tribeID]);
  await executeSql(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility, created_by)
    VALUES ($1, 'دار الرحلة', 'دار الرحله', 'region', ST_SetSRID(ST_MakePoint(41.5, 24.5), 4326), 'public', ${userOf(owner.email)})`, [placeID]);

  await executeSql(`INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
    VALUES ($1, $2, 1, 77, 'مدخل الرحلة', 'شاهد الرحلة العمومي عن القبيلة', 'شاهد الرحلة العمومي عن القبيلة')`, [publicPassage, publicSource]);
  await executeSql(`INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
    VALUES ($1, $2, 1, 5, 'الرحلة — أزل', 'شاهد الرحلة الخصوصي عن القبيلة', 'شاهد الرحلة الخصوصي عن القبيلة')`, [privatePassage, privateSource]);

  const publicAssociation = randomUUID();
  await executeSql(`INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, status, certainty, source_id, created_by)
    VALUES ($1, 'tribe', $2, $3, 'originated_in', 'documented', 'approximate', $4, ${userOf(owner.email)})`,
    [publicAssociation, tribeID, placeID, publicSource]);
  const pendingAssociation = randomUUID();
  await executeSql(`INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, status, certainty, source_id, created_by)
    VALUES ($1, 'tribe', $2, $3, 'documented_in', 'documented', 'approximate', $4, ${userOf(owner.email)})`,
    [pendingAssociation, tribeID, placeID, publicSource]);

  await executeSql(`INSERT INTO spatial_evidence (id, geographic_association_id, source_passage_id, review_status, created_by)
    VALUES ($1, $2, $3, 'accepted', ${userOf(owner.email)})`, [randomUUID(), publicAssociation, publicPassage]);
  await executeSql(`INSERT INTO spatial_evidence (id, geographic_association_id, source_passage_id, review_status, created_by)
    VALUES ($1, $2, $3, 'needs_review', ${userOf(owner.email)})`, [randomUUID(), pendingAssociation, publicPassage]);
  await executeSql(`INSERT INTO spatial_evidence (id, geographic_association_id, source_passage_id, review_status, created_by)
    VALUES ($1, $2, $3, 'accepted', ${userOf(owner.email)})`, [randomUUID(), publicAssociation, privatePassage]);

  // The API answers the plain reader one accepted receipt; the private
  // source's quote is not laundered through the public tribe.
  const readerCookie = await loginThroughApi(request, reader.email, reader.password);
  const readerFolio = await request.get(`${API_URL}/api/v1/tribes/${tribeID}/evidence`, { headers: { cookie: readerCookie } });
  expect(readerFolio.status(), await readerFolio.text()).toBe(200);
  const readerBody = (await readerFolio.json()) as { status: string; evidence: Array<{ sourceTitle?: string; status: string }> };
  expect(readerBody.status).toBe("accepted");
  expect(readerBody.evidence).toHaveLength(1);
  if (readerBody.evidence[0].sourceTitle?.includes("الخصوصي")) {
    throw new Error("the private source's receipt leaked to a plain reader");
  }

  // The moderation lane refuses what lacks the role, and answers the
  // moderator with the deeper dossier.
  const modCookie = await loginThroughApi(request, moderator.email, moderator.password);
  const readerThroughMod = await request.get(`${API_URL}/api/v1/tribes/${tribeID}/evidence?include=needs_review`, { headers: { cookie: readerCookie } });
  expect(readerThroughMod.status()).toBe(403);
  const moderated = await request.get(`${API_URL}/api/v1/tribes/${tribeID}/evidence?include=needs_review`, { headers: { cookie: modCookie } });
  expect(moderated.status(), await moderated.text()).toBe(200);
  const moderatedBody = (await moderated.json()) as { status: string; evidence: unknown[] };
  expect(moderatedBody.status).toBe("accepted+needs_review");
  expect(moderatedBody.evidence).toHaveLength(3);

  // The page: the reader's own folio is the accepted one, with its chips and
  // its stated absences; relative to this journey's rows.
  await loginThroughUi(page, reader);
  await page.goto(`${WEB_ORIGIN}/search?view=index&kind=tribes&q=${encodeURIComponent("قبيلة الرحلة")}`);
  await page.locator(".dictionary-index-item").filter({ hasText: "قبيلة الرحلة" }).first().click();
  const receipts = page.locator(".dictionary-receipts");
  await expect(receipts).toBeVisible();
  await expect(receipts.locator(".tribe-receipt")).toHaveCount(1);
  await expect(receipts).toContainText("لوريمر ١٩٠٨ (رحلة ١٤)");
  await expect(receipts).not.toContainText("الخصوصي");
  await expect(receipts).not.toContainText("إظهار ما بانتظار المراجعة");

  // The moderator sees the band toggle and, opened, the pending row beside
  // the accepted ones.
  await page.context().clearCookies();
  await loginThroughUi(page, moderator);
  await page.goto(`${WEB_ORIGIN}/search?view=index&kind=tribes&q=${encodeURIComponent("قبيلة الرحلة")}`);
  await page.locator(".dictionary-index-item").filter({ hasText: "قبيلة الرحلة" }).first().click();
  const modReceipts = page.locator(".dictionary-receipts");
  await expect(modReceipts).toBeVisible();
  await modReceipts.getByRole("button", { name: /إظهار ما بانتظار المراجعة/ }).click();
  await expect(modReceipts.locator(".tribe-receipt")).toHaveCount(3);
  await expect(modReceipts).toContainText("بانتظار المراجعة");
});
