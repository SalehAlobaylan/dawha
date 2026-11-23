import { randomUUID } from "node:crypto";
import { expect, test } from "@playwright/test";

import { API_URL, executeSql, grantPlatformRole, loginThroughUi, newAccount, registerThroughApi, sessionCookieHeader, syntheticSourcePNG, waitForApi } from "../fixtures";

/**
 * Journey 13: the region workbench retires a tile on two independent passes.
 *
 * Two moderators work one tile of a plate's proposed labels: the first claims
 * the square in the browser, reads each crop from the plate's own pixels
 * (typing the reading, rejecting the non-label), and submits. A second reviewer
 * confirms the same readings in the browser and retires the tile. Every refusal
 * that guards two-person retirement is asserted where it happens; relative
 * assertions only — the tile and the candidates are found by the ids the
 * journey itself planted, never by a count of anyone else's rows. And the
 * arc promotes nothing: the journey's last read asserts the candidates it
 * answered carry no region geometry, because promotion is a later surface.
 */
test("a tile retires on two independent passes and never on one", async ({ page, request }) => {
  await waitForApi(request);
  await page.context().clearCookies();

  const first = newAccount("region-workbench-a");
  const second = newAccount("region-workbench-b");
  await registerThroughApi(request, first);
  await grantPlatformRole(first.email, "moderator");
  await registerThroughApi(request, second);
  await grantPlatformRole(second.email, "moderator");
  const userOf = (email: string) => `(SELECT id FROM users WHERE email = '${email}')`;

  // The source carries a real stored PNG, so the crops on the bench are the
  // plate's own pixels from the real store; the georef block mirrors the
  // 030-D shape the honesty panel reads.
  const sourceID = randomUUID();
  await executeSql(
    `INSERT INTO sources (id, title_ar, source_type, visibility, metadata, created_by)
     VALUES ($1, 'خريطة قبائل تجريبية ١٩١٦', 'archive_record', 'public',
       jsonb_build_object('license', 'public_domain',
         'georef', jsonb_build_object('method', 'graticule_labels', 'certainty', 'approximate',
           'needsReview', true, 'reviewReasons', jsonb_build_array('interior_parallels_unverified_by_ink'))),
       ${userOf(first.email)})`,
    [sourceID],
  );

  await loginThroughUi(page, first);
  const cookie = await sessionCookieHeader(page);
  const uploaded = await page.request.post(`${API_URL}/api/v1/sources/${sourceID}/files`, {
    multipart: {
      file: {
        name: "خريطة-تجريبية.png",
        mimeType: "image/png",
        buffer: syntheticSourcePNG(800, 500),
      },
    },
  });
  expect(uploaded.status(), await uploaded.text()).toBeLessThan(300);

  // Three proposals inside one tile: two readable labels and one non-label.
  const tileID = randomUUID();
  const candidates = [
    { id: randomUUID(), occurrence: "label:test:100", reading: "عنزة" },
    { id: randomUUID(), occurrence: "label:test:300", reading: "بني تميم" },
    { id: randomUUID(), occurrence: "label:test:500", reading: "" },
  ];
  for (const candidate of candidates) {
    await executeSql(
      `INSERT INTO external_region_candidates (id, source_id, plate_key, occurrence_id, crop_px, coordinates, ink_density, component_count, label_confidence, transform_rms_km, rank, status)
       VALUES ($1, $2, $3, $4, $5::jsonb, ST_SetSRID(ST_MakePoint(44.66, 26.78), 4326), 0.2, 7, 0.8, 8.56, 0, 'suggested')`,
      [candidate.id, sourceID, "journey-plate", candidate.occurrence, JSON.stringify({ x: 100 + 200 * candidates.indexOf(candidate), y: 80, w: 80, h: 40 })],
    );
  }
  await executeSql(
    `INSERT INTO region_tile_tasks (id, source_id, plate_key, tile_id, box_px, status)
     VALUES ($1, $2, 'journey-plate', 'tile:journey-plate:r0c0', '{"x":0,"y":0,"w":800,"h":500}'::jsonb, 'open')`,
    [tileID, sourceID],
  );

  // The bench: claim the tile, read two labels, reject the non-label. The
  // keyboard lane and the buttons answer identically; the journey uses both.
  await page.goto("/region-workbench");
  await page.locator(".workbench-tile").filter({ hasText: "tile:journey-plate" }).click();
  await page.getByRole("button", { name: "تبنَّ المربع" }).click();

  const submitButton = page.getByRole("button", { name: "أرسل المرتكز" });
  await expect(submitButton).toBeDisabled();
  const firstCard = page.locator(".workbench-candidate").nth(0);
  await firstCard.getByRole("textbox").fill(candidates[0].reading);
  await firstCard.getByRole("textbox").press("Enter");
  const secondCard = page.locator(".workbench-candidate").nth(1);
  await secondCard.getByRole("textbox").fill(candidates[1].reading);
  await secondCard.getByRole("textbox").press("Enter");
  await expect(firstCard.getByText("مصحَّح بقراءة")).toBeVisible();
  await expect(secondCard.getByText("مصحَّح بقراءة")).toBeVisible();
  await expect(submitButton).toBeDisabled();
  const thirdCard = page.locator(".workbench-candidate").nth(2);
  await thirdCard.getByRole("button", { name: "تأجيل" }).click();
  await expect(submitButton).toBeEnabled();
  await thirdCard.getByRole("button", { name: "إعادة فتح" }).click();
  await expect(thirdCard.getByText("بانتظار إجابتك")).toBeVisible();
  await expect(submitButton).toBeDisabled();
  await thirdCard.getByRole("button", { name: "رفض" }).click();

  // The submit unlocks only when nothing in the tile is unanswered (a defer
  // is an answer too); the wait IS the rule, so the assertion is on the
  // button, not on a sleep.
  await expect(submitButton).toBeEnabled();
  await submitButton.click();
  await expect(page.getByRole("button", { name: "ابنَ المراجعة الثانية" })).toBeVisible();

  // One pass is not a retirement: the first submitter cannot claim their own
  // tile back.
  const selfAttempt = await request.post(`${API_URL}/api/v1/admin/region-workbench/tiles/${tileID}/claim`, {
    headers: { cookie },
  });
  expect(selfAttempt.status(), "a submitter cannot own both passes").toBe(409);

  // The second reviewer sees the first pass's readings, but must answer all three.
  await page.context().clearCookies();
  await loginThroughUi(page, second);
  await page.goto("/region-workbench");
  await page.getByRole("tab", { name: "بانتظار مراجعة ثانية" }).click();
  await page.locator(".workbench-tile").filter({ hasText: "tile:journey-plate" }).click();
  await page.getByRole("button", { name: "ابنَ المراجعة الثانية" }).click();
  await expect(submitButton).toBeDisabled();
  await expect(page.getByText("بانتظار إجابتك", { exact: true })).toHaveCount(3);
  await firstCard.getByRole("button", { name: "تأكيد القراءة المعروضة" }).click();
  await secondCard.getByRole("button", { name: "تأكيد القراءة المعروضة" }).click();
  await expect(page.getByText("بانتظار إجابتك", { exact: true })).toHaveCount(1);
  await expect(submitButton).toBeDisabled();
  await thirdCard.getByRole("button", { name: "رفض" }).click();
  await expect(submitButton).toBeEnabled();
  const [submitted] = await Promise.all([
    page.waitForResponse((response) => response.url().endsWith(`/tiles/${tileID}/submit`) && response.request().method() === "POST"),
    submitButton.click(),
  ]);
  expect(submitted.status(), await submitted.text()).toBe(200);
  const submittedBody = (await submitted.json()) as {
    action: string;
    tile: { status: string; passes: number };
    agreement: { retires: boolean; confirmations: unknown[] };
  };
  expect(submittedBody.action).toBe("confirm");
  expect(submittedBody.tile.status).toBe("retired");
  expect(submittedBody.tile.passes).toBe(2);
  expect(submittedBody.agreement.retires).toBe(true);
  // Both reads and the agreed rejection are confirmations: three proposals,
  // three agreements.
  expect(submittedBody.agreement.confirmations).toHaveLength(3);

  // The retired lane shows the retired tile, and the tile's history carries
  // the whole arc, oldest first, ending in the confirmation.
  await page.goto("/region-workbench");
  await page.getByRole("tab", { name: "المتقاعدة" }).click();
  await expect(page.locator(".workbench-tile").filter({ hasText: "tile:journey-plate" })).toBeVisible();

  const history = await request.get(`${API_URL}/api/v1/admin/region-workbench/tiles/${tileID}/history`, { headers: { cookie } });
  expect(history.status(), await history.text()).toBe(200);
  const historyBody = (await history.json()) as { history: Array<{ action: string; toStatus: string }> };
  expect(historyBody.history[0].action).toBe("claim");
  expect(historyBody.history.at(-1)?.toStatus).toBe("retired");

  // And the arc promoted nothing: the answered candidates keep their review
  // statuses and carry no region geometry — promotion is a later surface,
  // and this lane never renders as fact.
  const answered = await request.get(`${API_URL}/api/v1/admin/region-candidates?status=corrected&plate_key=journey-plate`, { headers: { cookie } });
  const answeredBody = (await answered.json()) as { candidates: Array<{ id: string; hasRegionGeometry: boolean }> };
  expect(answeredBody.candidates.length).toBeGreaterThanOrEqual(1);
  for (const candidate of answeredBody.candidates) {
    expect(candidate.hasRegionGeometry).toBe(false);
  }
});
