import { expect, test } from "@playwright/test";

import { waitForApi } from "../fixtures";

/**
 * Journey 10 of the declared V1 list: browse the map.
 *
 * The map page has to work without a session, because a reader who has not
 * signed in is exactly the reader the product is for. The canvas is asserted
 * through its accessible name and the place list through its buttons, not
 * through pixels: a WebGL assertion would pass on a blank canvas and fail on a
 * slow one.
 */
test.describe("browse the map", () => {
  test("a reader sees the places, the layers and the uncertainty note", async ({ page, request }) => {
    await waitForApi(request);
    await page.context().clearCookies();
    await page.goto("/places");

    // The map is announced as a map, and it is drawn without any network tile
    // service, so the journey needs no credentials and no public deployment.
    await expect(page.getByRole("heading", { name: "الموضع جزء من السؤال" })).toBeVisible();
    const canvas = page.getByLabel("خريطة مواضع البحث");
    await expect(canvas).toBeVisible();

    // The layers are the uncertainty story: a dotted route is a proposal, not a
    // border, and the page says so.
    await expect(page.getByText("المسارات المنقطة استنتاجات، وليست حدوداً تاريخية.")).toBeVisible();
    await expect(page.getByText("كيف نعرض عدم اليقين؟")).toBeVisible();

    const markers = page.locator(".map-marker");
    const count = await markers.count();
    expect(count).toBeGreaterThan(0);
  });

  test("choosing a place shows its detail and keeps the layer toggles honest", async ({ page }) => {
    await page.goto("/places");
    const firstCard = page.locator(".place-index-card").first();
    const placeName = (await firstCard.locator("strong").innerText()).trim();
    await firstCard.click();

    // The selected place is named in the sidebar and its marker is marked active.
    await expect(page.locator(".place-detail-title").getByRole("heading", { name: placeName })).toBeVisible();
    const marker = page.locator(`.map-marker[aria-label="${placeName}"]`);
    await expect(marker).toHaveClass(/map-marker-active/);

    // Turning a layer off removes it from the active set rather than silently
    // doing nothing.
    const pill = page.locator(".map-layer-pill").first();
    await expect(pill).toHaveClass(/map-layer-pill-active/);
    await pill.click();
    await expect(pill).not.toHaveClass(/map-layer-pill-active/);
  });

  test("the place search narrows the index, and the period control filters it too", async ({ page }) => {
    await page.goto("/places");
    const cards = page.locator(".place-index-card");
    const everyPlace = await cards.count();
    expect(everyPlace).toBeGreaterThan(1);
    const search = page.getByLabel("ابحث عن موضع");

    // Typing a place name narrows the index to the places that carry it, rather
    // than leaving the list untouched.
    await search.fill("الأحساء");
    await expect(cards).toHaveCount(1);
    await expect(cards.first()).toContainText("الأحساء");

    // Clearing it gives the whole index back, so the filter is not a one-way door.
    await search.fill("");
    await expect(cards).toHaveCount(everyPlace);

    // The period control filters the same index: every option it offers has to
    // leave the places of that period behind.
    const period = page.getByLabel("تصفية بالفترة");
    await period.selectOption("القرن الثاني عشر");
    await expect(cards).toHaveCount(1);
    await expect(cards.first()).toContainText("الأحساء");
    await period.selectOption("كل الفترات");
    await expect(cards).toHaveCount(everyPlace);

    // A search that matches nothing says so, rather than showing an empty grid
    // that reads like a page that failed to load.
    await search.fill("مدينة لا وجود لها");
    await expect(cards).toHaveCount(0);
    await expect(page.getByText("لا توجد نتائج مطابقة")).toBeVisible();
  });

  test("a marker on the canvas selects the same place as the list", async ({ page }) => {
    await page.goto("/places");
    const card = page.locator(".place-index-card").nth(1);
    const placeName = (await card.locator("strong").innerText()).trim();
    const marker = page.locator(`.map-marker[aria-label="${placeName}"]`);
    await expect(marker).toBeVisible();

    // The canvas and the list are two views of the same selection, so choosing
    // on the map moves the sidebar.
    await marker.click();
    await expect(page.locator(".place-detail-title").getByRole("heading", { name: placeName })).toBeVisible();
    // The sidebar swaps to the place the marker named, including its type and
    // period, which is the evidence line the reader is meant to check.
    await expect(page.locator(".place-detail-title")).toContainText("منطقة");
  });
});
