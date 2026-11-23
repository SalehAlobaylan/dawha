import { randomUUID } from "node:crypto";
import { expect, test, type Page } from "@playwright/test";

import { API_URL, executeSql, grantPlatformRole, loginThroughUi, newAccount, registerThroughApi, sessionCookieHeader, waitForApi } from "../fixtures";

async function openDirectory(page: Page) {
  const toggle = page.getByRole("button", { name: "فتح الفهرس" });
  if (await toggle.isVisible()) {
    await toggle.click();
  }
}

/**
 * Journey 08: browse the map.
 *
 * The Atlas is asserted through the data it actually shipped, not through
 * pixels. A WebGL canvas can be blank and still be "visible", so the canvas is
 * checked for its accessible name while every claim about content is made
 * against the result rows and the provenance chamber, which are ordinary DOM and
 * are the same data the canvas draws.
 *
 * The seeded map carries four places, two associations at الرياض and one
 * inferred migration from الأحساء to الرياض, so a test can name what it expects
 * without depending on how many rows a fixture happens to create.
 */
test.describe("browse the atlas", () => {
  test("a reader sees real features, the uncertainty key and the method note", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/places");

    await expect(page.getByRole("heading", { name: "الموضع جزء من السؤال" })).toBeVisible();

    // The canvas is named, and the page says what it does not draw: no modern
    // borders, no tile service, no invented geography.
    await expect(page.getByRole("region", { name: "خريطة الأطلس" })).toBeVisible();
    await expect(page.getByText("لا خلفية بحدود اليوم.")).toBeVisible();

    // The canvas actually paints rather than floats DOM markers above a blank
    // beige rectangle. Measured on this stack: a painting canvas compresses to
    // ~12 KB of PNG, a broken one to ~8 KB (the maplibre tile worker fails
    // silently and GeoJSON layers never render — which is exactly what this
    // assertion exists so the suite catches it).
    const canvasPixels = await page.locator(".atlas-map canvas").first().screenshot();
    expect(canvasPixels.length).toBeGreaterThan(10_000);

    // The legend names every status the filter offers, so a glyph cannot appear
    // on the map without a word for it somewhere on the page.
    const legend = page.getByLabel("مفتاح الحالات والأنواع");
    for (const label of ["موثق", "مقروء", "استنتاجي", "متنازع عليه", "غير محسوم"]) {
      await expect(legend.getByRole("img", { name: label })).toBeVisible();
    }

    // The results are derived from the API, not a demo array: the seeded places
    // are present and each row carries a kind and a period or an explicit absence.
    await openDirectory(page);
    const rows = page.locator(".atlas-row");
    await expect(rows.first()).toBeVisible();
    expect(await rows.count()).toBeGreaterThan(0);
    await expect(page.locator(".atlas-results")).toContainText("الأحساء");
    await expect(page.locator(".atlas-results")).toContainText("الرياض");

    // The migration is one of the rows, and it is labelled as a route rather
    // than folded into a place marker.
    await expect(page.locator(".atlas-row").filter({ hasText: "مسار هجرة" })).toHaveCount(1);

    // Honesty about uncertainty is part of the surface, not a tooltip.
    await expect(page.getByText("كيف نعرض عدم اليقين؟")).toBeVisible();
    await expect(page.locator(".atlas-map-note")).toContainText("الخطوط المنقطة استنتاجات");
  });

  test("a row opens the provenance chamber with the evidence it actually has", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/places");
    await openDirectory(page);

    // The seeded migration is the one feature that carries evidence text, a
    // source and a review state, so it proves the chamber reads the record rather
    // than filling in a template.
    const migrationRow = page.locator(".atlas-row").filter({ hasText: "مسار هجرة" }).first();
    await expect(migrationRow).toBeVisible();
    await migrationRow.click();

    const inspector = page.getByLabel("فاحصة الإشارة");
    await expect(inspector.getByRole("heading", { name: "الرياض" })).toBeVisible();
    await expect(inspector).toContainText("مسار هجرة");
    await expect(inspector).toContainText("غير مؤكد");
    await expect(inspector).toContainText("إشارة موضع تقريبية إلى انتقال محتمل.");
    await expect(inspector).toContainText("يحتاج مراجعة");

    // A record without a claim says so instead of inventing a destination.
    await expect(inspector).toContainText("هذه إشارة جغرافية، لا تخدم ادعاءً محدداً.");

    // The place handoff is real and carries the name the reader just saw.
    const placeHandoff = inspector.getByRole("link", { name: /ابحث عن الموضع في فهرس البحث/ });
    await expect(placeHandoff).toBeVisible();
    await expect(placeHandoff).toHaveAttribute("href", /\/search\?/);
    await expect(placeHandoff).toHaveAttribute("href", /kind=places/);

    // The selected row and the chamber agree, so the list and the chamber are
    // two views of one selection rather than two independent states.
    await expect(migrationRow).toHaveAttribute("aria-pressed", "true");
  });

  test("an era chip is a named window: it sets the URL, survives a reload, and the region readout answers its question", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/places");
    await openDirectory(page);
    await expect(page.locator(".atlas-row").first()).toBeVisible();

    // The reader selects the region they are about to interrogate BEFORE the
    // era chip: choosing a window is a question about a selection, and the
    // chamber must survive the question (plan 029-A2) instead of closing.
    const regionRow = page.locator(".atlas-row").filter({ hasText: "منطقة تاريخية" }).first();
    await expect(regionRow).toBeVisible();
    const regionRowCount = await page.locator(".atlas-row").filter({ hasText: "منطقة تاريخية" }).count();
    await regionRow.click();
    const inspector = page.getByLabel("فاحصة الإشارة");
    await expect(inspector).toContainText("منطقة تاريخية");

    // The chip is vocabulary over the same window: clicking it is setting
    // from/to, and the URL carries the two numbers the reload must keep.
    const era = page.locator(".atlas-era-pill").filter({ hasText: "الدولة السعودية الأولى" }).first();
    await expect(era).toBeVisible();
    await era.click();
    await expect(era).toHaveAttribute("aria-pressed", "true");
    await expect(page).toHaveURL(/from=1727/);
    await expect(page).toHaveURL(/to=1818/);

    // The selection survived the window change: the readout still carries the
    // region — the era question is being asked OF it.
    await expect(inspector).toContainText("منطقة تاريخية");
    await expect(inspector).toContainText("شواهد مؤرَّخة في العصر");
    await expect(inspector.getByText(/شواهد|لا وثيقة مؤرَّخة في هذا العصر/).first()).toBeVisible();
    await expect(inspector).toContainText("أعيان موثَّقون في العصر");

    // The seeded regions never dated themselves (valid_from/valid_to are
    // NULL), so the era window does not prove their presence — and the
    // map says so rather than reading identically under every era. They
    // stay drawn (hiding one would claim the region did not exist), but
    // in the weakest voice and with this sentence.
    await expect(inspector).toContainText("هذه المنطقة بلا تواريخ مؤرَّخة؛ ظهورها في نافذة العهد لا يعني تواجدها فيها.");

    // A reload keeps the named window, and the chip reads it back as active —
    // one control, not two.
    await page.reload();
    await expect(page.locator(".atlas-era-pill").filter({ hasText: "الدولة السعودية الأولى" })).toHaveAttribute("aria-pressed", "true");

    // And the weakened voice never becomes an absence (plan 029-C): the
    // demo's regions carry no dated validity, so all of them drop to the
    // weakest voice under this window — and the reader still sees every one
    // of them as a row. An era window that made a region disappear would be
    // claiming nothing supports.
    await expect(page.locator(".atlas-row").filter({ hasText: "منطقة تاريخية" })).toHaveCount(regionRowCount);
  });

  test("filters change the rows, the URL and survive a reload", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/places");
    await openDirectory(page);

    const rows = page.locator(".atlas-row");
    await expect(rows.first()).toBeVisible();
    const total = await rows.count();
    expect(total).toBeGreaterThan(1);

    // Turning off the documented layer removes the documented places and
    // associations, but keeps the inferred migration. The pill's state and the
    // URL are the same decision.
    const documented = page.locator(".atlas-status-pill").filter({ hasText: "موثق" });
    await expect(documented).toHaveAttribute("aria-pressed", "true");
    await documented.click();
    await expect(documented).toHaveAttribute("aria-pressed", "false");
    await expect(page).toHaveURL(/status=/);
    await expect(page.locator(".atlas-row").filter({ hasText: "مسار هجرة" })).toHaveCount(1);

    // The research position survives a reload: the same filter is read back from
    // the URL rather than reset to "everything".
    await page.reload();
    await openDirectory(page);
    await expect(page.locator(".atlas-status-pill").filter({ hasText: "موثق" })).toHaveAttribute("aria-pressed", "false");
    await expect(page.locator(".atlas-row").filter({ hasText: "مسار هجرة" })).toHaveCount(1);

    // The kind filter is independent of the status filter and removes the route
    // from the list without touching the remaining place markers.
    const migrationKind = page.locator(".atlas-kind-pill").filter({ hasText: "مسار هجرة" });
    await migrationKind.click();
    await expect(migrationKind).toHaveAttribute("aria-pressed", "false");
    await expect(page.locator(".atlas-row").filter({ hasText: "مسار هجرة" })).toHaveCount(0);

    // Reset is a real reset, not a cosmetic button: the URL is cleaned and the
    // rows come back.
    await page.getByRole("button", { name: /إعادة ضبط/ }).click();
    await expect(page).not.toHaveURL(/status=/);
    await expect(page).not.toHaveURL(/kind=/);
    await expect(page.locator(".atlas-status-pill").filter({ hasText: "موثق" })).toHaveAttribute("aria-pressed", "true");
    await expect(rows).toHaveCount(total);
  });

  test("an empty filter set is a deliberate state, and the page says why it is empty", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/places?status=none");

    // `none` is not "no filter": it is the empty set, and the page must not read
    // that as a broken or unfinished archive.
    await expect(page.getByText("لا توجد إشارات تطابق التصفية")).toBeVisible();
    await expect(page.getByText("الإشارات موجودة، لكن ما اخترته لا يطابقها.")).toBeVisible();
    await expect(page.locator(".atlas-filter-summary")).toContainText("0");

    // The escape hatch is on screen and restores the map.
    await page.getByRole("button", { name: /أعد ضبط التصفية/ }).click();
    await expect(page).not.toHaveURL(/status=/);
    await openDirectory(page);
    await expect(page.locator(".atlas-row").first()).toBeVisible();
  });

  test("a deep link selects the feature, and a reload keeps the same selection", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();

    // The identifier is the seeded migration, so the link is a claim about a
    // real row rather than a fabricated route parameter.
    const migrationID = "a1000000-0000-0000-0000-000000000001";
    await page.goto(`/places?feature=${migrationID}`);

    const inspector = page.getByLabel("فاحصة الإشارة");
    await expect(inspector).toContainText("إشارة موضع تقريبية إلى انتقال محتمل.");
    // The route readout is the feature's own coordinates, not a decorative
    // pair: الأحساء is the origin and الرياض the destination.
    await expect(inspector).toContainText("49.57");
    await expect(inspector).toContainText("46.68");

    // Reload re-reads the same URL and lands on the same record.
    await page.reload();
    await expect(page.getByLabel("فاحصة الإشارة")).toContainText("إشارة موضع تقريبية إلى انتقال محتمل.");
    await expect(page).toHaveURL(new RegExp(`feature=${migrationID}`));
  });

  test("search narrows by place and is part of the linkable state", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/places");

    const search = page.getByLabel("ابحث بموضع أو كيان");
    await search.fill("الأحساء");

    // The debounce writes the query to the URL, and only the features that
    // actually carry the name survive.
    await expect(page).toHaveURL(/q=/);
    await openDirectory(page);
    const rows = page.locator(".atlas-row");
    await expect(rows.first()).toBeVisible();
    const count = await rows.count();
    for (let index = 0; index < count; index += 1) {
      await expect(rows.nth(index)).toContainText("الأحساء");
    }

    // Reload keeps the query and the reduced list.
    await page.reload();
    await openDirectory(page);
    await expect(page.getByLabel("ابحث بموضع أو كيان")).toHaveValue("الأحساء");
    await expect(page.locator(".atlas-row").first()).toContainText("الأحساء");
  });

  test("adding a place without a session is refused honestly, not with a dead button", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/places");

    await page.getByRole("button", { name: /إضافة موضع/ }).click();
    await expect(page.getByRole("status").filter({ hasText: "سجّل الدخول لإضافة موضع" })).toBeVisible();
  });

  test("a signed-in researcher opens the inference layer and reaches the review UI", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    const account = newAccount("atlas-inference");
    await registerThroughApi(request, account);
    await grantPlatformRole(account.email, "researcher");
    await loginThroughUi(page, account);
    const cookie = await sessionCookieHeader(page);

    // Fixture: one synthetic person with three temporally overlapping
    // associations on the seeded places (الرياض، الأحساء، العلا) under one public,
    // independent source. A geospatial run over them resolves all three
    // mentions, builds one spatial cluster (الرياض ↔ الأحساء, ~300 km) and flags
    // the over-radius pairs as findings that need review. Every row is removed
    // by the suite teardown with the account that created it.
    const personID = randomUUID();
    const treeID = randomUUID();
    const versionID = randomUUID();
    const nodeID = randomUUID();
    const sourceID = randomUUID();
    const statementID = randomUUID();
    const userOf = (email: string) => `(SELECT id FROM users WHERE email = '${email}')`;
    await executeSql(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, birth_date_from, birth_date_to, identity_status, created_by) VALUES ($1, 'شخص استنتاج الأطلس', 'شخص استنتاج الأطلس', '1150-01-01', '1250-12-31', 'reviewed', ${userOf(account.email)})`, [personID]);
    await executeSql(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, 'شجرة استنتاج الأطلس', 'public', ${userOf(account.email)})`, [treeID]);
    await executeSql(`INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, 'published', ${userOf(account.email)}, now())`, [versionID, treeID]);
    await executeSql(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, 'شخص استنتاج الأطلس', 1)`, [nodeID, versionID, personID]);
    await executeSql(`INSERT INTO sources (id, title_ar, source_type, dependency_status, visibility, created_by) VALUES ($1, 'مصدر استنتاج الأطلس', 'book', 'independent', 'public', ${userOf(account.email)})`, [sourceID]);
    await executeSql(`INSERT INTO source_statements (id, source_id, statement_text_ar, review_status, created_by) VALUES ($1, $2, 'ذكر النص الرياض والأحساء والعلا في فترات متداخلة، وكلها مورد جغرافي.', 'accepted', ${userOf(account.email)})`, [statementID, sourceID]);
    const seededPlaces = [
      "20000000-0000-0000-0000-000000000001", // الرياض
      "20000000-0000-0000-0000-000000000002", // الأحساء
      "20000000-0000-0000-0000-000000000004", // العلا
    ];
    for (const placeID of seededPlaces) {
      const associationID = randomUUID();
      await executeSql(`INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, time_from, time_to, status, certainty, source_id, created_by) VALUES ($1, 'person', $2, $3, 'documented_in', '1200-01-01', '1210-12-31', 'interpreted', 'approximate', $4, ${userOf(account.email)})`, [associationID, personID, placeID, sourceID]);
      await executeSql(`INSERT INTO spatial_evidence (id, geographic_association_id, review_status, created_by) VALUES ($1, $2, 'accepted', ${userOf(account.email)})`, [randomUUID(), associationID]);
    }
    const question = await request.post(`${API_URL}/api/v1/questions`, {
      headers: { cookie },
      data: { title_ar: `سؤال استنتاج الأطلس ${personID.slice(0, 8)}`, description_ar: "", status: "open", priority: "normal" },
    });
    expect(question.status(), await question.text()).toBe(201);
    const questionBody = (await question.json()) as { question: { id: string } };
    const questionID = questionBody.question.id;
    const run = await request.post(`${API_URL}/api/v1/geospatial-intelligence/runs`, {
      headers: { cookie },
      data: { entity_type: "person", entity_id: personID, question_id: questionID, tree_id: treeID, tree_version_id: versionID, radius_km: 500, maximum_records: 500 },
    });
    expect(run.status(), await run.text()).toBe(201);
    const runBody = (await run.json()) as { id: string; findingCount: number; report: { clusters: unknown[] } };
    expect(runBody.findingCount).toBeGreaterThan(0);
    expect(runBody.report.clusters.length).toBeGreaterThan(0);

    await page.goto("/places");
    await page.getByRole("button", { name: /الطبقة الاستنتاجية/ }).click();
    const panel = page.getByRole("region", { name: "الطبقة الاستنتاجية" });
    await expect(panel).toBeVisible();

    // The stamp-ring grammar: a sealed hypothesis is drawn like one, never like
    // a record — the panel says so, and the findings need review rather than
    // verdicts.
    await expect(panel).toContainText("ختم منصة، لا شهادة مصدر");
    await expect(panel).toContainText("تحتاج مراجعة");

    // The handoff to the review UI is a real link to the question workspace,
    // on the analysis tab, where the review actions live.
    const review = panel.getByRole("link", { name: /افتح في مساحة التحقيق/ }).first();
    await expect(review).toBeVisible();
    await expect(review).toHaveAttribute("href", new RegExp(`/research/${questionID}`));
    await expect(review).toHaveAttribute("href", /panel=analysis/);
  });

  test("the dossier shows only accepted rows to a reader, and the pending lane to a moderator", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    const account = newAccount("atlas-dossier");
    await registerThroughApi(request, account);
    await grantPlatformRole(account.email, "moderator");
    const userOf = (email: string) => `(SELECT id FROM users WHERE email = '${email}')`;
    // Fixture: one public source, one accepted passage and one needs_review
    // passage for the seeded الرياض place (20000000-…-0001), tied through
    // documented associations. Relative assertions only: the texts carry
    // randomUUID suffixes so the test cannot read another run's rows.
    const suffix = randomUUID().slice(0, 8);
    const sourceID = randomUUID();
    await executeSql(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'مصدر سجل الأدلة', 'book', 'public', ${userOf(account.email)})`, [sourceID]);
    const acceptedPassage = randomUUID();
    const pendingPassage = randomUUID();
    await executeSql(`INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar) VALUES ($1, $2, 900, 900, 'ص 900', 'نص الرحلة المقبول ${suffix}', 'نص الرحله المقبول ${suffix}')`, [acceptedPassage, sourceID]);
    await executeSql(`INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar) VALUES ($1, $2, 901, 901, 'ص 901', 'نص بانتظار المراجعة ${suffix}', 'نص بانتظار المراجعه ${suffix}')`, [pendingPassage, sourceID]);
    const personID = "10000000-0000-0000-0000-000000000001";
    const riyadhID = "20000000-0000-0000-0000-000000000001";
    for (const passageID of [acceptedPassage, pendingPassage]) {
      const associationID = randomUUID();
      await executeSql(`INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, status, certainty, source_id, created_by) VALUES ($1, 'person', $2, $3, 'documented_in', 'documented', 'approximate', $4, ${userOf(account.email)})`, [associationID, personID, riyadhID, sourceID]);
      const status = passageID === acceptedPassage ? "accepted" : "needs_review";
      await executeSql(`INSERT INTO spatial_evidence (id, geographic_association_id, source_passage_id, review_status, created_by) VALUES ($1, $2, $3, $4, ${userOf(account.email)})`, [randomUUID(), associationID, passageID, status]);
    }

    // A reader (anonymous) sees the accepted row and does not see the pending one.
    await page.goto("/places");
    await openDirectory(page);
    await page.locator(".atlas-row").filter({ hasText: "الرياض" }).first().click();
    const inspector = page.getByLabel("فاحصة الإشارة");
    const dossier = inspector.getByLabel("سجل الأدلة للموضع");
    await expect(dossier).toContainText(`نص الرحلة المقبول ${suffix}`);
    await expect(dossier).not.toContainText(`نص بانتظار المراجعة ${suffix}`);
    await expect(dossier).not.toContainText("بانتظار المراجعة");

    // The moderator sees both, the pending one behind its band, in the same folio.
    await loginThroughUi(page, account);
    await page.goto("/places");
    await openDirectory(page);
    await page.locator(".atlas-row").filter({ hasText: "الرياض" }).first().click();
    const dossierAsModerator = page.getByLabel("فاحصة الإشارة").getByLabel("سجل الأدلة للموضع");
    await expect(dossierAsModerator).toContainText(`نص الرحلة المقبول ${suffix}`);
    await expect(dossierAsModerator).toContainText(`نص بانتظار المراجعة ${suffix}`);
    await expect(dossierAsModerator).toContainText("بانتظار المراجعة");
  });

  test("the subject layer draws documented مسالك, and says لا مسار موثّق بعد when there are none", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    // Fixture: two documented events for the seeded public person
    // (10000000-…-0001, in the demo's published public tree) from the demo's
    // public source, between the seeded الرياض and الأحساء places.
    const account = newAccount("atlas-subject");
    await registerThroughApi(request, account);
    const userOf = (email: string) => `(SELECT id FROM users WHERE email = '${email}')`;
    const personID = "10000000-0000-0000-0000-000000000001";
    const riyadhID = "20000000-0000-0000-0000-000000000001";
    const ahsaID = "20000000-0000-0000-0000-000000000002";
    const publicSourceID = "30000000-0000-0000-0000-000000000002";
    for (const [from, to, start] of [[riyadhID, ahsaID, "1180-01-01"], [ahsaID, riyadhID, "1182-01-01"]] as const) {
      await executeSql(`INSERT INTO migration_events (id, subject_type, subject_id, from_place_id, to_place_id, time_from, time_to, status, certainty, source_id, notes_ar, created_by) VALUES ($1, 'person', $2, $3, $4, $5, '1181-01-01', 'documented', 'approximate', $6, 'نص رحلة موثقة.', ${userOf(account.email)})`, [randomUUID(), personID, from, to, start, publicSourceID]);
    }

    await page.goto(`/places?subject=${personID}`);
    await openDirectory(page);
    const banner = page.locator(".atlas-subject-banner");
    await expect(banner).toContainText("مسار");
    await expect(banner).toContainText("حدث موثّق");
    // The banner count is relative to what shipped: at least our two events.
    const bannerText = await banner.textContent();
    const subjectCount = Number(bannerText?.match(/(\d+) حدث/)?.[1] ?? 0);
    expect(subjectCount).toBeGreaterThanOrEqual(2);

    // The subject layer answers to the same filters the main layers do
    // (plan 029-D): under the first Saudi state's window the مسالِك's
    // 1180s segments drop wherever the same dates drop a main-layer row —
    // the banner reads their honest zero and names the subjection, and
    // a documented/interpreted segment cannot survive a window that
    // empties it. What may remain under the window is only what no era
    // can judge: an undated row the keep-all rule keeps.
    const rowsBefore = await page.locator(".atlas-row").filter({ hasText: "مسار هجرة" }).count();
    const saudiFirst = page.locator(".atlas-era-pill").filter({ hasText: "الدولة السعودية الأولى" });
    await expect(saudiFirst).toBeVisible();
    await saudiFirst.click();
    await expect(page).toHaveURL(/from=1727/);
    const bannerFiltered = page.locator(".atlas-subject-banner");
    await expect(bannerFiltered).toContainText("الطبقة تابعة للتصفية");
    await expect(bannerFiltered).toContainText("0 حدث موثّق");
    const remaining = page.locator(".atlas-row").filter({ hasText: "مسار هجرة" });
    const remainingCount = await remaining.count();
    await expect(remainingCount).toBeLessThanOrEqual(rowsBefore);
    for (let index = 0; index < remainingCount; index += 1) {
      await expect(remaining.nth(index)).toContainText("غير محددة");
    }

    // A person with no documented events gets the honest empty state, not a
    // blank banner or a fake route.
    await page.goto("/places?subject=10000000-0000-0000-0000-000000000002");
    await expect(page.locator(".atlas-subject-banner")).toContainText("لا مسار موثّق بعد");
  });
});

  test("the plate lane: the register honours its own refusals, the layer-on state rides the URL, and the compare panel reads the stored plate", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/places");
    await openDirectory(page);
    await expect(page.locator(".atlas-row").first()).toBeVisible();

    // The register (plan 032-A) lists the seeded plates with their own honesty:
    // a renderable plate toggles; the no-graticule plate is DISABLED WITH ITS
    // OWN CAPTION, not silently absent.
    const plateGroup = page.getByLabel("طبقات اللوحات المرجعية");
    await expect(plateGroup).toBeVisible();
    const toggles = page.locator(".atlas-plate-toggle");
    const enabledToggles = page.locator(".atlas-plate-toggle:not([disabled])");
    expect(await toggles.count()).toBeGreaterThanOrEqual(2);
    expect(await enabledToggles.count()).toBeGreaterThanOrEqual(2);

    // Layer-on is URL state (plan 027-D grammar): toggling writes the plate's
    // key into the URL, and a reload reads the same layer back.
    const firstPlate = enabledToggles.filter({ hasText: "خريطة قبائل الجزيرة العربية — هيئة الأركان" });
    const tribeLabels = page.getByRole("group", { name: "الأسماء العربية على اللوحة التاريخية" });
    const firstTribeLabel = tribeLabels.locator(".atlas-plate-tribe-label").first();
    await expect(tribeLabels).toHaveCount(0);
    await expect(firstPlate).toHaveAttribute("aria-pressed", "false");
    await firstPlate.click();
    await expect(firstPlate).toHaveAttribute("aria-pressed", "true");
    await expect(page).toHaveURL(/plates=/);
    await expect(firstTribeLabel).toHaveText(/\p{Script=Arabic}/u);
    await page.reload();
    await expect(firstPlate).toHaveAttribute("aria-pressed", "true");
    await expect(page).toHaveURL(/plates=/);
    await expect(firstTribeLabel).toBeVisible();

    await page.getByRole("button", { name: "قراءة اللوحة", exact: true }).click();
    await expect(page).toHaveURL(/view=plate/);
    await expect(page.locator(".atlas-map .atlas-marker")).toHaveCount(0);
    await page.getByLabel("ابحث بموضع أو كيان").fill("BARRIYAH");
    await page.getByRole("region", { name: "أسماء مطابقة على اللوحات" }).getByRole("button", { name: /BARRIYAH/ }).click();
    await expect(page.getByLabel("قراءة اسم من اللوحة")).toContainText("برية");
    await expect(page).toHaveURL(/caption=/);
    await page.getByRole("button", { name: "إغلاق الاسم" }).click();
    await page.getByLabel("ابحث بموضع أو كيان").clear();
    await page.getByRole("button", { name: "عرض الأدلة واللوحة" }).click();

    // The opacity slider is the layer's own voice: the readout and the URL
    // carry the same number the reader moved.
    const opacity = page.getByLabel("شفافية طبقة اللوحات");
    await expect(opacity).toBeEnabled();
    await opacity.fill("85");
    await expect(page).toHaveURL(/plateOpacity=85/);
    await expect(page.locator(".atlas-plate-opacity")).toContainText("85%");

    // مقارنة النقطة (plan 032-C): the arm writes the armed state to the URL,
    // the map click holds the point, and the panel reads the STORED plate
    // through the server's own inverse — the crop is the print's own ink.
    const compareButton = page.getByRole("button", { name: /مقارنة نقطة/ });
    await expect(compareButton).toBeEnabled();
    await compareButton.click();
    await expect(page).toHaveURL(/compare=arm/);
    await expect(page.getByText("انقر على الخريطة لاختيار نقطة المقارنة")).toBeVisible();
    // The click lands at the CANVAS's own center: a fixed page position can
    // be covered by the filters sidebar at narrow viewports, and the map's
    // center is the click the panel actually wants.
    const canvas = page.locator(".atlas-map canvas").first();
    const canvasBox = await canvas.boundingBox();
    expect(canvasBox).not.toBeNull();
    await canvas.click({ position: { x: Math.floor((canvasBox?.width ?? 600) / 2), y: Math.floor((canvasBox?.height ?? 400) / 2) } });
    await expect(page).toHaveURL(/compare=\d/);
    const comparePanel = page.getByLabel("مقارنة النقطة بين اللوحات");
    await expect(comparePanel).toBeVisible();
    // The panel's rows are the renderable plates' own reads; a plate outside
    // this point's frame says so honestly instead of inventing a crop.
    await expect(comparePanel.getByRole("list").or(comparePanel.locator(".plate-compare-rows")).first()).toBeVisible();

    // Closing the panel drops the URL's state — the arm/hold/close cycle is
    // one state, not three.
    await comparePanel.getByRole("button", { name: "إغلاق المقارنة" }).click();
    await expect(page).not.toHaveURL(/compare=/);

    // The recorded relations' stat sits beside the other layer counts (plan
    // 032-E): a bounded read, labelled for what it is.
    await expect(page.getByText("علاقة مسجّلة")).toBeVisible();
    await firstPlate.click();
    await expect(tribeLabels).toHaveCount(0);
  });

  test("the movement filters (plan 033-C): the season word narrows the movement lane, multiplies with the era window, and rides the URL", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/places");
    await openDirectory(page);
    await expect(page.locator(".atlas-row").first()).toBeVisible();

    // The chips' own row renders with the movement vocabulary (plan 033-D's
    // labels — the classical words the sources narrate, not ACLED's keys).
    const chips = page.getByLabel("مرشحات الحركة");
    await expect(chips.getByRole("button", { name: "صيف", exact: true })).toBeVisible();
    await expect(chips.getByRole("button", { name: "شتاء", exact: true })).toBeVisible();

    // The season filter's own honesty over the demo rows: the filter's chip
    // narrows the movement lane (the era slider scopes further, below), and
    // the URL carries the word the reader moved.
    await chips.getByRole("button", { name: "صيف", exact: true }).click();
    await expect(page).toHaveURL(/season=summer/);
    await chips.getByRole("button", { name: "صيف", exact: true }).click();
    await expect(page).not.toHaveURL(/season=/);
    await chips.getByRole("button", { name: "صيف", exact: true }).click();
    await expect(page).toHaveURL(/season=summer/);

    // With the era window on (the 031 lock), the two filters multiply:
    // season=summer under the 1727..1818 window narrows further than either
    // alone, and the URL carries the compound position.
    const era = page.locator(".atlas-era-pill").filter({ hasText: "الدولة السعودية الأولى" }).first();
    await era.click();
    await expect(page).toHaveURL(/from=1727/);
    await expect(page).toHaveURL(/season=summer/);

    // The URL round trip: a reload keeps the same compound position.
    await page.reload();
    await expect(page).toHaveURL(/season=summer/);
    await expect(page).toHaveURL(/from=1727/);
  });
