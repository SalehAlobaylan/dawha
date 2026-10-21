import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createHash, webcrypto } from "node:crypto";
import { test } from "node:test";
import { JSDOM } from "jsdom";

const html = await readFile(new URL("./label.html", import.meta.url), "utf8");
const script = await readFile(new URL("./label.js", import.meta.url), "utf8");
const sourceImage = Buffer.from("synthetic chart source page");
const sha256 = (value) => createHash("sha256").update(value).digest("hex");

function createWorkspace() {
  const downloads = [];
  const dom = new JSDOM(html, {
    runScripts: "outside-only",
    url: "http://localhost/label.html",
    beforeParse(window) {
      Object.defineProperty(window, "crypto", { configurable: true, value: webcrypto });
      window.HTMLCanvasElement.prototype.getContext = () => ({
        setTransform() {}, fillRect() {}, drawImage() {}, save() {}, restore() {},
        strokeRect() {}, beginPath() {}, moveTo() {}, lineTo() {}, stroke() {}, arc() {}, fill() {}, setLineDash() {},
      });
      window.HTMLImageElement.prototype.decode = async () => {};
      Object.defineProperty(window.HTMLImageElement.prototype, "naturalWidth", { configurable: true, get: () => 220 });
      Object.defineProperty(window.HTMLImageElement.prototype, "naturalHeight", { configurable: true, get: () => 220 });
      Object.defineProperty(window.File.prototype, "arrayBuffer", {
        configurable: true,
        value() {
          return new Promise((resolve, reject) => {
            const reader = new window.FileReader();
            reader.onload = () => resolve(reader.result);
            reader.onerror = () => reject(reader.error);
            reader.readAsArrayBuffer(this);
          });
        },
      });
      window.URL.createObjectURL = (blob) => { window.__downloadBlob = blob; return "blob:labeler-test"; };
      window.HTMLAnchorElement.prototype.click = function click() { downloads.push({ href: this.href, name: this.download }); };
    },
  });
  dom.window.eval(script);
  return { dom, downloads };
}

function annotation(annotatorId, secondName, evidence, { coverage = true } = {}) {
  return {
    schema_version: 2,
    document_id: "synthetic-chart",
    source_class: "chart",
    split: "development",
    annotation_mode: "blind_independent",
    annotation_policy: "Synthetic test labels; no machine output shown.",
    annotator_id: annotatorId,
    pages: [{ page_id: "p1", page_number: 1, width: 220, height: 220, image_sha256: sha256(sourceImage), text_ar: "", text_status: "unreadable", coverage_confirmed: coverage, processed: true }],
    nodes: [
      { id: `${annotatorId}-n1`, page_id: "p1", name_ar: "سالم", name_status: "readable", unreadable_spans_ar: "", region: { x: 0.2, y: 0.25, width: 0.25, height: 0.12 } },
      { id: `${annotatorId}-n2`, page_id: "p1", name_ar: secondName, name_status: "readable", unreadable_spans_ar: "", region: { x: 0.55, y: 0.58, width: 0.25, height: 0.12 } },
    ],
    branches: [{ id: `${annotatorId}-b1`, page_id: "p1", region: { x: 0.1, y: 0.2, width: 0.8, height: 0.6 }, label: "فرع" }],
    connectors: [{ id: `${annotatorId}-c1`, page_id: "p1", path: [[0.32, 0.37], [0.45, 0.48], [0.67, 0.58]], kind: "curve", support_notes: "خط ظاهر" }],
    edges: [{ id: `${annotatorId}-e1`, page_id: "p1", subject_id: `${annotatorId}-n1`, predicate: "parent_of", object_id: `${annotatorId}-n2`, evidence_ar: evidence }],
  };
}

function setFiles(window, input, files) {
  Object.defineProperty(input, "files", { configurable: true, value: files });
  input.dispatchEvent(new window.Event("change", { bubbles: true }));
}

function uploadJSON(window, selector, value, name) {
  const file = new window.File([JSON.stringify(value)], name, { type: "application/json" });
  setFiles(window, window.document.querySelector(selector), [file]);
}

function uploadImage(window, selector, bytes = sourceImage, name = "source.png") {
  const file = new window.File([bytes], name, { type: "image/png" });
  setFiles(window, window.document.querySelector(selector), [file]);
}

async function waitFor(predicate, message) {
  for (let attempt = 0; attempt < 50; attempt += 1) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  assert.fail(message);
}

async function loadReviews(window, readerA = annotation("reader-a", "ناصر", "خط أبوي ظاهر"), readerB = annotation("reader-b", "نصير", "خط أبوي متصل")) {
  uploadJSON(window, "#review-a", readerA, "reader-a.json");
  uploadJSON(window, "#review-b", readerB, "reader-b.json");
  await waitFor(() => window.document.querySelector("#adjudication-status").textContent.includes("reader-a + reader-b"), "The two independent reviews did not load.");
}

test("adjudication requires the exact source, explicit relationship review, and accepted endpoints", async () => {
  const { dom, downloads } = createWorkspace();
  const { window } = dom;
  try {
    window.document.querySelector("#adjudicator-id").value = "reviewer-c";
    window.document.querySelector("#adjudicator-id").dispatchEvent(new window.Event("input", { bubbles: true }));
    await loadReviews(window);

    const exportButton = window.document.querySelector("#export-gold");
    assert.equal(exportButton.disabled, true, "gold must wait for the exact rendered page image");
    uploadImage(window, "#adjudication-images", Buffer.from("different image, same dimensions"), "wrong.png");
    await waitFor(() => window.document.querySelector("#adjudication-page").textContent.includes("wrong.png"), "The source mismatch was not rendered.");
    assert.match(window.document.querySelector("#conflict-list").textContent, /Image fingerprint does not match/);
    assert.equal(exportButton.disabled, true, "a same-sized but different page image must not verify source");

    uploadImage(window, "#adjudication-images");
    await waitFor(() => window.document.querySelector("#adjudication-page").textContent.includes("source.png") && window.document.querySelector("#adjudication-summary").textContent.includes("0 pages without two full reviews"), "Matching source fingerprint was not accepted.");
    assert.equal(exportButton.disabled, true, "name and relationship issues must remain unresolved");
    const confirmRelation = window.document.querySelector('[data-resolve="edge-0"][data-choice="confirmed"]');
    assert.ok(confirmRelation, "matching reader relationships still require adjudicator confirmation");
    window.document.querySelector('[data-inspect="edge-0"]').click();
    assert.equal(window.document.querySelectorAll("#adjudication-overlay rect").length, 4, "relationship review should highlight both readers' endpoints");
    assert.equal(window.document.querySelectorAll("#adjudication-overlay polyline").length, 2, "relationship review should show both readers' connector traces");
    confirmRelation.click();
    assert.equal(exportButton.disabled, true, "the unrelated name disagreement still needs adjudication");

    window.document.querySelector('[data-resolve="node-1"][data-choice="a"]').click();
    assert.equal(exportButton.disabled, false, "export can proceed when every issue is resolved");
    window.document.querySelector('[data-resolve="node-1"][data-choice="absent"]').click();
    assert.equal(exportButton.disabled, true, "a confirmed relationship cannot outlive an excluded endpoint");
    assert.match(window.document.querySelector("#adjudication-summary").textContent, /1 confirmed relations without accepted endpoints/);
    window.document.querySelector('[data-resolve="edge-0"][data-choice="pending"]').click();
    window.document.querySelector('[data-resolve="edge-0"][data-choice="absent"]').click();
    assert.equal(exportButton.disabled, false, "excluding the unsupported relation resolves its missing endpoint");

    exportButton.click();
    assert.equal(downloads.at(-1)?.name, "synthetic-chart.gold.jsonl");
    const jsonl = await new Promise((resolve, reject) => {
      const reader = new window.FileReader();
      reader.onload = () => resolve(reader.result);
      reader.onerror = () => reject(reader.error);
      reader.readAsText(window.__downloadBlob);
    });
    const gold = JSON.parse(jsonl.trim());
    assert.equal(gold.pages[0].image_sha256, sha256(sourceImage));
    assert.ok(gold.adjudication.decisions.some((item) => item.kind === "edge" && item.decision === "absent"));
  } finally {
    window.close();
  }
});

test("machine processed status cannot replace either reader's full-page confirmation", async () => {
  const { dom } = createWorkspace();
  const { window } = dom;
  try {
    window.document.querySelector("#adjudicator-id").value = "reviewer-c";
    window.document.querySelector("#adjudicator-id").dispatchEvent(new window.Event("input", { bubbles: true }));
    await loadReviews(window, annotation("reader-a", "ناصر", "خط أبوي ظاهر", { coverage: false }));
    uploadImage(window, "#adjudication-images");
    await waitFor(() => window.document.querySelector("#adjudication-page").textContent.includes("source.png") && window.document.querySelector("#adjudication-summary").textContent.includes("1 pages without two full reviews"), "An unconfirmed reader page was not recorded.");
    assert.equal(window.document.querySelector("#export-gold").disabled, true);
  } finally {
    window.close();
  }
});

test("independent exports fingerprint the exact source image bytes", async () => {
  const { dom } = createWorkspace();
  const { window } = dom;
  try {
    const docId = window.document.querySelector("#document-id");
    docId.value = "synthetic-chart";
    docId.dispatchEvent(new window.Event("input", { bubbles: true }));
    const annotatorId = window.document.querySelector("#annotator-id");
    annotatorId.value = "reader-a";
    annotatorId.dispatchEvent(new window.Event("input", { bubbles: true }));
    uploadImage(window, "#page-files");
    await waitFor(() => window.document.querySelector("#page-count").textContent === "1", "The source image did not load in independent annotation mode.");
    const coverage = window.document.querySelector("#coverage-confirmed");
    coverage.checked = true;
    coverage.dispatchEvent(new window.Event("change", { bubbles: true }));
    const exportButton = window.document.querySelector("#export-annotation");
    assert.equal(exportButton.disabled, false);
    exportButton.click();
    const json = await new Promise((resolve, reject) => {
      const reader = new window.FileReader();
      reader.onload = () => resolve(reader.result);
      reader.onerror = () => reject(reader.error);
      reader.readAsText(window.__downloadBlob);
    });
    const exported = JSON.parse(json);
    assert.equal(exported.pages[0].image_sha256, sha256(sourceImage));
    assert.equal(exported.pages[0].width, 220);
    assert.equal(exported.pages[0].height, 220);
  } finally {
    window.close();
  }
});
