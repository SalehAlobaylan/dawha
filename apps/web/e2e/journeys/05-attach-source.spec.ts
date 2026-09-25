import { expect, test } from "@playwright/test";

import { API_URL, newAccount, registerThroughUi, waitForApi, waitForProcessing } from "../fixtures";

/**
 * Journey 7 of the declared V1 list: attach a source.
 *
 * The file is a .txt upload because that is what the source-format contract
 * accepts: text/* plus JSON and XML, everything else refused with the same list
 * the API would give. The journey covers both halves, because a format contract
 * nobody refuses is a contract that does not exist.
 */

/** A two-page Arabic manuscript, split the way the text extractor splits pages. */
const MANUSCRIPT = "قال ابن سعد: أبو بكر هو والد عبدالله.\fوفي الصفحة الثانية: هاجر سعد إلى الرياض.";

test.describe("attach a source", () => {
  test("a researcher creates a source and uploads a text file to it", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("source");
    await registerThroughUi(page, account);

    await page.goto("/sources");
    const workspace = page.getByText("مساحة المصدر والدليل");
    await expect(workspace).toBeVisible();
    await page.getByRole("button", { name: /مصدر جديد/ }).click();

    const title = `مخطوط ${account.displayName}`;
    await page.getByLabel("العنوان").fill(title);
    await page.getByRole("button", { name: /احفظ المصدر/ }).click();

    // The saved source lands in the library and is opened, so the upload panel
    // appears for it. Both the library row and the detail pane carry the title,
    // so each is scoped to the region it belongs to.
    const library = page.locator(".evidence-source-list");
    await expect(library.getByRole("button", { name: new RegExp(title) })).toBeVisible();
    await expect(page.locator(".evidence-source-detail").getByRole("heading", { name: title })).toBeVisible();

    // The panel states the accepted formats before the caller picks a file, so a
    // refusal is never a surprise.
    await expect(page.getByText("الصيغ المدعومة: نص (text/*) وملفات JSON و XML")).toBeVisible();

    const sourceId = await sourceIdOf(page, title);
    await page.locator('input[type="file"]').setInputFiles({
      name: "سجل.txt",
      mimeType: "text/plain",
      buffer: Buffer.from(MANUSCRIPT, "utf8"),
    });
    await page.getByRole("button", { name: /رفع ومعالجة/ }).click();

    // The upload is acknowledged and a processing run is queued for it. The
    // workspace and the upload panel each carry a status line, so the assertion
    // is scoped to the panel that owns the upload.
    const panelStatus = page.locator(".source-processing-panel").getByRole("status");
    await expect(panelStatus).toContainText("تم رفع الملف");
    const file = await waitForProcessing(request, page, sourceId, 5_000);
    expect(["queued", "running", "succeeded", "failed"]).toContain(file);

    const detail = (await (await page.request.get(`${API_URL}/api/v1/sources/${sourceId}`)).json()) as {
      source: { titleAr: string; sourceType: string };
    };
    expect(detail.source.titleAr).toBe(title);
  });

  test("the accepted .txt file reaches the processing pipeline", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("source-process");
    await registerThroughUi(page, account);

    const sourceId = await createSourceThroughUi(page, account);
    await page.locator('input[type="file"]').setInputFiles({
      name: "مخطوط.txt",
      mimeType: "text/plain",
      buffer: Buffer.from(MANUSCRIPT, "utf8"),
    });
    await page.getByRole("button", { name: /رفع ومعالجة/ }).click();
    await expect(page.locator(".source-processing-panel").getByRole("status")).toContainText("تم رفع الملف");

    // The worker is a separate process, so the run may still be queued when the
    // browser moves on. The API is the place the outcome is decided, and it must
    // never read as silently finished either way.
    const outcome = await waitForProcessing(request, page, sourceId, 45_000);
    test.info().annotations.push({ type: "processing outcome", description: outcome });
    expect(outcome, "the queued run never reached a terminal state").not.toBe("still running");

    const processing = (await (await page.request.get(`${API_URL}/api/v1/sources/${sourceId}/processing`)).json()) as {
      runs: { status: string; passageCount: number; error?: string }[];
    };
    const run = processing.runs[0];
    expect(run.status).toBe("succeeded");
    // Two form-feed separated pages become two passages, which is the shape the
    // source-processing path promises.
    expect(run.passageCount).toBe(2);
    expect(run.error ?? "").toBe("");

    // The candidates are waiting for a person, and each one still offers an
    // explicit accept or reject. The model never decides.
    const panel = page.locator(".source-processing-panel");
    await expect(panel.getByText(/مرشح$/)).toBeVisible();
    await expect(panel.getByRole("button", { name: /قبول/ }).first()).toBeVisible();
    await expect(panel.getByRole("button", { name: /رفض/ }).first()).toBeVisible();
    expect(outcome).toBe("succeeded");
  });

  test("an unsupported format is refused with the list of accepted ones", async ({ page, request }) => {
    await waitForApi(request);
    const account = newAccount("source-refuse");
    await registerThroughUi(page, account);
    await createSourceThroughUi(page, account);

    const sourceId = await createSourceThroughUi(page, account);
    const fileInput = page.locator(".source-processing-panel input[type=\"file\"]");
    await fileInput.setInputFiles({
      name: "سجل.pdf",
      mimeType: "application/pdf",
      buffer: Buffer.from("%PDF-1.7\nbinary", "utf8"),
    });

    // The form refuses the file before it is sent, naming the refused format and
    // listing what is accepted, so the caller never has to guess.
    const refusal = page.locator(".source-processing-panel").getByRole("status");
    await expect(refusal).toContainText("application/pdf");
    await expect(refusal).toContainText("text/*");
    await expect(refusal).toContainText("JSON");
    await expect(refusal).toContainText("XML");
    await expect(page.getByRole("button", { name: /رفع ومعالجة/ })).toBeDisabled();

    // The API refuses the same file the same way, so a caller that skips the
    // form cannot get further than the browser could.
    const direct = await page.request.post(`${API_URL}/api/v1/sources/${sourceId}/files`, {
      multipart: {
        file: {
          name: "سجل.pdf",
          mimeType: "application/pdf",
          buffer: Buffer.from("%PDF-1.7\nbinary", "utf8"),
        },
      },
    });
    // 415 is the status the upload handler returns for a refused format; what
    // matters is that it is a refusal and that the body teaches the caller.
    expect(direct.status()).toBe(415);
    const body = (await direct.json()) as { error?: string };
    expect(body.error ?? "").toContain("application/pdf");
    expect(body.error ?? "").toContain("text/*");
    expect(body.error ?? "").toContain("application/json");
    expect(body.error ?? "").toContain("application/xml");
  });
});

/** Creates a source through the UI and returns its id. */
async function createSourceThroughUi(page: import("@playwright/test").Page, account: { displayName: string }): Promise<string> {
  await page.goto("/sources");
  await page.getByRole("button", { name: /مصدر جديد/ }).click();
  const title = `مخطوط ${account.displayName}`;
  await page.getByLabel("العنوان").fill(title);
  await page.getByRole("button", { name: /احفظ المصدر/ }).click();
  await expect(page.locator(".evidence-source-detail").getByRole("heading", { name: title })).toBeVisible();
  return sourceIdOf(page, title);
}

/** Reads a saved source's id from the library entry the UI renders for it. */
async function sourceIdOf(page: import("@playwright/test").Page, title: string): Promise<string> {
  const body = (await (await page.request.get(`${API_URL}/api/v1/sources`)).json()) as {
    items: { id: string; titleAr: string }[];
  };
  const libraries = body.items;
  const found = libraries.filter((source) => source.titleAr === title).pop();
  expect(found, `no saved source titled ${title}`).toBeTruthy();
  return found!.id;
}
