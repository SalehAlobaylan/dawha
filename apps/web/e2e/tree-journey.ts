import { expect, type Page } from "@playwright/test";

import { API_URL, type Account } from "./fixtures";

/**
 * The tree half of the acceptance journeys: creating a public tree, publishing
 * it, and reading back its ids.
 *
 * It lives outside the spec files because Playwright refuses to let one spec
 * import another, and the fork and suggestion journeys both need a published
 * public tree to work on. Every assertion is on something the reader can still
 * see a second later - the heading, the version timeline, the canvas - because
 * the action banner is replaced as soon as the query cache is invalidated.
 */

export interface PublishedTree {
  account: Account;
  treeName: string;
  treeId: string;
  publishedVersionId: string;
  draftVersionId: string;
  firstPersonName: string;
}

/** Creates a public tree with one person, publishes it, and returns its ids. */
export async function publishedTree(page: Page, account: Account): Promise<PublishedTree> {
  const treeName = `شجرة ${account.displayName}`;
  const firstPersonName = `سعد ${account.displayName.slice(-6)}`;

  await page.goto("/tree");
  await page.getByRole("button", { name: "شجرة جديدة" }).click();
  await page.getByLabel("اسم الشجرة").fill(treeName);
  await page.getByLabel("الوصف").fill("شجرة تفسيرية اختبارية من رحلة القبول.");
  await page.getByLabel("الظهور").selectOption("public");
  await page.getByLabel("أول شخص (اختياري)").fill(firstPersonName);
  await page.getByRole("button", { name: /احفظ كمسودة/ }).click();

  // Creating a tree opens it as a draft with its first person on the canvas.
  await expect(page.getByRole("heading", { name: treeName })).toBeVisible();
  await expect(page.getByRole("button", { name: `فتح ${firstPersonName}` })).toBeVisible();
  const treeId = treeIdFromUrl(page.url());
  const draftVersionId = versionIdFromUrl(page.url());
  await expect(page.getByRole("list", { name: "نسخ الشجرة" }).getByText("مسودة حالية")).toBeVisible();

  await page.getByRole("button", { name: /نشر المسودة/ }).click();

  // Publishing closes the draft as version 1 and opens draft 2, which is the
  // shape the fork journey depends on. The timeline marks the sealed version as
  // published and the new one as the editable draft.
  const timeline = page.getByRole("list", { name: "نسخ الشجرة" });
  await expect(timeline.getByText("v2")).toBeVisible();
  await expect(timeline.getByText("مسودة حالية")).toBeVisible();
  await expect(timeline.getByText("نسخة منشورة")).toBeVisible();
  // The status badge and the version line are separate elements, so the version
  // text is matched on its own.
  await expect(page.getByText("النسخة 2 من 2")).toBeVisible();

  const publishedVersionId = await publishedVersionIdFor(page, treeId);
  return { account, treeName, treeId, publishedVersionId, draftVersionId, firstPersonName };
}

/** Reads the published version id of a tree through the API the UI itself uses. */
export async function publishedVersionIdFor(page: Page, treeId: string): Promise<string> {
  const detail = await page.request.get(`${API_URL}/api/v1/trees/${treeId}`);
  expect(detail.status(), `read tree ${treeId}`).toBe(200);
  const body = (await detail.json()) as { versions?: { id: string; state: string }[] };
  const published = (body.versions ?? []).find((version) => version.state === "published");
  expect(published, "the tree has no published version").toBeTruthy();
  return published!.id;
}

/** Reads the current draft version id of a tree through the API the UI uses. */
export async function draftVersionIdFor(page: Page, treeId: string): Promise<string> {
  const detail = await page.request.get(`${API_URL}/api/v1/trees/${treeId}`);
  expect(detail.status(), `read tree ${treeId}`).toBe(200);
  const body = (await detail.json()) as { versions?: { id: string; state: string }[] };
  const draft = (body.versions ?? []).find((version) => version.state === "draft");
  expect(draft, "the tree has no draft version").toBeTruthy();
  return draft!.id;
}

export function treeIdFromUrl(url: string): string {
  const match = /\/tree\/([0-9a-f-]{36})/.exec(url);
  expect(match, `no tree id in ${url}`).toBeTruthy();
  return match![1];
}

export function versionIdFromUrl(url: string): string {
  const match = /\/versions\/([0-9a-f-]{36})/.exec(url);
  expect(match, `no version id in ${url}`).toBeTruthy();
  return match![1];
}
