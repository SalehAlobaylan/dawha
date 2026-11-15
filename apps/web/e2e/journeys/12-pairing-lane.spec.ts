import { randomUUID } from "node:crypto";
import { expect, test } from "@playwright/test";

import { API_URL, executeSql, grantPlatformRole, loginThroughUi, newAccount, registerThroughApi, sessionCookieHeader, waitForApi } from "../fixtures";

/**
 * Journey 12: complete the pairing lane end to end.
 *
 * A moderator works a seeded machine proposal in the browser from the atlas
 * inspector: the inspector lists it with its measured components, the reviewer
 * writes the note on the card itself and chooses **link**, and the crosswalk
 * row that choice created is then visible in the same inspector as a read-only
 * external identifier. There is no separate pairing page.
 * Cleanup runs through the same synthetic-actor sweep as the other journeys.
 */
test("a moderator decides a pairing and the crosswalk reaches the inspector", async ({ page, request }) => {
  await waitForApi(request);
  await page.context().clearCookies();

  const account = newAccount("pairing-lane");
  await registerThroughApi(request, account);
  await grantPlatformRole(account.email, "moderator");
  const userOf = (email: string) => `(SELECT id FROM users WHERE email = '${email}')`;

  const suffix = randomUUID().slice(0, 8);
  const externalKey = `Q${Math.floor(100000000 + Math.random() * 899999999)}`;
  const occurrence = `${externalKey}::يثرب-${suffix}`;
  const sourceID = randomUUID();
  const placeID = "20000000-0000-0000-0000-000000000001"; // الرياض
  const candidateID = randomUUID();

  await executeSql(`INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by) VALUES ($1, 'مصدر المقارن التجريبي', 'website', 'public', jsonb_build_object('license', 'CC0'), ${userOf(account.email)})`, [sourceID]);
  await executeSql(`INSERT INTO external_place_candidates (id, source_id, external_key, occurrence_id, our_place_id, normalized_candidates, coordinates, name_similarity, distance_km, rank, coordinate_delta_km, status)
    VALUES ($1, $2, $3, $4, $5, 'يثرب', ST_SetSRID(ST_MakePoint(46.6753, 24.7136), 4326), 0.9, 0.2, 1, 0.2, 'suggested')`, [candidateID, sourceID, externalKey, occurrence, placeID]);

  await loginThroughUi(page, account);
  const cookie = await sessionCookieHeader(page);

  // The list endpoint sees only the lane the moderator asked for.
  const listed = await request.get(`${API_URL}/api/v1/admin/place-pairings?status=suggested`, { headers: { cookie } });
  expect(listed.status(), await listed.text()).toBe(200);
  const listedBody = (await listed.json()) as { candidates: Array<{ id: string; externalKey: string }> };
  expect(listedBody.candidates.some((item) => item.id === candidateID && item.externalKey === externalKey)).toBe(true);

  // The inspector renders the proposal for the selected place and accepts
  // the note the reviewer typed; no separate pairing page exists.
  await page.goto("/places");
  await page.locator(".atlas-row").filter({ hasText: "الرياض" }).first().click();
  const externalIds = page.getByLabel("فاحصة الإشارة").getByLabel("معرّفات خارجية");
  const card = externalIds.locator(".pairing-card").filter({ hasText: externalKey }).first();
  await expect(card).toBeVisible();
  await expect(card).toContainText("يثرب");
  await expect(card.locator('textarea[aria-label*="Q"]')).toBeVisible();
  await card.locator('textarea[aria-label*="Q"]').fill("تحققنا من Wikidata (CC0).");
  await card.getByRole("button", { name: "ربط" }).click();
  await expect(card).toHaveCount(0);

  // The decision trail has the action the reviewer chose, and nothing else.
  const history = await request.get(`${API_URL}/api/v1/admin/place-pairings/${candidateID}/history`, { headers: { cookie } });
  expect(history.status(), await history.text()).toBe(200);
  const historyBody = (await history.json()) as { history: Array<{ action: string; fromStatus: string; toStatus: string }> };
  expect(historyBody.history).toHaveLength(1);
  expect(historyBody.history[0]).toEqual(expect.objectContaining({ action: "link", fromStatus: "suggested", toStatus: "accepted" }));

  // The same row appears as a fact only after the review — read-only, in the
  // atlas inspector next to the source list.
  await expect(externalIds).toContainText(externalKey);
});
