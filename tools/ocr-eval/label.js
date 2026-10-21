(() => {
  "use strict";

  const $ = (selector) => document.querySelector(selector);
  const maxImageBytes = 80 * 1024 * 1024;
  const maxImagePixels = 180_000_000;
  const pages = [];
  const canvas = $("#page-canvas");
  const context = canvas.getContext("2d");
  let activePageId = "";
  let selected = null;
  let tool = "select";
  let drag = null;
  let connectorDraft = [];
  let zoom = 1;
  let annotationA = null;
  let annotationB = null;
  let adjudicationRows = [];
  let adjudication = null;
  let adjudicationImages = [];
  let activeAdjudicationKey = "";

  const id = (prefix) => `${prefix}-${crypto.randomUUID()}`;
  const currentPage = () => pages.find((page) => page.pageId === activePageId) || null;
  const exactText = (value) => String(value || "").normalize("NFC");
  const pageNames = (page) => page?.nodes || [];
  const pageBranches = (page) => page?.branches || [];
  const pageConnectors = (page) => page?.connectors || [];
  const escapeHtml = (value) => String(value ?? "").replace(/[&<>"']/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[char]);
  const box = (x, y, width, height) => ({ x: Math.max(0, Math.min(x, 1)), y: Math.max(0, Math.min(y, 1)), width: Math.max(0, Math.min(width, 1 - x)), height: Math.max(0, Math.min(height, 1 - y)) });
  const download = (name, content, type) => {
    const link = document.createElement("a");
    const url = URL.createObjectURL(new Blob([content], { type }));
    link.href = url; link.download = name; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };

  function safeFilename(value) {
    return String(value || "annotation").normalize("NFKC").replace(/[^\p{L}\p{N}_.-]+/gu, "-").replace(/^-+|-+$/g, "").slice(0, 80) || "annotation";
  }

  async function sha256Hex(file) {
    if (!window.crypto?.subtle || typeof file.arrayBuffer !== "function") throw new Error("This browser cannot verify source images with SHA-256. Open the tool in a current browser over localhost.");
    const digest = await window.crypto.subtle.digest("SHA-256", await file.arrayBuffer());
    return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, "0")).join("");
  }

  function showMessage(title, message) {
    $("#dialog-title").textContent = title;
    $("#dialog-message").textContent = message;
    $("#status-dialog").showModal();
  }

  function pageRecord(page) {
    return {
      page_id: page.pageId,
      page_number: page.pageNumber,
      width: page.width || null,
      height: page.height || null,
      image_sha256: page.imageSha256 || "",
      text_ar: page.textAR || "",
      text_status: page.textStatus || "unreadable",
      coverage_confirmed: page.coverageConfirmed === true,
      nodes: pageNames(page).map((item) => ({ ...item, region: { ...item.region } })),
      branches: pageBranches(page).map((item) => ({ ...item, region: { ...item.region } })),
      connectors: pageConnectors(page).map((item) => ({ ...item, path: item.path.map((point) => [...point]) })),
      edges: (page.edges || []).map((item) => ({ ...item })),
    };
  }

  function independentExport() {
    return {
      schema_version: 2,
      document_id: $("#document-id").value.trim(),
      source_class: $("#source-class").value,
      split: $("#split").value,
      annotator_id: $("#annotator-id").value.trim(),
      annotation_mode: "blind_independent",
      annotation_policy: "No machine proposals or other annotator labels were shown.",
      pages: pages.map(pageRecord),
      nodes: pages.flatMap((page) => pageNames(page).map((item) => ({ ...item, page_id: page.pageId, region: { ...item.region } }))),
      branches: pages.flatMap((page) => pageBranches(page).map((item) => ({ ...item, page_id: page.pageId, region: { ...item.region } }))),
      connectors: pages.flatMap((page) => pageConnectors(page).map((item) => ({ ...item, page_id: page.pageId, path: item.path.map((point) => [...point]) }))),
      edges: pages.flatMap((page) => (page.edges || []).map((item) => ({ ...item, page_id: page.pageId }))),
    };
  }

  function updatePageMeta() {
    const page = currentPage();
    if (!page) return;
    $("#coverage-confirmed").checked = page.coverageConfirmed === true;
    $("#active-page-title").textContent = `${page.pageId} · ${page.fileName || "source image"}`;
    $("#image-dimensions").textContent = page.width && page.height ? `${page.width} × ${page.height} px` : "Image not loaded";
    $("#canvas-empty").hidden = Boolean(page.image);
    canvas.hidden = !page.image;
    drawCanvas();
    renderPageList();
    renderSelection();
    renderOccurrenceList();
    renderRelationships();
    updateTotals();
  }

  function renderPageList() {
    const host = $("#page-list");
    $("#page-count").textContent = String(pages.length);
    if (!pages.length) { host.innerHTML = '<div class="empty-pages">No pages loaded</div>'; return; }
    host.innerHTML = pages.map((page) => `<button class="page-item ${page.pageId === activePageId ? "active" : ""}" data-page="${escapeHtml(page.pageId)}"><span class="page-chip">${page.pageNumber}</span><span>${escapeHtml(page.pageId)}</span><small>${page.coverageConfirmed ? "FULL REVIEW" : "PENDING"}</small></button>`).join("");
    host.querySelectorAll("[data-page]").forEach((button) => button.addEventListener("click", () => {
      if (connectorDraft.length) finishConnector();
      activePageId = button.dataset.page; selected = null; zoom = 1; updatePageMeta();
    }));
  }

  function updateTotals() {
    const page = currentPage();
    const allNodes = pages.reduce((sum, item) => sum + pageNames(item).length, 0);
    const allBranches = pages.reduce((sum, item) => sum + pageBranches(item).length, 0);
    const allConnectors = pages.reduce((sum, item) => sum + pageConnectors(item).length, 0);
    $("#name-total").textContent = String(allNodes);
    $("#branch-total").textContent = String(allBranches);
    $("#connector-total").textContent = String(allConnectors);
    $("#export-annotation").disabled = !pages.length || pages.some((item) => !item.image) || !$("#document-id").value.trim() || !$("#annotator-id").value.trim();
    $("#page-count").textContent = String(pages.length);
    if (page) renderPageList();
  }

  function displayScale(page) {
    if (!page?.image) return 1;
    const wrap = $("#canvas-wrap");
    const availableWidth = Math.max(180, wrap.clientWidth - 34);
    const availableHeight = Math.max(180, wrap.clientHeight - 34);
    return Math.min(availableWidth / page.width, availableHeight / page.height) * zoom;
  }

  function drawCanvas() {
    const page = currentPage();
    if (!page?.image) return;
    const scale = displayScale(page);
    const cssWidth = Math.max(1, Math.round(page.width * scale));
    const cssHeight = Math.max(1, Math.round(page.height * scale));
    const ratio = window.devicePixelRatio || 1;
    canvas.style.width = `${cssWidth}px`;
    canvas.style.height = `${cssHeight}px`;
    canvas.width = Math.round(cssWidth * ratio);
    canvas.height = Math.round(cssHeight * ratio);
    context.setTransform(ratio, 0, 0, ratio, 0, 0);
    context.fillStyle = "#fcfaf3";
    context.fillRect(0, 0, cssWidth, cssHeight);
    context.drawImage(page.image, 0, 0, cssWidth, cssHeight);
    const drawBox = (item, color, fill, active) => {
      const x = item.region.x * cssWidth, y = item.region.y * cssHeight;
      const w = item.region.width * cssWidth, h = item.region.height * cssHeight;
      context.save(); context.lineWidth = active ? 2.5 : 1.5; context.strokeStyle = color; context.fillStyle = fill;
      if (active) context.shadowColor = color, context.shadowBlur = 8;
      context.fillRect(x, y, w, h); context.strokeRect(x, y, w, h); context.restore();
    };
    for (const item of pageBranches(page)) drawBox(item, "#d19134", "#d191341a", selected?.kind === "branch" && selected.id === item.id);
    for (const item of pageNames(page)) drawBox(item, "#31a279", "#31a27920", selected?.kind === "node" && selected.id === item.id);
    for (const item of pageConnectors(page)) drawPath(item.path, cssWidth, cssHeight, "#6cb2cc", selected?.kind === "connector" && selected.id === item.id);
    if (connectorDraft.length) drawPath(connectorDraft, cssWidth, cssHeight, "#e3bf73", true);
    if (drag) {
      const x = Math.min(drag.startX, drag.currentX), y = Math.min(drag.startY, drag.currentY);
      const width = Math.abs(drag.currentX - drag.startX), height = Math.abs(drag.currentY - drag.startY);
      context.save(); context.setLineDash([6, 4]); context.lineWidth = 2; context.strokeStyle = tool === "branch" ? "#e4b657" : "#58c79a"; context.strokeRect(x * cssWidth, y * cssHeight, width * cssWidth, height * cssHeight); context.restore();
    }
  }

  function drawPath(points, width, height, color, preview = false) {
    if (!points?.length) return;
    context.save(); context.strokeStyle = color; context.lineWidth = 2; context.lineCap = "round"; context.lineJoin = "round";
    if (preview) context.setLineDash([5, 4]);
    context.beginPath(); context.moveTo(points[0][0] * width, points[0][1] * height);
    points.slice(1).forEach((point) => context.lineTo(point[0] * width, point[1] * height)); context.stroke();
    points.forEach((point) => { context.beginPath(); context.arc(point[0] * width, point[1] * height, 3, 0, Math.PI * 2); context.fillStyle = color; context.fill(); });
    context.restore();
  }

  function pointFromEvent(event) {
    const page = currentPage();
    const rect = canvas.getBoundingClientRect();
    const x = (event.clientX - rect.left) / rect.width;
    const y = (event.clientY - rect.top) / rect.height;
    return [Math.max(0, Math.min(1, x)), Math.max(0, Math.min(1, y))];
  }

  function hitTest(point) {
    const page = currentPage();
    if (!page) return null;
    const inBox = (item) => point[0] >= item.region.x && point[0] <= item.region.x + item.region.width && point[1] >= item.region.y && point[1] <= item.region.y + item.region.height;
    for (const item of [...pageNames(page)].reverse()) if (inBox(item)) return { kind: "node", id: item.id };
    for (const item of [...pageBranches(page)].reverse()) if (inBox(item)) return { kind: "branch", id: item.id };
    for (const item of [...pageConnectors(page)].reverse()) {
      if (item.path.some((p) => Math.hypot((p[0] - point[0]) * 120, (p[1] - point[1]) * 120) < 2.5)) return { kind: "connector", id: item.id };
    }
    return null;
  }

  function setTool(next) {
    if (tool === "connector" && connectorDraft.length) finishConnector();
    tool = next;
    document.querySelectorAll(".tool[data-tool]").forEach((button) => button.classList.toggle("active", button.dataset.tool === tool));
    $("#finish-connector").hidden = tool !== "connector";
    canvas.style.cursor = tool === "select" ? "default" : tool === "connector" ? "cell" : "crosshair";
    drawCanvas();
  }

  function addBox(kind, bounds) {
    const page = currentPage();
    if (!page) return;
    const coords = box(Math.min(bounds[0], bounds[2]), Math.min(bounds[1], bounds[3]), Math.abs(bounds[2] - bounds[0]), Math.abs(bounds[3] - bounds[1]));
    if (coords.width < .001 || coords.height < .001) return;
    if (kind === "name") {
      const item = { id: id("name"), page_id: page.pageId, name_ar: "", unreadable_spans_ar: "", name_status: "unreadable", status: "unreadable", region: coords, annotation_status: "needs_review" };
      page.nodes.push(item); selected = { kind: "node", id: item.id };
    } else {
      const item = { id: id("branch"), page_id: page.pageId, label: "", region: coords, annotation_status: "confirmed" };
      page.branches.push(item); selected = { kind: "branch", id: item.id };
    }
    renderSelection(); renderOccurrenceList(); renderRelationships(); updateTotals(); drawCanvas();
  }

  function finishConnector() {
    const page = currentPage();
    if (page && connectorDraft.length >= 2) {
      const item = { id: id("connector"), page_id: page.pageId, path: connectorDraft.map((point) => [...point]), kind: "line", support_notes: "", annotation_status: "needs_review" };
      page.connectors.push(item); selected = { kind: "connector", id: item.id };
    }
    connectorDraft = [];
    $("#finish-connector").hidden = tool !== "connector";
    renderSelection(); renderOccurrenceList(); updateTotals(); drawCanvas();
  }

  canvas.addEventListener("pointerdown", (event) => {
    const page = currentPage(); if (!page?.image) return;
    const point = pointFromEvent(event);
    if (tool === "select") { selected = hitTest(point); renderSelection(); renderOccurrenceList(); drawCanvas(); return; }
    if (tool === "connector") { connectorDraft.push(point); drawCanvas(); return; }
    drag = { startX: point[0], startY: point[1], currentX: point[0], currentY: point[1] };
    canvas.setPointerCapture(event.pointerId);
  });
  canvas.addEventListener("pointermove", (event) => { if (!drag) return; const point = pointFromEvent(event); drag.currentX = point[0]; drag.currentY = point[1]; drawCanvas(); });
  canvas.addEventListener("pointerup", () => {
    if (!drag) return;
    const bounds = [drag.startX, drag.startY, drag.currentX, drag.currentY];
    const selectedTool = tool; drag = null;
    addBox(selectedTool, bounds);
  });

  function selectedItem() {
    const page = currentPage(); if (!page || !selected) return null;
    const source = selected.kind === "node" ? pageNames(page) : selected.kind === "branch" ? pageBranches(page) : pageConnectors(page);
    return source.find((item) => item.id === selected.id) || null;
  }

  function renderSelection() {
    const host = $("#selection-panel");
    const item = selectedItem();
    if (!item) { host.innerHTML = '<div class="empty-selection"><span>Choose an occurrence</span><p>Select a mark on the page or draw a new region.</p></div>'; return; }
    if (selected.kind === "node") {
      host.innerHTML = `<div class="detail-form"><div class="detail-form-head"><span>Name occurrence · ${escapeHtml(item.id.slice(0, 13))}</span><button class="delete-item" data-delete="1">Remove</button></div><label>Exact visible Arabic label<input id="item-text" dir="rtl" lang="ar" value="${escapeHtml(item.name_ar)}" placeholder="اكتب ما يظهر في المصدر فقط"></label><label>Reading status<select id="item-status"><option value="readable" ${item.name_status === "readable" ? "selected" : ""}>Readable</option><option value="partial" ${item.name_status === "partial" ? "selected" : ""}>Partly readable</option><option value="unreadable" ${item.name_status === "unreadable" ? "selected" : ""}>Unreadable</option></select></label><label>Unreadable span or uncertain glyph<textarea id="item-unreadable" dir="rtl" lang="ar" placeholder="Describe the missing or ambiguous visible part">${escapeHtml(item.unreadable_spans_ar || "")}</textarea></label><label>Review note<textarea id="item-note" dir="rtl" lang="ar" placeholder="Clipped edge, crossing line, or other issue">${escapeHtml(item.note_ar || "")}</textarea></label></div>`;
      $("#item-text").addEventListener("input", (event) => { item.name_ar = event.target.value; if (item.name_ar.trim() && item.name_status === "unreadable") item.name_status = "readable"; item.status = item.name_status; $("#item-status").value = item.name_status; renderOccurrenceList(); renderRelationships(); updateTotals(); });
      $("#item-status").addEventListener("change", (event) => { item.name_status = event.target.value; item.status = event.target.value; if (event.target.value === "unreadable") item.name_ar = ""; renderSelection(); renderOccurrenceList(); renderRelationships(); });
      $("#item-unreadable").addEventListener("input", (event) => { item.unreadable_spans_ar = event.target.value; });
      $("#item-note").addEventListener("input", (event) => { item.note_ar = event.target.value; });
    } else if (selected.kind === "branch") {
      host.innerHTML = `<div class="detail-form"><div class="detail-form-head"><span>Branch region · ${escapeHtml(item.id.slice(0, 13))}</span><button class="delete-item" data-delete="1">Remove</button></div><label>Visible branch heading<input id="item-label" dir="rtl" lang="ar" value="${escapeHtml(item.label || "")}" placeholder="Optional visible label"></label><p class="hint">A region records where a branch appears. It does not assert a family relationship.</p></div>`;
      $("#item-label").addEventListener("input", (event) => { item.label = event.target.value; renderOccurrenceList(); });
    } else {
      host.innerHTML = `<div class="detail-form"><div class="detail-form-head"><span>Connector trace · ${escapeHtml(item.id.slice(0, 13))}</span><button class="delete-item" data-delete="1">Remove</button></div><label>Path type<select id="item-kind"><option value="line" ${item.kind === "line" ? "selected" : ""}>Line</option><option value="curve" ${item.kind === "curve" ? "selected" : ""}>Curve</option><option value="junction" ${item.kind === "junction" ? "selected" : ""}>Junction</option><option value="crossing" ${item.kind === "crossing" ? "selected" : ""}>Crossing</option><option value="uncertain" ${item.kind === "uncertain" ? "selected" : ""}>Uncertain</option></select></label><label>Visible support or uncertainty<textarea id="item-support" dir="rtl" lang="ar" placeholder="Record visible line evidence; do not infer kinship">${escapeHtml(item.support_notes || "")}</textarea></label><p class="hint">${item.path.length} source points · endpoint names are recorded separately as relationship annotations.</p></div>`;
      $("#item-kind").addEventListener("change", (event) => { item.kind = event.target.value; renderOccurrenceList(); });
      $("#item-support").addEventListener("input", (event) => { item.support_notes = event.target.value; });
    }
    host.querySelector("[data-delete]").addEventListener("click", () => removeSelected());
  }

  function removeSelected() {
    const page = currentPage(); if (!page || !selected) return;
    if (selected.kind === "node") {
      page.nodes = page.nodes.filter((item) => item.id !== selected.id);
      page.edges = (page.edges || []).filter((edge) => edge.subject_id !== selected.id && edge.object_id !== selected.id);
    } else if (selected.kind === "branch") page.branches = page.branches.filter((item) => item.id !== selected.id);
    else page.connectors = page.connectors.filter((item) => item.id !== selected.id);
    selected = null; renderSelection(); renderOccurrenceList(); renderRelationships(); updateTotals(); drawCanvas();
  }

  function renderOccurrenceList() {
    const host = $("#occurrence-list"); const page = currentPage();
    if (!page) { host.innerHTML = ""; return; }
    const entries = [
      ...pageNames(page).map((item) => ({ kind: "node", item, label: item.name_ar || (item.name_status === "unreadable" ? "Unreadable label" : "Untranscribed label"), status: item.name_status })),
      ...pageBranches(page).map((item) => ({ kind: "branch", item, label: item.label || "Unlabeled branch region", status: "region" })),
      ...pageConnectors(page).map((item) => ({ kind: "connector", item, label: `${item.kind} path · ${item.path.length} points`, status: "trace" })),
    ];
    host.innerHTML = entries.length ? entries.map(({ kind, item, label, status }) => `<button class="occurrence-row ${kind} ${selected?.kind === kind && selected.id === item.id ? "active" : ""}" data-kind="${kind}" data-id="${escapeHtml(item.id)}"><span class="swatch"></span><span>${escapeHtml(label)}</span><small>${escapeHtml(status)}</small></button>`).join("") : '<p class="hint">No regions marked yet.</p>';
    host.querySelectorAll("[data-kind]").forEach((button) => button.addEventListener("click", () => { selected = { kind: button.dataset.kind, id: button.dataset.id }; renderSelection(); renderOccurrenceList(); drawCanvas(); }));
  }

  function renderRelationships() {
    const page = currentPage(); const names = page ? pageNames(page) : [];
    const options = ['<option value="">Choose a reviewed name</option>', ...names.filter((item) => item.name_status !== "unreadable" && item.name_ar.trim()).map((item) => `<option value="${escapeHtml(item.id)}">${escapeHtml(item.name_ar)}</option>`)];
    const subject = $("#edge-subject"), object = $("#edge-object");
    const previousSubject = subject.value, previousObject = object.value;
    subject.innerHTML = options.join(""); object.innerHTML = options.join("");
    if (names.some((item) => item.id === previousSubject)) subject.value = previousSubject;
    if (names.some((item) => item.id === previousObject)) object.value = previousObject;
    const list = $("#edge-list");
    list.innerHTML = page?.edges?.length ? page.edges.map((edge) => {
      const from = names.find((name) => name.id === edge.subject_id)?.name_ar || "?";
      const to = names.find((name) => name.id === edge.object_id)?.name_ar || "?";
      return `<div class="edge-row"><span dir="rtl">${escapeHtml(from)} · ${escapeHtml(edge.predicate)} · ${escapeHtml(to)}</span><button data-edge-delete="${escapeHtml(edge.id)}" aria-label="Remove relation">×</button></div>`;
    }).join("") : "";
    list.querySelectorAll("[data-edge-delete]").forEach((button) => button.addEventListener("click", () => { page.edges = page.edges.filter((edge) => edge.id !== button.dataset.edgeDelete); renderRelationships(); }));
  }

  $("#add-edge").addEventListener("click", () => {
    const page = currentPage(); const subject = $("#edge-subject").value, object = $("#edge-object").value;
    const predicate = $("#edge-predicate").value; const evidence = $("#edge-evidence").value.trim();
    if (!subject || !object || subject === object || !evidence) { showMessage("More evidence is needed", "Choose two distinct readable occurrences and record the visible line or legend that supports this relationship."); return; }
    const nodes = pageNames(page); const edge = { id: id("edge"), subject_id: subject, predicate, object_id: object, evidence_ar: evidence };
    if (!nodes.some((item) => item.id === subject) || !nodes.some((item) => item.id === object)) return;
    page.edges ||= []; page.edges.push(edge); $("#edge-evidence").value = ""; renderRelationships();
  });

  $("#page-files").addEventListener("change", async (event) => {
    const files = [...event.target.files || []]; event.target.value = "";
    for (const file of files) {
      if (!file.type.startsWith("image/")) continue;
      if (file.size > maxImageBytes) { showMessage("Page image is too large", "Load page images up to 80 MB. Render at the original resolution and avoid unnecessary upscaling."); continue; }
      let imageSha256;
      try { imageSha256 = await sha256Hex(file); } catch (error) { showMessage("Source image cannot be verified", error.message); continue; }
      const dataURL = await new Promise((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(reader.result); reader.onerror = reject; reader.readAsDataURL(file); });
      const image = new Image(); image.src = dataURL;
      try { await image.decode(); } catch { showMessage("Image could not be opened", `${file.name} is not a readable image.`); continue; }
      if (image.naturalWidth * image.naturalHeight > maxImagePixels) { showMessage("Page image dimensions are too large", "This browser labeler accepts up to 180 megapixels per page. Render an archival working copy at source quality and record its dimensions; do not enlarge it."); continue; }
      let page = pages.find((item) => !item.image);
      if (!page) {
        const pageNumber = pages.length + 1;
        page = { pageId: `p${pageNumber}`, pageNumber, nodes: [], branches: [], connectors: [], edges: [], coverageConfirmed: false };
        pages.push(page);
      }
      if (page.width && page.height && (image.naturalWidth !== page.width || image.naturalHeight !== page.height)) { showMessage("Source dimensions do not match", `The page image for ${page.pageId} must match the annotation dimensions (${page.width} × ${page.height}).`); continue; }
      if (page.imageSha256 && page.imageSha256 !== imageSha256) { showMessage("Source image does not match", `The page image for ${page.pageId} has different content from the image used for this annotation.`); continue; }
      page.image = image; page.imageURL = dataURL; page.fileName = file.name; page.width = image.naturalWidth; page.height = image.naturalHeight; page.imageSha256 = imageSha256;
      if (!activePageId) activePageId = page.pageId;
    }
    updatePageMeta();
  });

  $("#coverage-confirmed").addEventListener("change", (event) => { const page = currentPage(); if (page) page.coverageConfirmed = event.target.checked; renderPageList(); });
  document.querySelectorAll(".tool[data-tool]").forEach((button) => button.addEventListener("click", () => setTool(button.dataset.tool)));
  $("#finish-connector").addEventListener("click", finishConnector);
  $("#zoom-in").addEventListener("click", () => { zoom = Math.min(zoom * 1.25, 8); $("#zoom-value").textContent = `${Math.round(zoom * 100)}%`; drawCanvas(); });
  $("#zoom-out").addEventListener("click", () => { zoom = Math.max(zoom / 1.25, .25); $("#zoom-value").textContent = `${Math.round(zoom * 100)}%`; drawCanvas(); });
  $("#zoom-fit").addEventListener("click", () => { zoom = 1; $("#zoom-value").textContent = "Fit"; drawCanvas(); });
  window.addEventListener("resize", drawCanvas);
  $("#document-id").addEventListener("input", updateTotals);
  $("#annotator-id").addEventListener("input", updateTotals);
  $("#export-annotation").addEventListener("click", () => {
    try {
      const data = validateAnnotation(independentExport());
      download(`${safeFilename(data.document_id)}.${safeFilename(data.annotator_id)}.independent.json`, JSON.stringify(data, null, 2), "application/json");
    } catch (error) { showMessage("Annotation needs attention", error.message || "Complete the required source labels before exporting."); }
  });

  $("#open-annotation").addEventListener("change", async (event) => {
    const file = event.target.files?.[0]; event.target.value = ""; if (!file) return;
    try {
      const data = validateAnnotation(JSON.parse(await file.text()));
      pages.splice(0, pages.length);
      data.pages.forEach((source, index) => {
        const pageId = source.page_id || `p${index + 1}`;
        pages.push({ pageId, pageNumber: source.page_number || index + 1, fileName: "", width: source.width, height: source.height, imageSha256: source.image_sha256, textAR: source.text_ar || "", textStatus: source.text_status || "unreadable", image: null, nodes: (data.nodes || []).filter((item) => item.page_id === pageId).map((item) => ({ ...item })), branches: (data.branches || []).filter((item) => item.page_id === pageId).map((item) => ({ ...item })), connectors: (data.connectors || []).filter((item) => item.page_id === pageId).map((item) => ({ ...item })), edges: (data.edges || []).filter((item) => item.page_id === pageId).map((item) => ({ ...item })), coverageConfirmed: source.coverage_confirmed === true });
      });
      if (!pages.length) throw new Error("The annotation has no page records.");
      $("#document-id").value = data.document_id; $("#annotator-id").value = data.annotator_id || ""; $("#source-class").value = data.source_class || "chart"; $("#split").value = data.split || "development"; activePageId = pages[0].pageId;
      updatePageMeta(); showMessage("Annotation opened", "The page images are not embedded. Add the same page images in the same order to continue reviewing this annotation.");
    } catch (error) { showMessage("Could not open annotation", error.message || "The JSON file is invalid."); }
  });

  function readAnnotation(fileInput) {
    return new Promise((resolve, reject) => {
      const file = fileInput.files?.[0]; if (!file) { resolve(null); return; }
      const reader = new FileReader(); reader.onload = () => { try { resolve(validateAnnotation(JSON.parse(reader.result))); } catch (error) { reject(error instanceof Error ? error : new Error("Annotation JSON is invalid.")); } }; reader.onerror = () => reject(new Error("Annotation file could not be read.")); reader.readAsText(file);
    });
  }

  function validateAnnotation(data) {
    const fail = (message) => { throw new Error(`Invalid independent annotation: ${message}`); };
    const validText = (value) => typeof value === "string";
    const validRegion = (region) => {
      if (!region || typeof region !== "object") return false;
      const { x, y, width, height } = region;
      return [x, y, width, height].every((value) => typeof value === "number" && Number.isFinite(value)) && x >= 0 && y >= 0 && width > 0 && height > 0 && x + width <= 1.000001 && y + height <= 1.000001;
    };
    if (!data || typeof data !== "object" || Array.isArray(data) || data.schema_version !== 2 || data.annotation_mode !== "blind_independent") fail("unsupported schema or annotation mode.");
    if (!validText(data.document_id) || !data.document_id.trim() || !validText(data.annotator_id) || !data.annotator_id.trim()) fail("document and annotator IDs are required.");
    if (data.source_class !== "chart" || !new Set(["development", "held_out", "unspecified"]).has(data.split)) fail("this annotation tool accepts family charts only; source class or split is invalid.");
    if (!Array.isArray(data.pages) || !data.pages.length) fail("at least one page is required.");
    for (const key of ["nodes", "branches", "connectors", "edges"]) if (!Array.isArray(data[key])) fail(`${key} must be an array.`);
    const pageIds = new Set();
    for (const page of data.pages) {
      if (!page || typeof page.page_id !== "string" || !page.page_id.trim() || pageIds.has(page.page_id)) fail("page IDs must be present and unique.");
      pageIds.add(page.page_id);
      if (!Number.isInteger(page.page_number) || page.page_number <= 0) fail(`page ${page.page_id} needs a valid page number.`);
      if (page.coverage_confirmed !== true && page.coverage_confirmed !== false) fail(`page ${page.page_id} needs an explicit human coverage decision.`);
      if (typeof page.image_sha256 !== "string" || !/^[a-f0-9]{64}$/.test(page.image_sha256)) fail(`page ${page.page_id} needs the SHA-256 fingerprint of its source image.`);
      if (!["readable", "partial", "unreadable"].includes(page.text_status) || !validText(page.text_ar || "")) fail(`page ${page.page_id} has invalid transcription metadata.`);
      for (const key of ["width", "height"]) if (!Number.isFinite(page[key]) || page[key] <= 0) fail(`page ${page.page_id} needs valid source dimensions.`);
    }
    const nodesById = new Map();
    const ids = new Set();
    const checkId = (item, kind) => {
      if (!item || typeof item.id !== "string" || !item.id.trim() || ids.has(item.id)) fail(`${kind} IDs must be present and unique.`);
      ids.add(item.id);
    };
    for (const node of data.nodes) {
      checkId(node, "name occurrence");
      if (!pageIds.has(node.page_id) || !validRegion(node.region)) fail(`name ${node.id} has invalid page or coordinates.`);
      const status = node.name_status || node.status;
      if (!new Set(["readable", "partial", "unreadable"]).has(status) || !validText(node.name_ar || "") || !validText(node.unreadable_spans_ar || "")) fail(`name ${node.id} has invalid reading data.`);
      if (status !== "unreadable" && !node.name_ar.trim()) fail(`name ${node.id} needs visible text or an unreadable status.`);
      if (status === "unreadable" && node.name_ar.trim()) fail(`unreadable name ${node.id} cannot contain guessed text.`);
      if (status === "partial" && !node.unreadable_spans_ar.trim()) fail(`partly readable name ${node.id} needs its unreadable span recorded.`);
      nodesById.set(node.id, node);
    }
    for (const branch of data.branches) {
      checkId(branch, "branch");
      if (!pageIds.has(branch.page_id) || !validRegion(branch.region) || !validText(branch.label || "")) fail(`branch ${branch.id} has invalid page, coordinates, or label.`);
    }
    for (const connector of data.connectors) {
      checkId(connector, "connector");
      if (!pageIds.has(connector.page_id) || !Array.isArray(connector.path) || connector.path.length < 2 || connector.path.some((point) => !Array.isArray(point) || point.length !== 2 || point.some((value) => typeof value !== "number" || !Number.isFinite(value) || value < 0 || value > 1))) fail(`connector ${connector.id} has invalid page or path.`);
    }
    for (const edge of data.edges) {
      checkId(edge, "relationship");
      const subject = nodesById.get(edge.subject_id), object = nodesById.get(edge.object_id);
      if (!subject || !object || subject.page_id !== edge.page_id || object.page_id !== edge.page_id || subject.id === object.id || !validText(edge.predicate) || !edge.predicate.trim() || !validText(edge.evidence_ar) || !edge.evidence_ar.trim()) fail(`relationship ${edge.id} has invalid endpoints or lacks visible evidence.`);
    }
    return data;
  }

  function regionIoU(a, b) {
    const x = a?.region, y = b?.region; if (!x || !y) return 0;
    const intersection = Math.max(0, Math.min(x.x + x.width, y.x + y.width) - Math.max(x.x, y.x)) * Math.max(0, Math.min(x.y + x.height, y.y + y.height) - Math.max(x.y, y.y));
    const union = x.width * x.height + y.width * y.height - intersection;
    return union > 0 ? intersection / union : 0;
  }

  function pairByGeometry(left, right, kind) {
    const candidates = [];
    left.forEach((a, ai) => right.forEach((b, bi) => {
      if (a.page_id !== b.page_id) return;
      const score = kind === "connector" ? pathSimilarity(a.path, b.path) : regionIoU(a, b);
      if (score >= (kind === "connector" ? .8 : .5)) candidates.push({ ai, bi, score });
    }));
    candidates.sort((a, b) => b.score - a.score);
    const usedLeft = new Set(), usedRight = new Set(), pairs = [];
    for (const candidate of candidates) { if (usedLeft.has(candidate.ai) || usedRight.has(candidate.bi)) continue; usedLeft.add(candidate.ai); usedRight.add(candidate.bi); pairs.push({ a: left[candidate.ai], b: right[candidate.bi], score: candidate.score, kind }); }
    left.forEach((a, index) => { if (!usedLeft.has(index)) pairs.push({ a, b: null, score: 0, kind }); });
    right.forEach((b, index) => { if (!usedRight.has(index)) pairs.push({ a: null, b, score: 0, kind }); });
    return pairs;
  }

  function pathSimilarity(a, b) {
    if (!Array.isArray(a) || !Array.isArray(b) || a.length < 2 || b.length < 2) return 0;
    const distance = (x, y) => Math.hypot(x[0] - y[0], x[1] - y[1]);
    const direct = Math.max(distance(a[0], b[0]), distance(a[a.length - 1], b[b.length - 1]));
    const reverse = Math.max(distance(a[0], b[b.length - 1]), distance(a[a.length - 1], b[0]));
    return Math.max(0, 1 - Math.min(direct, reverse) / .025);
  }

  function compareField(a, b, entityType) {
    if (!a || !b) return false;
    if (entityType === "node") return exactText(a.name_ar) === exactText(b.name_ar) && exactText(a.unreadable_spans_ar) === exactText(b.unreadable_spans_ar) && (a.name_status || a.status || "readable") === (b.name_status || b.status || "readable") && regionIoU(a, b) >= .8;
    if (entityType === "branch") return exactText(a.label) === exactText(b.label) && regionIoU(a, b) >= .8;
    if (entityType === "connector") return a.kind === b.kind && pathSimilarity(a.path, b.path) >= .8;
    return false;
  }

  function conflictRecord(pair, index) {
    const a = pair.a ? { ...pair.a } : null;
    const b = pair.b ? { ...pair.b } : null;
    const agreed = a && b && compareField(a, b, pair.kind);
    return { key: `${pair.kind}-${index}`, kind: pair.kind, a, b, score: pair.score, decision: agreed ? "consensus" : "pending" };
  }

  function createAdjudication(left, right) {
    if (!left || !right) return null;
    if (left.document_id !== right.document_id || left.source_class !== right.source_class || left.split !== right.split) throw new Error("The two files must describe the same document, source class, and split.");
    if (!left.annotator_id || !right.annotator_id || left.annotator_id === right.annotator_id) throw new Error("The two independent files must have distinct annotator IDs.");
    const nodes = pairByGeometry(left.nodes || [], right.nodes || [], "node");
    const branches = pairByGeometry(left.branches || [], right.branches || [], "branch");
    const connectors = pairByGeometry(left.connectors || [], right.connectors || [], "connector");
    const nodeGroups = new Map();
    nodes.forEach((pair, index) => { const group = `name-group-${index}`; if (pair.a) nodeGroups.set(`a:${pair.a.id}`, group); if (pair.b) nodeGroups.set(`b:${pair.b.id}`, group); pair.group = group; });
    const edgeSignature = (edge, side) => [nodeGroups.get(`${side}:${edge.subject_id}`) || `missing:${edge.subject_id}`, edge.predicate, nodeGroups.get(`${side}:${edge.object_id}`) || `missing:${edge.object_id}`].join("|");
    const aEdges = left.edges || [], bEdges = right.edges || [];
    const bEdgeIndexes = new Map(); bEdges.forEach((edge, index) => { const signature = edgeSignature(edge, "b"); if (!bEdgeIndexes.has(signature)) bEdgeIndexes.set(signature, []); bEdgeIndexes.get(signature).push(index); });
    const pairedEdges = [], usedB = new Set();
    aEdges.forEach((edge) => { const signature = edgeSignature(edge, "a"); const match = (bEdgeIndexes.get(signature) || []).find((index) => !usedB.has(index)); if (match !== undefined) { usedB.add(match); pairedEdges.push({ a: edge, b: bEdges[match], kind: "edge" }); } else pairedEdges.push({ a: edge, b: null, kind: "edge" }); });
    bEdges.forEach((edge, index) => { if (!usedB.has(index)) pairedEdges.push({ a: null, b: edge, kind: "edge" }); });
    const rows = [...nodes.map((pair, index) => conflictRecord(pair, index)), ...branches.map((pair, index) => conflictRecord(pair, index)), ...connectors.map((pair, index) => conflictRecord(pair, index))];
    pairedEdges.forEach((pair, index) => rows.push({ key: `edge-${index}`, kind: "edge", a: pair.a, b: pair.b, score: pair.a && pair.b ? 1 : 0, decision: "pending" }));
    const leftPages = new Map((left.pages || []).map((page) => [page.page_id, page]));
    const rightPages = new Map((right.pages || []).map((page) => [page.page_id, page]));
    const pageIds = [...new Set([...leftPages.keys(), ...rightPages.keys()])].sort();
    const coverage = pageIds.map((pageId) => {
      const aPage = leftPages.get(pageId), bPage = rightPages.get(pageId);
      const sameSourcePage = Boolean(aPage && bPage && aPage.page_number === bPage.page_number && aPage.width === bPage.width && aPage.height === bPage.height && aPage.image_sha256 === bPage.image_sha256);
      return { pageId, a: Boolean(aPage && aPage.coverage_confirmed === true), b: Boolean(bPage && bPage.coverage_confirmed === true), annotationsMatch: sameSourcePage, shared: sameSourcePage };
    });
    return { documentId: left.document_id, sourceClass: left.source_class, split: left.split || "development", annotatorA: left.annotator_id, annotatorB: right.annotator_id, left, right, nodes, branches, connectors, nodeGroups, rows, coverage, adjudicatorId: $("#adjudicator-id").value.trim() };
  }

  async function refreshAdjudication() {
    try {
      annotationA = await readAnnotation($("#review-a"));
      annotationB = await readAnnotation($("#review-b"));
      if (!annotationA || !annotationB) { $("#adjudication-status").textContent = "Two independent exports are required."; renderAdjudication(); return; }
      adjudication = createAdjudication(annotationA, annotationB);
      adjudicationRows = adjudication.rows;
      alignAdjudicationImages();
      renderAdjudicationImageOptions();
      $("#adjudication-status").textContent = `${adjudication.documentId} · ${adjudication.annotatorA} + ${adjudication.annotatorB}`;
      renderAdjudication();
    } catch (error) { adjudication = null; adjudicationRows = []; $("#adjudication-status").textContent = error.message; renderAdjudication(); }
  }

  function renderAdjudication() {
    const host = $("#conflict-list"), summary = $("#adjudication-summary");
    if (!adjudication) { host.innerHTML = '<div class="empty-pages">Load two independent annotations for the same document to compare occurrences.</div>'; summary.innerHTML = ""; $("#export-gold").disabled = true; return; }
    adjudication.coverage = adjudication.coverage.map((page) => {
      const aPage = adjudication.left.pages.find((item) => item.page_id === page.pageId);
      const bPage = adjudication.right.pages.find((item) => item.page_id === page.pageId);
      const image = adjudicationImages.find((item) => item.pageId === page.pageId);
      const matchingDimensions = Boolean(image && image.width === aPage?.width && image.height === aPage?.height && image.width === bPage?.width && image.height === bPage?.height);
      const matchingFingerprint = Boolean(image && image.imageSha256 === aPage?.image_sha256 && image.imageSha256 === bPage?.image_sha256);
      const sourceStatus = !image ? "Source image not loaded" : !page.annotationsMatch ? "Reader source records differ" : !matchingDimensions ? "Image dimensions do not match" : !matchingFingerprint ? "Image fingerprint does not match" : "Exact source image verified";
      return { ...page, shared: Boolean(page.annotationsMatch && matchingDimensions && matchingFingerprint), sourceStatus };
    });
    const pending = adjudicationRows.filter((row) => row.decision === "pending").length;
    const acceptedNodeIds = new Set();
    for (const row of adjudicationRows.filter((item) => item.kind === "node" && item.decision !== "absent")) {
      if (row.a) acceptedNodeIds.add(`a:${row.a.id}`);
      if (row.b) acceptedNodeIds.add(`b:${row.b.id}`);
    }
    const ungroundedRelationships = adjudicationRows.filter((row) => row.kind === "edge" && row.decision !== "pending" && row.decision !== "absent").filter((row) => {
      const side = row.decision === "b" ? "b" : row.a ? "a" : "b";
      const edge = side === "b" ? row.b : row.a;
      return !edge || !acceptedNodeIds.has(`${side}:${edge.subject_id}`) || !acceptedNodeIds.has(`${side}:${edge.object_id}`);
    }).length;
    const unreviewedCoverage = adjudication.coverage.filter((page) => !page.a || !page.b || !page.shared).length;
    const imagePages = adjudication.left.pages || [];
    summary.innerHTML = `<span class="summary-chip"><b>${adjudicationRows.length}</b> comparison units</span><span class="summary-chip"><b>${pending}</b> unresolved differences</span><span class="summary-chip"><b>${ungroundedRelationships}</b> confirmed relations without accepted endpoints</span><span class="summary-chip"><b>${unreviewedCoverage}</b> pages without two full reviews and matching source</span><span class="summary-chip"><b>${imagePages.length}</b> pages in common</span>`;
    const coverageRows = adjudication.coverage.map((page) => `<div class="conflict-row ${page.a && page.b && page.shared ? "resolved" : "pending"}"><div class="conflict-title">Full-page review · ${escapeHtml(page.pageId)} · ${escapeHtml(page.sourceStatus)}</div><div class="conflict-reader"><small>READER A</small><span>${page.a ? "Confirmed full-page and residual review" : "Not confirmed or page missing"}</span></div><div class="conflict-reader"><small>READER B</small><span>${page.b ? "Confirmed full-page and residual review" : "Not confirmed or page missing"}</span></div><div class="conflict-actions">${page.a && page.b && page.shared ? "<button class=\"chosen\" disabled>Both reviewed · source matched</button>" : "<span class=\"unresolved\">Needs reviews and matching source image</span>"}</div></div>`).join("");
    const entityRows = adjudicationRows.map((row) => {
      const aValue = describe(row.a, row.kind), bValue = describe(row.b, row.kind);
      const statusClass = row.decision === "pending" ? "pending" : "resolved";
      let controls;
      if (row.kind === "edge") {
        controls = row.decision === "confirmed"
          ? `<button class="chosen" disabled>Adjudicator confirmed</button><button data-resolve="${escapeHtml(row.key)}" data-choice="pending">Reopen decision</button>`
          : `<button data-resolve="${escapeHtml(row.key)}" data-choice="confirmed" ${row.a && row.b ? "" : "disabled"}>Confirm relationship</button><button data-resolve="${escapeHtml(row.key)}" data-choice="a" class="${row.decision === "a" ? "chosen" : ""}" ${row.a ? "" : "disabled"}>Use A</button><button data-resolve="${escapeHtml(row.key)}" data-choice="b" class="${row.decision === "b" ? "chosen" : ""}" ${row.b ? "" : "disabled"}>Use B</button><button data-resolve="${escapeHtml(row.key)}" data-choice="absent" class="${row.decision === "absent" ? "chosen" : ""}">Mark absent</button>`;
      } else if (row.decision === "consensus") {
        controls = '<button class="chosen" disabled>Independent agreement</button>';
      } else {
        controls = `<button data-resolve="${escapeHtml(row.key)}" data-choice="a" class="${row.decision === "a" ? "chosen" : ""}" ${row.a ? "" : "disabled"}>Use A</button><button data-resolve="${escapeHtml(row.key)}" data-choice="b" class="${row.decision === "b" ? "chosen" : ""}" ${row.b ? "" : "disabled"}>Use B</button><button data-resolve="${escapeHtml(row.key)}" data-choice="absent" class="${row.decision === "absent" ? "chosen" : ""}">Mark absent</button>`;
      }
      const pairDescription = row.kind === "edge" ? row.a && row.b ? "same mapped endpoints and predicate" : "only one reader recorded this relation" : row.a && row.b ? `region overlap ${Math.round(row.score * 100)}%` : "only one reader found this item";
      const statusFor = (item) => !item ? "No matching annotation" : row.kind === "node" ? item.name_status || item.annotation_status || "" : row.kind === "edge" ? "Visible support recorded" : item.annotation_status || "Marked by reader";
      return `<div class="conflict-row ${statusClass}"><div class="conflict-title">${escapeHtml(row.kind)} · ${escapeHtml(row.a?.page_id || row.b?.page_id || "page missing")} · ${escapeHtml(pairDescription)}</div><div class="conflict-reader"><small>READER A · ${escapeHtml(adjudication.annotatorA)}</small><b>${escapeHtml(aValue)}</b><span>${escapeHtml(statusFor(row.a))}</span></div><div class="conflict-reader"><small>READER B · ${escapeHtml(adjudication.annotatorB)}</small><b>${escapeHtml(bValue)}</b><span>${escapeHtml(statusFor(row.b))}</span></div><div class="conflict-actions"><button data-inspect="${escapeHtml(row.key)}" class="inspect-source">View source regions</button>${controls}</div></div>`;
    }).join("");
    host.innerHTML = coverageRows + entityRows || '<div class="empty-pages">No labels to compare.</div>';
    host.querySelectorAll("[data-resolve]").forEach((button) => button.addEventListener("click", () => {
      const row = adjudicationRows.find((item) => item.key === button.dataset.resolve); if (!row) return;
      row.decision = button.dataset.choice; renderAdjudication();
    }));
    host.querySelectorAll("[data-inspect]").forEach((button) => button.addEventListener("click", () => {
      activeAdjudicationKey = button.dataset.inspect;
      const row = adjudicationRows.find((item) => item.key === activeAdjudicationKey);
      const pageId = row?.a?.page_id || row?.b?.page_id;
      const image = adjudicationImages.find((candidate) => candidate.pageId === pageId);
      if (image) $("#adjudication-page").value = image.id;
      updateAdjudicationImage();
    }));
    const ready = Boolean($("#adjudicator-id").value.trim()) && pending === 0 && unreviewedCoverage === 0 && ungroundedRelationships === 0;
    $("#export-gold").disabled = !ready;
    if (!ready && adjudicationRows.length === 0) $("#export-gold").disabled = true;
  }

  function describe(item, entityType) {
    if (!item) return "—";
    if (entityType === "node") return item.name_status === "unreadable" || item.status === "unreadable" ? "Unreadable label" : `${item.name_ar || "Empty transcription"}${item.unreadable_spans_ar ? ` · unclear: ${item.unreadable_spans_ar}` : ""}`;
    if (entityType === "branch") return item.label || "Unlabeled region";
    if (entityType === "connector") return `${item.kind || "connector"} · ${item.path?.length || 0} points`;
    return `${item.predicate || "relation"}: ${item.evidence_ar || "no visible support recorded"}`;
  }

  $("#review-a").addEventListener("change", refreshAdjudication);
  $("#review-b").addEventListener("change", refreshAdjudication);
  $("#adjudicator-id").addEventListener("input", renderAdjudication);
  function alignAdjudicationImages() {
    const sourcePages = adjudication?.left.pages || [];
    adjudicationImages.forEach((image, index) => { if (sourcePages[index]) image.pageId = sourcePages[index].page_id; });
  }
  function renderAdjudicationImageOptions() {
    const select = $("#adjudication-page");
    const selectedValue = select.value;
    select.innerHTML = adjudicationImages.length
      ? adjudicationImages.map((image) => `<option value="${escapeHtml(image.id)}">${escapeHtml(image.pageId)} · ${escapeHtml(image.fileName)}</option>`).join("")
      : '<option value="">No source page loaded</option>';
    if (adjudicationImages.some((image) => image.id === selectedValue)) select.value = selectedValue;
    updateAdjudicationImage();
  }
  function updateAdjudicationImage() {
    const selectedImage = adjudicationImages.find((image) => image.id === $("#adjudication-page").value);
    const preview = $("#adjudication-preview");
    $("#adjudication-frame").hidden = !selectedImage;
    $("#adjudication-image-empty").hidden = Boolean(selectedImage);
    if (selectedImage) { preview.src = selectedImage.dataURL; preview.alt = `${selectedImage.pageId} · ${selectedImage.fileName}`; }
    else preview.removeAttribute("src");
    drawAdjudicationOverlay(selectedImage);
  }
  function drawAdjudicationOverlay(selectedImage) {
    const overlay = $("#adjudication-overlay");
    overlay.replaceChildren();
    const row = adjudicationRows.find((item) => item.key === activeAdjudicationKey);
    if (!selectedImage || !row || (row.a?.page_id || row.b?.page_id) !== selectedImage.pageId) return;
    const svg = "http://www.w3.org/2000/svg";
    const addShape = (item, color) => {
      if (!item) return;
      if (Array.isArray(item.path)) {
        const path = document.createElementNS(svg, "polyline");
        path.setAttribute("points", item.path.map((point) => point.join(",")).join(" "));
        path.setAttribute("fill", "none"); path.setAttribute("stroke", color); path.setAttribute("stroke-width", "0.004");
        overlay.append(path); return;
      }
      if (!item.region) return;
      const rect = document.createElementNS(svg, "rect");
      for (const key of ["x", "y", "width", "height"]) rect.setAttribute(key, String(item.region[key]));
      rect.setAttribute("fill", "#00000008"); rect.setAttribute("stroke", color); rect.setAttribute("stroke-width", "0.004");
      overlay.append(rect);
    };
    if (row.kind === "edge") {
      for (const [edge, color, annotation] of [[row.a, "#1fbb83", adjudication.left], [row.b, "#e2ac50", adjudication.right]]) {
        if (!edge) continue;
        for (const endpointId of [edge.subject_id, edge.object_id]) addShape(annotation.nodes.find((node) => node.id === endpointId), color);
        annotation.connectors.filter((connector) => connector.page_id === edge.page_id).forEach((connector) => addShape(connector, color));
      }
      return;
    }
    addShape(row.a, "#1fbb83"); addShape(row.b, "#e2ac50");
  }
  $("#adjudication-page").addEventListener("change", updateAdjudicationImage);
  $("#adjudication-images").addEventListener("change", async (event) => {
    const files = [...event.target.files || []];
    if (!files.length) return;
    adjudicationImages = [];
    for (const file of files) {
      if (!file.type.startsWith("image/")) continue;
      if (file.size > maxImageBytes) { showMessage("Source image is too large", "Load rendered source pages up to 80 MB each."); continue; }
      let imageSha256;
      try { imageSha256 = await sha256Hex(file); } catch (error) { showMessage("Source image cannot be verified", error.message); continue; }
      const dataURL = await new Promise((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(reader.result); reader.onerror = reject; reader.readAsDataURL(file); });
      const preview = new Image(); preview.src = dataURL;
      try { await preview.decode(); } catch { showMessage("Image could not be opened", `${file.name} is not a readable image.`); continue; }
      if (preview.naturalWidth * preview.naturalHeight > maxImagePixels) { showMessage("Source image dimensions are too large", "This browser labeler accepts up to 180 megapixels per page."); continue; }
      const pageNumber = adjudicationImages.length + 1;
      adjudicationImages.push({ id: id("source-page"), pageId: `p${pageNumber}`, fileName: file.name, dataURL, width: preview.naturalWidth, height: preview.naturalHeight, imageSha256 });
    }
    event.target.value = "";
    alignAdjudicationImages();
    renderAdjudicationImageOptions();
    renderAdjudication();
  });
  document.querySelectorAll(".mode-tab").forEach((button) => button.addEventListener("click", () => {
    document.querySelectorAll(".mode-tab").forEach((tab) => tab.classList.toggle("active", tab === button));
    document.querySelectorAll(".pane").forEach((pane) => { const active = pane.id === button.dataset.pane; pane.hidden = !active; pane.classList.toggle("active", active); });
    if (button.dataset.pane === "adjudicate-pane" && adjudicationImages.length === 0) {
      adjudicationImages = pages.filter((page) => page.imageURL).map((page) => ({ id: id("source-page"), pageId: page.pageId, fileName: page.fileName || "page image", dataURL: page.imageURL, width: page.width, height: page.height, imageSha256: page.imageSha256 }));
      alignAdjudicationImages();
      renderAdjudicationImageOptions();
    }
  }));

  function adjudicatedGold() {
    const mappedNodes = new Map();
    const nodes = [];
    for (const row of adjudicationRows.filter((item) => item.kind === "node" && item.decision !== "absent")) {
      const chosen = row.decision === "b" ? row.b : row.a || row.b;
      if (!chosen) continue;
      const stableId = id("gold"); const goldNode = { id: stableId, page_id: chosen.page_id, name_ar: chosen.name_ar || "", name_status: chosen.name_status || chosen.status || "unreadable", unreadable_spans_ar: chosen.unreadable_spans_ar || "", region: { ...chosen.region } };
      nodes.push(goldNode);
      if (row.a) mappedNodes.set(`a:${row.a.id}`, stableId);
      if (row.b) mappedNodes.set(`b:${row.b.id}`, stableId);
    }
    const mappedBranches = adjudicationRows.filter((item) => item.kind === "branch" && item.decision !== "absent").map((row) => { const chosen = row.decision === "b" ? row.b : row.a || row.b; if (!chosen) return null; return { id: id("branch-gold"), page_id: chosen.page_id, region: { ...chosen.region }, label: chosen.label || "" }; }).filter(Boolean);
    const mappedConnectors = adjudicationRows.filter((item) => item.kind === "connector" && item.decision !== "absent").map((row) => { const chosen = row.decision === "b" ? row.b : row.a || row.b; if (!chosen) return null; return { id: id("connector-gold"), page_id: chosen.page_id, path: chosen.path, kind: chosen.kind || "uncertain" }; }).filter(Boolean);
    const edges = [];
    for (const row of adjudicationRows.filter((item) => item.kind === "edge" && item.decision !== "absent")) {
      const chosen = row.decision === "b" ? row.b : row.a || row.b; if (!chosen) continue;
      const side = row.decision === "b" ? "b" : row.a ? "a" : "b";
      const subject = mappedNodes.get(`${side}:${chosen.subject_id}`), object = mappedNodes.get(`${side}:${chosen.object_id}`);
      if (subject && object) edges.push({ subject_id: subject, object_id: object, predicate: chosen.predicate || "", evidence_ar: chosen.evidence_ar || "" });
    }
    const pagesOut = (adjudication.left.pages || []).map((page) => ({ page_id: page.page_id, page_number: page.page_number, width: page.width, height: page.height, image_sha256: page.image_sha256, text_ar: page.text_ar || "", text_status: page.text_status || "unreadable", coverage_confirmed: true, processed: true }));
    const decisionLedger = adjudicationRows.map((row) => ({ kind: row.kind, decision: row.decision, reader_a_item_id: row.a?.id || null, reader_b_item_id: row.b?.id || null }));
    return {
      schema_version: 2,
      annotation_mode: "adjudicated_gold",
      document_id: adjudication.documentId,
      source_class: adjudication.sourceClass,
      split: adjudication.split,
      pages: pagesOut,
      nodes,
      branches: mappedBranches,
      connectors: mappedConnectors,
      edges,
      adjudication: { status: "adjudicated", adjudicator_id: $("#adjudicator-id").value.trim(), independent_annotators: [adjudication.annotatorA, adjudication.annotatorB], decisions: decisionLedger, policy: "Machine output was not shown during blind annotation." },
    };
  }

  $("#export-gold").addEventListener("click", () => {
    if (!adjudication) return;
    const gold = adjudicatedGold();
    download(`${safeFilename(gold.document_id)}.gold.jsonl`, `${JSON.stringify(gold)}\n`, "application/x-ndjson");
  });

  updateTotals();
})();
