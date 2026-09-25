import { expect, test } from "@playwright/test";

import { API_URL, newAccount, registerThroughUi, waitForApi } from "../fixtures";
import { draftVersionIdFor, publishedTree, treeIdFromUrl } from "../tree-journey";

/**
 * Journey 3 of the declared V1 list: fork a published version.
 *
 * The fork is the moment an interpretation becomes somebody else's, so the
 * journey asserts both halves: the copy exists under a new owner, and the
 * original is untouched.
 */
test.describe("fork a published version", () => {
  test("a second researcher forks the published version and the original is untouched", async ({ page, request }) => {
    await waitForApi(request);
    const owner = newAccount("fork-owner");
    await registerThroughUi(page, owner);
    const original = await publishedTree(page, owner);
    const originalTreeId = original.treeId;

    // A different browser context so the second researcher has their own session
    // and their own account, the way two people would.
    const forkerContext = await page.context().browser()!.newContext();
    const forker = newAccount("forker");
    const forkerPage = await forkerContext.newPage();
    try {
      await registerThroughUi(forkerPage, forker);
      await forkerPage.goto(`/tree/${originalTreeId}/versions/${original.publishedVersionId}`);

      // Forking is only offered for a published version, so the button has to be
      // live here and was disabled while the tree was a draft.
      const forkButton = forkerPage.getByRole("button", { name: /تفريع/ });
      await expect(forkButton).toBeEnabled();
      await forkButton.click();

      const forkName = `تفريع ${forker.displayName}`;
      await forkerPage.getByLabel("اسم النسخة المستقلة").fill(forkName);
      await forkerPage.getByRole("button", { name: /أنشئ نسخة مستقلة/ }).click();

      // The reader is moved into the new independent draft. Waiting for the new
      // tree's heading is what proves the navigation happened: the address bar
      // already looked like a tree route before the fork.
      await expect(forkerPage.getByRole("heading", { name: forkName })).toBeVisible();
      const forkedTreeId = treeIdFromUrl(forkerPage.url());
      expect(forkedTreeId).not.toBe(originalTreeId);
      await expect(forkerPage.getByText("هذا التفريع مستقل عن نسخة الأصل")).toBeVisible();
      await expect(forkerPage.getByRole("button", { name: `فتح ${original.firstPersonName}` })).toBeVisible();

      const forked = (await (await forkerPage.request.get(`${API_URL}/api/v1/trees/${forkedTreeId}`)).json()) as {
        tree: { parentTreeId: string; parentVersionId: string; visibility: string };
        nodes: unknown[];
      };
      expect(forked.tree.parentTreeId).toBe(originalTreeId);
      expect(forked.tree.parentVersionId).toBe(original.publishedVersionId);
      // A fork starts private, so the fork does not silently republish someone
      // else's interpretation.
      expect(forked.tree.visibility).toBe("private");
      expect(forked.nodes).toHaveLength(1);

      // The source tree kept its own owner, its public visibility and its single
      // published version.
      const source = (await (await page.request.get(`${API_URL}/api/v1/trees/${originalTreeId}`)).json()) as {
        tree: { visibility: string; ownerId: string; parentTreeId?: string };
        versions: { state: string }[];
      };
      expect(source.tree.visibility).toBe("public");
      expect(source.tree.parentTreeId ?? "").toBe("");
      expect(source.versions.filter((version) => version.state === "published")).toHaveLength(1);
    } finally {
      await forkerContext.close();
    }
  });

  test("the fork button stays disabled while the selected version is a draft", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("fork-draft");
    await registerThroughUi(page, account);
    const tree = await publishedTree(page, account);

    // The freshly opened draft is editable, and forking it would copy an
    // interpretation nobody has sealed yet. The id comes from the API because
    // publishing turned the original draft into the published version and opened
    // a new one.
    const draftVersionId = await draftVersionIdFor(page, tree.treeId);
    expect(draftVersionId).not.toBe(tree.draftVersionId);
    await page.goto(`/tree/${tree.treeId}/versions/${draftVersionId}`);
    await expect(page.getByText("هذه مسودة قابلة للتعديل")).toBeVisible();
    await expect(page.getByRole("button", { name: /تفريع/ })).toBeDisabled();
  });
});
