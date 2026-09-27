import { expect, test } from "@playwright/test";

import { API_URL, grantPlatformRole, newAccount, readRows, registerThroughUi, waitForApi } from "../fixtures";
import { publishedTree } from "../tree-journey";

/**
 * Journey 10: accept a suggestion WITH a typed change set, and prove the record it
 * describes exists and the review is audited.
 *
 * The suggestion panel before this journey could only send {decision, note_ar}, so
 * "an accepted suggestion can create domain changes" was reachable through the API and
 * through nothing a reviewer can click. This journey is the acceptance half of that
 * gap: it fills the composer, reads the description of what will happen before
 * pressing the button, and then checks three separate things -
 *
 *   1. the record exists, through the API a reader uses (the person page now lists the
 *      alias the change set wrote);
 *   2. the decision is on the suggestion, through the API the panel itself reads;
 *   3. the review is audited and the change set is stored with the record it produced,
 *      against the tables - because neither audit_log nor suggestion_change_sets is
 *      readable through any route, and a journey that asserted them through one would
 *      be asserting something weaker.
 *
 * The reviewer is granted a global write role through pg first. A tree owner can
 * review a suggestion but cannot write to the research tables, and the API answers 403
 * for a change set in that case; the second test is that case, and it is the reason
 * the panel explains the refusal instead of hiding the composer.
 */
test.describe("accept a suggestion with a documented change set", () => {
  test("a reviewer with a global write role applies the change the panel described", async ({ page, browser, request }) => {
    await waitForApi(request);
    const owner = newAccount("change-owner");
    await registerThroughUi(page, owner);
    await grantPlatformRole(owner.email, "researcher");
    const tree = await publishedTree(page, owner);

    // The person the tree is about. The composer pre-fills it from the node the
    // suggestion is attached to, and the journey reads the same id from the API the
    // app reads, so a form that pre-filled nothing would fail here rather than
    // silently attaching the change to the wrong record.
    const detail = (await (await request.get(`${API_URL}/api/v1/trees/${tree.treeId}`)).json()) as {
      nodes?: { personId: string; displayName: string }[];
    };
    const person = (detail.nodes ?? []).find((node) => node.displayName === tree.firstPersonName);
    expect(person, "the published tree has no node for its first person").toBeTruthy();

    const alias = `أبو ${person!.displayName.slice(0, 6)}`;
    const correction = `ورد في المخطوطة لقب «${alias}» لهذا الشخص، وهو غير مسجل في السجل.`;
    const strangerContext = await browser.newContext();
    const strangerPage = await strangerContext.newPage();
    try {
      await strangerPage.goto(`/tree/${tree.treeId}/versions/${tree.publishedVersionId}`);
      await strangerPage.getByPlaceholder("اكتب الفقرة أو التصحيح الذي تقترحه، مع أي سياق يساعد المراجع.").fill(correction);
      await strangerPage.getByRole("button", { name: /أرسل المقترح/ }).click();
      await expect(strangerPage.getByRole("status")).toContainText("وصل المقترح كما كتبته");
    } finally {
      await strangerContext.close();
    }

    await page.goto(`/tree/${tree.treeId}/versions/${tree.publishedVersionId}`);
    const reviewSection = page.locator(".suggestion-review-section");
    await expect(reviewSection).toBeVisible();
    await expect(reviewSection.getByText(correction)).toBeVisible();

    // The composer is opt-in: the plain accept path is the one on screen until the
    // reviewer asks for a documented change.
    await reviewSection.getByRole("button", { name: /قبول مع تغيير موثّق/ }).click();
    const composer = reviewSection.locator(".change-composer");
    await expect(composer).toBeVisible();
    await expect(composer.getByLabel("هدف التغيير")).toHaveValue("person");
    // The person id is prefilled from the node, and the field is editable: the panel
    // states a default, it does not lock the reviewer out of naming another person.
    await expect(composer.getByLabel("معرّف الشخص")).toHaveValue(person!.personId);
    await composer.getByLabel("الاسم البديل").fill(alias);
    await composer.getByLabel("نوع اللقب").selectOption("kunyah");

    // The preview states the target, the identifiers and the record that will exist,
    // BEFORE the button is pressed. A reviewer approving a change should not have to
    // imagine it.
    const preview = composer.locator(".change-composer-preview");
    await expect(preview).toContainText("سيُضاف");
    await expect(preview).toContainText(alias);
    await expect(preview).toContainText(person!.personId);
    // And it says what the change set does NOT do, because the API does not do those
    // things either and the panel must not imply that it might.
    await expect(preview).toContainText("لن يُنشر أي شجرة");

    await composer.getByRole("button", { name: /قبّل مع التغيير/ }).click();
    await expect(page.locator(".suggestion-panel-message")).toContainText("حُفظ قرار المراجعة في سجل المقترح");

    // 1. The record exists, read back through the route a reader uses.
    const dictionary = (await (await request.get(`${API_URL}/api/v1/dictionary/people/${person!.personId}`)).json()) as {
      aliases?: { valueAr: string; type: string }[];
    };
    const written = (dictionary.aliases ?? []).find((entry) => entry.valueAr === alias);
    expect(written, "the alias the change set described does not exist on the person page").toBeTruthy();
    expect(written!.type).toBe("kunyah");

    // 2. The decision is on the suggestion, with the note that explains it.
    await reviewSection.getByLabel("ملاحظة القرار").fill("اللقب مثبت في المخطوطة نفسها.");
    const suggestions = (await (await page.request.get(`${API_URL}/api/v1/trees/${tree.treeId}/suggestions`)).json()) as {
      items?: { id: string; textAr: string; status: string; reviews: { decision: string }[] }[];
    };
    const decided = (suggestions.items ?? []).find((item) => item.textAr === correction);
    expect(decided, "the accepted suggestion is not readable").toBeTruthy();
    expect(decided!.status).toBe("accepted");
    expect(decided!.reviews[0]?.decision).toBe("accepted");

    // 3. The review is audited, and the change set is stored with the record it made.
    const audited = await readRows<{ action: string; entity_id: string }>(
      `SELECT action, entity_id FROM audit_log WHERE entity_id = $1 ORDER BY created_at`,
      [decided!.id],
    );
    expect(audited.map((row) => row.action), "the review wrote no audit trail").toEqual(
      expect.arrayContaining(["suggestion_submitted", "suggestion_change_applied", "suggestion_reviewed"]),
    );

    const stored = await readRows<{ target: string; result_type: string; result_id: string }>(
      `SELECT target, result_type, result_id FROM suggestion_change_sets WHERE suggestion_id = $1`,
      [decided!.id],
    );
    expect(stored, "the change set was not stored against the review").toHaveLength(1);
    expect(stored[0].target).toBe("person");
    expect(stored[0].result_type).toBe("person_alias");
    const aliasRows = await readRows<{ id: string; person_id: string }>(
      `SELECT id, person_id FROM person_aliases WHERE id = $1`,
      [stored[0].result_id],
    );
    expect(aliasRows, "the stored change set points at no alias").toHaveLength(1);
    expect(aliasRows[0].person_id).toBe(person!.personId);
  });

  test("a reviewer without a global write role is told why, and can still accept", async ({ page, browser, request }) => {
    await waitForApi(request);
    // A tree owner with no platform role: they may review this tree, and they may not
    // write to the research tables a change set writes. That is the API's rule, and
    // the panel's job is to say so rather than to hide the composer or to pretend.
    const owner = newAccount("change-norole");
    await registerThroughUi(page, owner);
    const tree = await publishedTree(page, owner);

    const detail = (await (await request.get(`${API_URL}/api/v1/trees/${tree.treeId}`)).json()) as {
      nodes?: { personId: string; displayName: string }[];
    };
    const person = (detail.nodes ?? []).find((node) => node.displayName === tree.firstPersonName);
    expect(person).toBeTruthy();
    const alias = `كنية ${person!.displayName.slice(0, 6)}`;

    const strangerContext = await browser.newContext();
    const strangerPage = await strangerContext.newPage();
    try {
      await strangerPage.goto(`/tree/${tree.treeId}/versions/${tree.publishedVersionId}`);
      await strangerPage.getByPlaceholder("اكتب الفقرة أو التصحيح الذي تقترحه، مع أي سياق يساعد المراجع.").fill(`لقب «${alias}» ورد في السجل.`);
      await strangerPage.getByRole("button", { name: /أرسل المقترح/ }).click();
      await expect(strangerPage.getByRole("status")).toContainText("وصل المقترح كما كتبته");
    } finally {
      await strangerContext.close();
    }

    await page.goto(`/tree/${tree.treeId}/versions/${tree.publishedVersionId}`);
    const reviewSection = page.locator(".suggestion-review-section");
    await reviewSection.getByRole("button", { name: /قبول مع تغيير موثّق/ }).click();
    const composer = reviewSection.locator(".change-composer");
    await composer.getByLabel("الاسم البديل").fill(alias);
    await composer.getByRole("button", { name: /قبّل مع التغيير/ }).click();

    // The refusal is explained, in the panel, and the plain decision is still there.
    await expect(page.locator(".suggestion-panel-message")).toContainText("دور كتابة عام");
    await expect(composer).toContainText("دور كتابة عام");
    await expect(composer.getByRole("button", { name: /قبّل مع التغيير/ })).toBeDisabled();

    await reviewSection.getByRole("button", { name: /^قبول$/ }).click();
    await expect(page.locator(".suggestion-panel-message")).toContainText("حُفظ قرار المراجعة في سجل المقترح");

    // Nothing was written, and the decision stands: the accept-without-change-set path
    // is the one this panel has always had, and it is still there.
    const aliases = await readRows<{ value_ar: string }>(
      `SELECT value_ar FROM person_aliases WHERE person_id = $1 AND value_ar = $2`,
      [person!.personId, alias],
    );
    expect(aliases, "a refused change set still wrote an alias").toHaveLength(0);
    const stored = await readRows<{ target: string }>(
      `SELECT c.target FROM suggestion_change_sets c JOIN suggestions s ON s.id = c.suggestion_id WHERE s.tree_id = $1`,
      [tree.treeId],
    );
    expect(stored, "a refused change set was still stored").toHaveLength(0);
  });
});
