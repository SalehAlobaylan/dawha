import { expect, test } from "@playwright/test";

import { newAccount, registerThroughUi, waitForApi } from "../fixtures";

/**
 * Journey 11 of the declared V1 list: run a research query.
 *
 * The point of this journey is not that an answer appears. It is that the answer
 * arrives with its material attached and its own limits stated, so a reader can
 * tell a sourced statement from a guess. The assertions below check the
 * provenance panel and the refusal to answer without material, not the prose.
 */
test.describe("run a research query", () => {
  test("a query returns an answer with its material and its routing stated", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("research");
    await registerThroughUi(page, account);

    await page.goto("/research");
    const form = page.locator(".research-query-form");
    await expect(form).toBeVisible();
    const question = `من كان والد عبدالله في سجل ${account.displayName}؟`;
    await form.locator("#research-question").fill(question);
    await form.getByRole("button", { name: /ابحث في الأدلة/ }).click();

    const result = page.locator(".research-result-card");
    await expect(result).toBeVisible();
    // The card is a live region, so the answer is announced rather than swapped
    // in silently.
    await expect(result).toHaveAttribute("aria-live", "polite");
    await expect(result.getByRole("heading", { name: question })).toBeVisible();
    await expect(result.locator(".research-result-answer")).not.toBeEmpty();

    // Every number in the strip is a count the reader can check against the
    // material listed underneath it.
    const stats = result.locator(".research-result-stats");
    await expect(stats).toContainText("مادة");
    await expect(stats).toContainText("مرشح");
    await expect(stats).toContainText("تعارض");

    // The routing decision is shown rather than hidden, so a reader knows
    // whether the answer went through the cheap or the deep path.
    await expect(stats).toContainText(/مسار عميق|مسار سريع|متجاهل/);
  });

  test("a query about nothing in the corpus says it cannot answer", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/research");
    const form = page.locator(".research-query-form");
    // A term that appears nowhere in the corpus, AND that is too short for the
    // retrieval index to manufacture a match for it.
    //
    // The length is load-bearing and it was learned the hard way. The passage legs
    // score with pg_trgm CHARACTER trigrams - `similarity(normalized_text, query)`
    // - and their only filter is `> 0`, so ANY query of three characters or more
    // shares a trigram with something in a corpus of ordinary sentences and is
    // therefore a candidate. This journey used to assert a seven character
    // nonsense word; against the three-passage demo seed it matched nothing, and
    // against a larger corpus it matched three passages and the product answered
    // instead of refusing, which is correct behaviour and a broken journey.
    //
    // Two characters cannot produce a trigram, so this reaches the no-evidence
    // branch for a reason that does not depend on what else is in the corpus.
    // `docs/retrieval-measurement.md` records the same property from the other
    // side: it is why a "nothing matches" query is not expressible as a long term
    // at all.
    //
    // The other precondition is that the corpus carries no embeddings. A hashed
    // embedding is positive for roughly half of any corpus, so an embedded corpus
    // cannot reach this branch either - and `make e2e` does not embed, because the
    // measurement corpus and its embeddings are provisioned by
    // `make retrieval-report` into a schema it drops again.
    await form.locator("#research-question").fill("زر");
    await form.getByRole("button", { name: /ابحث في الأدلة/ }).click();

    const result = page.locator(".research-result-card");
    await expect(result).toBeVisible();
    // The badge is the honest outcome: not enough evidence, rather than a
    // confident sentence, and no material is listed to back one.
    await expect(result).toContainText("أدلة غير كافية");
    // The container is always rendered; what must be absent is any group of
    // cited material behind the answer.
    await expect(result.locator(".research-evidence-group")).toHaveCount(0);
    await expect(result.locator(".research-result-stats")).toContainText("0 مادة");
  });

  test("an empty question cannot be submitted", async ({ page, request }) => {
    await waitForApi(request);
    await page.goto("/research");
    const form = page.locator(".research-query-form");
    // The submit control is disabled until there is something to ask, so no
    // request is spent on an empty question.
    await expect(form.getByRole("button", { name: /ابحث في الأدلة/ })).toBeDisabled();
    await form.locator("#research-question").fill("ماذا نعرف عنhatt 这个؟");
    await expect(form.getByRole("button", { name: /ابحث في الأدلة/ })).toBeEnabled();
  });
});
