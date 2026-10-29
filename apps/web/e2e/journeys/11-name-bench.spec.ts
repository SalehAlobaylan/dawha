import { expect, test } from "@playwright/test";

import {
  API_URL,
  executeSql,
  loginThroughUi,
  newAccount,
  readRows,
  registerThroughApi,
  registerThroughUi,
  syntheticSourcePNG,
  waitForApi,
} from "../fixtures";

/**
 * Plan 026's bench journey: defer a real occurrence, reload, find it again,
 * reopen it, correct it, and inspect the append-only history. The machine
 * readings are seeded into the test's own source because the bench has no
 * public "create a reading" route; every row is scoped to the synthetic account
 * and is removed with its source by e2e/cleanup.sql.
 */

// A 600x400 synthetic chart is large enough for the source-space crop and
// region-repair path. The journey never asserts on the picture; it asserts on
// the ledger and the geometry revisions.
const CHART_PNG = syntheticSourcePNG();

test.describe("name review history and repair", () => {
  test("deferred work survives reload, reopens, and keeps both human answers", async ({ page, browser, request }) => {
    await waitForApi(request);
    const owner = newAccount("name-bench");
    await registerThroughUi(page, owner);

    const title = `شجرة مراجعة ${owner.displayName}`;
    const sourceResponse = await page.request.post(`${API_URL}/api/v1/sources`, {
      data: { title_ar: title, source_type: "family_document" },
    });
    expect(sourceResponse.status(), await sourceResponse.text()).toBe(201);
    const sourceBody = (await sourceResponse.json()) as { source: { id: string } };
    const sourceId = sourceBody.source.id;

    const uploadResponse = await page.request.post(`${API_URL}/api/v1/sources/${sourceId}/files`, {
      multipart: { file: { name: "chart.png", mimeType: "image/png", buffer: CHART_PNG } },
    });
    expect(uploadResponse.status(), await uploadResponse.text()).toBe(201);
    const file = (await uploadResponse.json()) as { id: string; checksumSha256: string };

    const firstOccurrence = `e2e-name-${Date.now()}-a`;
    const secondOccurrence = `e2e-name-${Date.now()}-b`;
    for (const [occurrence, text] of [[firstOccurrence, "عبدالله"], [secondOccurrence, "صالح"]] as const) {
      await executeSql(
        `INSERT INTO source_tree_suggestions (
           source_id, source_file_id, occurrence_id, page_number, kind, suggested_text_ar,
           region, confidence, view_count, reading_method, reading_model
         ) VALUES ($1, $2, $3, 1, 'name', $4, $5::jsonb, 0.6, 1, 'e2e_fixture', 'e2e_fixture')`,
        [sourceId, file.id, occurrence, text, JSON.stringify({ x: 0.1, y: 0.1, width: 0.3, height: 0.2 })],
      );
    }

    await openBench(page, title);

    // Defer the first reading. It leaves pending work immediately.
    const bench = page.locator(".name-bench");
    await expect(bench.locator(".name-bench-reading strong")).toContainText("عبدالله");
    await bench.getByRole("button", { name: "تأجيل", exact: true }).click();
    await expect(bench.getByRole("status").filter({ hasText: "أُجّل" })).toBeVisible();
    await bench.getByRole("button", { name: "إغلاق ورشة الأسماء" }).click();

    // Reload the page, as a reviewer returning next day would. The deferred item
    // is not lost and is reachable from its own view.
    await openBench(page, title);
    const deferredTab = bench.getByRole("tab", { name: /مؤجلة/ });
    await deferredTab.click();
    await expect(deferredTab).toHaveAttribute("aria-selected", "true");
    await expect(bench.locator(".name-bench-history")).toContainText("عبدالله");
    await expect(bench.locator(".name-bench-history")).toContainText("تأجيل");
    await bench.getByRole("button", { name: /إعادة فتح الموضع/ }).click();
    await expect(bench.getByRole("tab", { name: /بانتظار المراجعة/ })).toHaveAttribute("aria-selected", "true");

    // Correct the reopened occurrence. The input keeps the original machine
    // reading until the reviewer changes it.
    const input = bench.locator(".name-bench-field input");
    await expect(input).toHaveValue("عبدالله");
    await input.fill("عبد الله");
    await bench.getByRole("button", { name: /احفظ الاسم/ }).click();
    await expect(bench.getByRole("status").filter({ hasText: "حُفظ التصحيح" })).toBeVisible();

    // History still shows the original reading and both human revisions.
    await bench.getByRole("tab", { name: /^السجل/ }).click();
    const history = bench.locator(".name-bench-history");
    await expect(history).toContainText("القراءة الأصلية");
    await expect(history).toContainText("عبدالله");
    await expect(history).toContainText("عبد الله");
    await expect(history).toContainText("إعادة فتح");

    // One physical occurrence, two human revisions, no duplicated person or
    // truth label. The ledger is the only place the answer lives.
    const suggestions = await readRows<{ id: string; occurrence_id: string; status: string; review_revision: number }>(
      `SELECT id, occurrence_id, status, review_revision
         FROM source_tree_suggestions
        WHERE source_file_id = $1
        ORDER BY occurrence_id`,
      [file.id],
    );
    expect(suggestions).toHaveLength(2);
    const reopened = suggestions.find((suggestion) => suggestion.occurrence_id === firstOccurrence);
    expect(reopened?.status).toBe("corrected");
    expect(reopened?.review_revision).toBe(2);

    const decisions = await readRows<{ review_revision: number; decision: string; decided_text_ar: string | null }>(
      `SELECT review_revision, decision, decided_text_ar
         FROM source_tree_suggestion_decisions
        WHERE suggestion_id = $1
        ORDER BY review_revision`,
      [reopened!.id],
    );
    expect(decisions).toEqual([
      { review_revision: 1, decision: "deferred", decided_text_ar: null },
      { review_revision: 2, decision: "corrected", decided_text_ar: "عبد الله" },
    ]);
    const events = await readRows<{ event_type: string; revision: number }>(
      `SELECT event_type, revision FROM source_name_review_events
        WHERE suggestion_id = $1 ORDER BY created_at, id`,
      [reopened!.id],
    );
    expect(events.filter((event) => event.event_type === "reopened")).toHaveLength(1);
    expect(events.filter((event) => event.event_type === "decision")).toHaveLength(2);
    const people = await readRows<{ count: string }>(
      `SELECT COUNT(*)::text AS count FROM people WHERE created_by = (SELECT id FROM users WHERE email = $1)`,
      [owner.email],
    );
    expect(Number(people[0].count)).toBe(0);

    // A researcher with no access to this source can neither inspect nor reopen
    // the history. The refusal happens before any ledger write.
    const outsider = newAccount("name-outsider");
    await registerThroughApi(request, outsider);
    const outsiderContext = await browser.newContext();
    const outsiderPage = await outsiderContext.newPage();
    try {
      await loginThroughUi(outsiderPage, outsider);
      const deniedHistory = await outsiderPage.request.get(`${API_URL}/api/v1/source-tree-suggestions/${reopened!.id}/history`);
      expect(deniedHistory.status()).toBe(403);
      const deniedReopen = await outsiderPage.request.post(`${API_URL}/api/v1/source-tree-suggestions/${reopened!.id}/reopen`, {
        data: { expectedRevision: 2 },
      });
      expect(deniedReopen.status()).toBe(403);
    } finally {
      await outsiderContext.close();
    }
    const unchanged = await readRows<{ review_revision: number; status: string }>(
      `SELECT review_revision, status FROM source_tree_suggestions WHERE id = $1`,
      [reopened!.id],
    );
    expect(unchanged[0]).toEqual({ review_revision: 2, status: "corrected" });
  });

  test("repairs a bad rectangle without losing the original crop or reading", async ({ page, request }) => {
    await waitForApi(request);
    const owner = newAccount("name-region");
    await registerThroughUi(page, owner);
    const title = `شجرة تصحيح ${owner.displayName}`;
    const sourceResponse = await page.request.post(`${API_URL}/api/v1/sources`, {
      data: { title_ar: title, source_type: "family_document" },
    });
    expect(sourceResponse.status(), await sourceResponse.text()).toBe(201);
    const sourceId = ((await sourceResponse.json()) as { source: { id: string } }).source.id;
    const uploadResponse = await page.request.post(`${API_URL}/api/v1/sources/${sourceId}/files`, {
      multipart: { file: { name: "chart.png", mimeType: "image/png", buffer: CHART_PNG } },
    });
    expect(uploadResponse.status(), await uploadResponse.text()).toBe(201);
    const file = (await uploadResponse.json()) as { id: string };

    const originalRegion = { x: 200 / 600, y: 150 / 400, width: 80 / 600, height: 40 / 400 };
    await executeSql(
      `INSERT INTO source_tree_suggestions (
         source_id, source_file_id, occurrence_id, page_number, kind, suggested_text_ar,
         region, alternatives, confidence, view_count, reading_method, reading_model
       ) VALUES ($1, $2, $3, 1, 'name', 'أحمد', $4::jsonb, '["أحمد","محمد"]'::jsonb, 0.6, 3, 'e2e_fixture', 'e2e_fixture')`,
      [sourceId, file.id, `e2e-region-${Date.now()}`, JSON.stringify(originalRegion)],
    );

    await openBench(page, title);
    const bench = page.locator(".name-bench");
    const suggestion = (await readRows<{ id: string }>(`SELECT id FROM source_tree_suggestions WHERE source_file_id=$1`, [file.id]))[0];

    // Cancel changes nothing; then reopen and confirm one bounded revision.
    await bench.getByRole("button", { name: /تعديل الحدود/ }).click();
    await bench.locator(".name-bench-region-grid input").first().fill("25");
    await bench.getByRole("button", { name: "إلغاء", exact: true }).click();
    expect((await readRows(`SELECT id FROM source_name_region_revisions WHERE suggestion_id=$1`, [suggestion.id])).length).toBe(0);

    await bench.getByRole("button", { name: /تعديل الحدود/ }).click();
    await bench.locator(".name-bench-region-grid input").first().fill("30");
    await bench.getByRole("button", { name: /معاينة/, exact: true }).click();
    await expect(bench.locator(".name-bench-region-preview img").first()).toBeVisible();
    await bench.getByRole("button", { name: /حفظ الحدود/ }).click();
    await expect(bench.getByRole("status").filter({ hasText: "حُدّثت حدود القراءة" })).toBeVisible();

    const suggestionRows = await readRows<{ region: { x: number }; geometry_revision: number; suggested_text_ar: string }>(
      `SELECT region, geometry_revision, suggested_text_ar FROM source_tree_suggestions WHERE id=$1`,
      [suggestion.id],
    );
    expect(suggestionRows[0].geometry_revision).toBe(1);
    expect(suggestionRows[0].suggested_text_ar).toBe("أحمد");
    expect(suggestionRows[0].region.x).toBeCloseTo(0.3, 6);
    const revisions = await readRows<{ revision: number; previous_region: { x: number }; region: { x: number } }>(
      `SELECT revision, previous_region, region FROM source_name_region_revisions WHERE suggestion_id=$1`,
      [suggestion.id],
    );
    expect(revisions).toHaveLength(1);
    expect(revisions[0].revision).toBe(1);
    expect(revisions[0].previous_region.x).toBeCloseTo(200 / 600, 6);
    expect(revisions[0].region.x).toBeCloseTo(0.3, 6);

    await bench.locator(".name-bench-editor-head").getByRole("button", { name: /السجل/ }).click();
    const history = bench.locator(".name-bench-history");
    await expect(history).toContainText("حدود القراءة المعدّلة");
    await expect(history).toContainText("33٪");
    await expect(history).toContainText("30٪");

    // A save after repair must carry the new geometry token through the real
    // browser/API path and retain it on the immutable decision.
    await bench.locator(".name-bench-field input").fill("أحمد محمد");
    const saved = page.waitForResponse((response) => response.url().endsWith(`/source-tree-suggestions/${suggestion.id}/decision`) && response.request().method() === "POST");
    await bench.getByRole("button", { name: /احفظ الاسم/ }).click();
    expect((await saved).status()).toBe(201);
    await expect(bench.getByRole("heading", { name: "لا توجد أسماء بانتظار المراجعة" })).toBeVisible();
    const savedGeometry = await readRows<{ geometry_revision: number }>(
      `SELECT geometry_revision FROM source_tree_suggestion_decisions WHERE suggestion_id=$1`,
      [suggestion.id],
    );
    expect(savedGeometry).toEqual([{ geometry_revision: 1 }]);
  });
});

async function openBench(page: import("@playwright/test").Page, title: string): Promise<void> {
  await page.goto("/sources");
  await page.locator(".evidence-source-list").getByRole("button", { name: new RegExp(title) }).click();
  await expect(page.locator(".source-processing-head").getByRole("heading", { name: title })).toBeVisible();
  const review = page.locator(".source-processing-panel").getByRole("link", { name: "مراجعة اقتراحات الأسماء", exact: true });
  await expect(review).toBeVisible();
  await review.click();
  await expect(page.locator(".name-bench")).toBeVisible();
}
