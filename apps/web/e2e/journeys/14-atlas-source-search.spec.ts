import { randomUUID } from "node:crypto";
import { expect, test } from "@playwright/test";

import { API_URL, executeSql, grantPlatformRole, loginThroughUi, newAccount, registerThroughApi, sessionCookieHeader } from "../fixtures";

test("clicking a tribe label on the map reveals cross-source mentions and comparison", async ({ page }) => {
  await page.goto("/places?plates=admiralty-1916-tribal");
  const filterSearch = page.getByLabel("ابحث بموضع أو كيان");
  await filterSearch.fill("مطير");
  await page.getByRole("region", { name: "أسماء مطابقة على اللوحات" }).getByRole("button", { name: /مطير/ }).click();

  const panel = page.getByLabel("قراءة اسم من اللوحة");
  await expect(panel).toBeVisible();
  const otherSources = panel.getByLabel("شواهد الاسم في مصادر أخرى");
  await expect(otherSources).toBeVisible();
  const qahtaniItem = otherSources.getByRole("listitem").filter({ hasText: "الأوضاع الاقتصادية والاجتماعية" });
  await expect(qahtaniItem).toBeVisible();
  await expect(qahtaniItem.getByRole("link", { name: "افتح المصدر عند الشاهد" })).toHaveAttribute("href", /#page=108$/);

  const dicksonItem = otherSources.getByRole("listitem").filter({ hasText: "رسم تخطيطي للمناطق القبلية" });
  await expect(dicksonItem).toContainText("قراءة من الفهرس");
  await dicksonItem.getByRole("button", { name: "قارن مع اللوحة" }).click();
  await expect(dicksonItem.getByText("استقلال المصدرين غير محقّق.", { exact: false })).toBeVisible();
});

test("a moderator reviews and publishes an extracted reading from the region workbench", async ({ page, request }) => {
  const moderator = newAccount("atlas-extraction-review");
  await registerThroughApi(request, moderator);
  await grantPlatformRole(moderator.email, "moderator");
  await loginThroughUi(page, moderator);

  const id = randomUUID();
  await executeSql(`INSERT INTO sources (id,title_ar,source_type,visibility,created_by)
    VALUES ($1,'لوحة اختبار الاستيراد','book','public',(SELECT id FROM users WHERE email=$2))`, [id, moderator.email]);
  await executeSql(`WITH item AS (
    INSERT INTO atlas_source_items(source_id,item_key) VALUES ($1,'stage-test') RETURNING id
  ) INSERT INTO atlas_source_item_revisions(item_id,revision_key,locator_ar,external_url,rights_ar,credit_ar)
    SELECT id,'v1','ص ١','https://example.org/page','رابط','المؤلف' FROM item`, [id]);

  const response = await request.post(`${API_URL}/api/v1/admin/atlas/sources/${id}/proposals`, {
    headers: { Cookie: await sessionCookieHeader(page) },
    data: {
      itemKey: "stage-test", sha256: "8".repeat(64), method: "tesseract", engine: "fixture OCR", frame: {},
      proposals: [{ original: "Staged label", selector: { pixelRect: [10, 20, 30, 40] } }],
    },
  });
  expect(response.ok()).toBeTruthy();
  expect(await response.json()).toEqual({ added: 1 });

  await page.goto("/region-workbench");
  await page.getByText("مراجعة قراءات المصادر", { exact: true }).click();
  const card = page.getByRole("article").filter({ hasText: "لوحة اختبار الاستيراد" });
  await card.getByLabel("الاسم المعروض بالعربية").fill("اسم اختبار الاستيراد");
  await card.getByLabel("لغة النص الأصلي").selectOption("en");
  await card.getByLabel("سبب القرار").fill("راجعت القراءة في المصدر");
  await card.getByRole("button", { name: "انشر القراءة" }).click();
  await expect(card).toHaveCount(0);

  const published = await request.get(`${API_URL}/api/v1/atlas/occurrences?q=${encodeURIComponent("اسم اختبار الاستيراد")}`);
  expect(published.ok()).toBeTruthy();
  const body = await published.json() as { occurrences: Array<{ id: string; original: string }> };
  expect(body.occurrences[0]?.original).toBe("Staged label");

  await page.goto(`/places?occurrence=${body.occurrences[0]!.id}`);
  await expect(page.getByLabel("شاهد من المصدر").getByText("Staged label", { exact: true })).toBeVisible();
});
