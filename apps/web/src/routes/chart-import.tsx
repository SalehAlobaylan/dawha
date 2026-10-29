import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { ArrowLeft, Check, FileText, GitBranch, LoaderCircle, LockKeyhole, UploadCloud, X } from "lucide-react";
import { useRef, useState, type FormEvent } from "react";
import { ApiError, createSource, createSourceTreeDraft, fetchChartLibrary, fetchSessionStatus, fetchSource, fetchSourceProcessing, fetchSourceTreeDraft, reprocessSourceFile, uploadSourceFile } from "../lib/api";
import { chartFileError } from "../lib/chart-import";
import { SourceTreeDraftDialog } from "../components/SourceTreeDraftDialog";
import { useChartWorkspace } from "../lib/use-chart-workspace";
import { ChartWorkspaceNavigation } from "../components/ChartWorkspace";
import type { SourceTreeDraft } from "../types";

export function ChartImportPage() {
  const search = useSearch({ from: "/import" });
  const { item: chart } = useChartWorkspace(search.fileId);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const inputRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [title, setTitle] = useState("");
  const [fileError, setFileError] = useState("");
  const [dragging, setDragging] = useState(false);
  const [reviewOpen, setReviewOpen] = useState(false);
  const sessionQuery = useQuery({ queryKey: ["import-session"], queryFn: fetchSessionStatus, staleTime: 0 });
  const sourceQuery = useQuery({ queryKey: ["source", search.sourceId], queryFn: () => fetchSource(search.sourceId!), enabled: Boolean(search.sourceId) });
  const processingQuery = useQuery({
    queryKey: ["source-processing", search.sourceId],
    queryFn: () => fetchSourceProcessing(search.sourceId!),
    enabled: Boolean(search.sourceId && search.fileId),
    staleTime: 0,
    refetchInterval: (query) => {
      const current = query.state.data?.files.find((item) => item.id === search.fileId);
      return current?.processingStatus === "queued" || current?.processingStatus === "running" ? 2000 : false;
    },
  });
  const draftQuery = useQuery({ queryKey: ["import-draft", search.draftId], queryFn: () => fetchSourceTreeDraft(search.draftId!), enabled: Boolean(search.draftId) });
  const uploadedFile = processingQuery.data?.files.find((item) => item.id === search.fileId);
  const draft = draftQuery.data;
  const imported = draft?.state === "imported";
  const ready = uploadedFile?.processingStatus === "succeeded";
  const failed = uploadedFile?.processingStatus === "failed";
  const missingFile = processingQuery.isSuccess && !processingQuery.isFetching && !uploadedFile;
  const sourceTitle = sourceQuery.data?.source.titleAr ?? title;
  const upload = useMutation({
    mutationFn: async () => {
      if (!file) throw new Error("اختر ملف PDF أولاً.");
      let sourceId = search.sourceId;
      if (!sourceId) {
        const created = await createSource({ title_ar: title.trim(), source_type: "family_document" });
        sourceId = created.source.id;
        queryClient.setQueryData(["source", sourceId], created);
        // Preserve the created source even if the subsequent upload fails.
        await navigate({ to: "/import", search: { sourceId }, replace: true });
      }
      const uploaded = await uploadSourceFile(sourceId, file);
      await queryClient.invalidateQueries({ queryKey: ["sources"] });
      await queryClient.invalidateQueries({ queryKey: ["chart-library"] });
      await queryClient.invalidateQueries({ queryKey: ["source-processing", sourceId] });
      await navigate({ to: "/import", search: { sourceId, fileId: uploaded.id }, replace: true });
      setFile(null);
    },
  });
  const review = useMutation({
    mutationFn: async () => {
      if (search.draftId) {
        const saved = await draftQuery.refetch();
        if (saved.error) throw saved.error;
        return;
      }
      const saved = (await fetchChartLibrary()).find((item) => item.fileId === search.fileId);
      const created = saved?.draftId ? await fetchSourceTreeDraft(saved.draftId) : await createSourceTreeDraft(search.fileId!, sourceTitle);
      queryClient.setQueryData(["import-draft", created.id], created);
      await navigate({ to: "/import", search: { ...search, draftId: created.id }, replace: true });
    },
    onSuccess: () => setReviewOpen(true),
  });
  const retry = useMutation({ mutationFn: () => reprocessSourceFile(search.fileId!), onSuccess: async () => { await processingQuery.refetch(); } });
  const error = upload.error ?? review.error ?? retry.error ?? sessionQuery.error ?? sourceQuery.error ?? processingQuery.error ?? draftQuery.error;
  const authRequired = sessionQuery.data === false || error instanceof ApiError && error.status === 401;
  const step = imported ? 3 : ready || search.draftId ? 2 : 1;
  const chooseFile = (next: File | undefined) => {
    if (!next) return;
    const validation = chartFileError(next);
    setFileError(validation ?? "");
    setFile(validation ? null : next);
    upload.reset();
    if (!validation && !title) setTitle(next.name.replace(/\.pdf$/i, "").slice(0, 200));
    if (inputRef.current) inputRef.current.value = "";
  };
  const updateDraft = (updated: SourceTreeDraft) => {
    queryClient.setQueryData(["import-draft", updated.id], updated);
    void queryClient.invalidateQueries({ queryKey: ["chart-library"] });
    if (updated.state === "imported") void queryClient.invalidateQueries({ queryKey: ["trees"] });
  };
  const submit = (event: FormEvent) => { event.preventDefault(); upload.mutate(); };

  return <div className="page-stack chart-import-page">
    <header className="chart-import-heading">
      <div className="toolbar-breadcrumb"><Link to="/tree">أشجاري</Link><span>/</span>{search.fileId ? <><Link to="/tree/charts/$fileId" params={{ fileId: search.fileId }}>{chart?.titleAr ?? "مساحة المشجر"}</Link><span>/</span><span>الوصلات والشجرة</span></> : <span>استيراد مشجر</span>}</div>
      <h1>{search.fileId ? "الوصلات والشجرة" : "استيراد مشجر"}</h1>
      <p>ارفع ملف PDF، ثم راجع الأسماء والوصلات لإنشاء شجرة خاصة.</p>
    </header>
    {chart ? <ChartWorkspaceNavigation item={chart} current="connections" /> : null}
    <ol className="chart-import-steps" aria-label="مراحل استيراد المشجر">
      {["رفع الملف", "مراجعة الأسماء والوصلات", "إنشاء الشجرة"].map((label, index) => <li key={label} className={index + 1 === step ? "is-current" : index + 1 < step ? "is-complete" : ""} aria-current={index + 1 === step ? "step" : undefined}><span>{index + 1 < step ? <Check size={17} /> : index + 1}</span><strong>{label}</strong></li>)}
    </ol>
    {error || authRequired ? <div className="evidence-workspace-error chart-import-error" role="alert">{authRequired ? "سجّل الدخول لحفظ المشجر ومتابعة مراجعته." : error instanceof Error ? error.message : "تعذر إكمال الاستيراد. حاول مرة أخرى."}{authRequired ? <a className="inline-link" href={`/login?returnTo=${encodeURIComponent(`/import${search.sourceId ? `?${new URLSearchParams(Object.entries(search).filter((entry): entry is [string, string] => Boolean(entry[1]))).toString()}` : ""}`)}`}>تسجيل الدخول <ArrowLeft size={15} /></a> : <button className="inline-link" type="button" onClick={() => { void sessionQuery.refetch(); if (search.sourceId) void sourceQuery.refetch(); if (search.fileId) void processingQuery.refetch(); if (search.draftId) void draftQuery.refetch(); }}>تحديث الحالة</button>}</div> : null}
    {!search.fileId && !search.draftId ? <form className="chart-import-surface" onSubmit={submit}>
      <div className="chart-import-form-heading"><h2>ارفع المشجر</h2><p>PDF حتى 128 ميغابايت.</p></div>
      <div className={`chart-file-drop${dragging ? " is-dragging" : ""}${file ? " has-file" : ""}`} onDragOver={(event) => { event.preventDefault(); if (!upload.isPending) setDragging(true); }} onDragLeave={() => setDragging(false)} onDrop={(event) => { event.preventDefault(); setDragging(false); if (!upload.isPending) chooseFile(event.dataTransfer.files[0]); }}>
        {file ? <FileText size={34} strokeWidth={1.4} /> : <UploadCloud size={38} strokeWidth={1.4} />}
        <strong>{file ? file.name : "اختر ملف PDF أو اسحبه هنا"}</strong>
        {file ? <small>{(file.size < 1024 * 1024 ? Math.max(1, file.size / 1024) : file.size / 1024 / 1024).toLocaleString("ar-SA", { maximumFractionDigits: 1 })} {file.size < 1024 * 1024 ? "كيلوبايت" : "ميغابايت"}</small> : null}
        <input ref={inputRef} type="file" accept="application/pdf,.pdf" aria-label="ملف المشجر PDF" onChange={(event) => chooseFile(event.target.files?.[0])} disabled={upload.isPending} />
        <button className="secondary-button" type="button" disabled={upload.isPending} onClick={() => inputRef.current?.click()}>{file ? "تغيير الملف" : "اختيار ملف"}</button>
      </div>
      {fileError ? <p className="source-processing-error" role="alert">{fileError}</p> : null}
      <label className="composer-label chart-import-title">اسم الشجرة<input required maxLength={200} value={search.sourceId ? sourceTitle : title} disabled={upload.isPending || Boolean(search.sourceId)} onChange={(event) => setTitle(event.target.value)} placeholder="مثال: مشجر أسرة العنبر" /></label>
      <p className="chart-import-privacy"><LockKeyhole size={16} /> الشجرة خاصة حتى تختار نشرها.</p>
      <div className="chart-import-footer"><button className="primary-button" type="submit" disabled={!file || !(search.sourceId ? sourceTitle : title).trim() || upload.isPending || sessionQuery.data !== true}>{upload.isPending ? <LoaderCircle className="spin" size={17} /> : <ArrowLeft size={17} />}{upload.isPending ? "جارٍ رفع الملف…" : "رفع ومتابعة"}</button><Link to="/tree" className="inline-link">العودة إلى أشجاري</Link></div>
    </form> : <section className="chart-import-surface chart-import-status" aria-live="polite" aria-busy={processingQuery.isFetching && !uploadedFile}>
      {imported ? <><Check size={40} /><h2>أُنشئت شجرتك الخاصة</h2><p>{draft.titleAr}</p><p>يمكنك الآن تصفّحها وتحريرها. تبقى خاصة حتى تختار نشرها.</p>{draft.treeId ? <Link className="primary-button" to="/tree/$treeId" params={{ treeId: draft.treeId }}><GitBranch size={17} /> فتح الشجرة الخاصة</Link> : null}</> : <>
        {failed || missingFile ? <X size={36} /> : ready || draft ? <Check size={36} /> : <LoaderCircle className="spin" size={36} />}
        <h2>{draft?.state === "retracted" ? "سُجّل التراجع عن الاستيراد" : draft ? "مراجعتك محفوظة" : ready ? "المشجر جاهز للمراجعة" : missingFile ? "لم نعثر على الملف في هذا المصدر" : failed ? "تعذرت قراءة الملف" : "جارٍ تجهيز المشجر"}</h2>
        <strong className="chart-import-filename">{uploadedFile?.originalFilenameAr ?? sourceTitle}</strong>
        <p>{draft?.state === "retracted" ? "يمكنك فتح سجل المراجعة أو بدء استيراد جديد." : ready || draft ? "تحقّق من الأسماء على الصورة الأصلية، ثم حدّد الوصلات الظاهرة. لن تُضاف أي علاقة تلقائياً." : missingFile ? "افتح المصدر المحفوظ واختر الملف الذي تريد متابعة استيراده." : failed ? "الملف الأصلي محفوظ. أعد المحاولة أو افتح أدوات المصدر لفحصه." : "نجهّز صفحات الملف للمراجعة. يمكنك مغادرة الصفحة والعودة من المصادر؛ لن تحتاج إلى رفعه مرة أخرى."}</p>
        {uploadedFile?.processingError ? <details className="chart-import-technical"><summary>تفاصيل تعذّر القراءة</summary><p>{uploadedFile.processingError}</p></details> : null}
        {search.fileId ? <Link className="primary-button" to="/tree/charts/$fileId/names" params={{ fileId: search.fileId }}>مراجعة اقتراحات الأسماء</Link> : null}
        {ready || draft ? <button className="secondary-button" type="button" disabled={review.isPending || draftQuery.isFetching} onClick={() => review.mutate()}>{review.isPending ? <LoaderCircle className="spin" size={17} /> : <ArrowLeft size={17} />}{review.isPending ? "جارٍ فتح المراجعة…" : draft ? "متابعة المراجعة" : "مراجعة الأسماء والوصلات"}</button> : failed ? <button className="primary-button" type="button" disabled={retry.isPending} onClick={() => retry.mutate()}>{retry.isPending ? "جارٍ إعادة المحاولة…" : "إعادة محاولة القراءة"}</button> : null}
        <Link className="inline-link" to="/sources" search={{ sourceId: search.sourceId }}>فتح المصدر المحفوظ <ArrowLeft size={14} /></Link>
      </>}
      <p className="chart-import-privacy"><LockKeyhole size={16} /> الشجرة خاصة حتى تختار نشرها.</p>
    </section>}
    <p className="chart-import-resume">لديك مشجر محفوظ؟ <Link className="inline-link" to="/tree">افتح أشجاري</Link></p>
    {reviewOpen && draft ? <SourceTreeDraftDialog key={draft.id} initialDraft={draft} onClose={() => setReviewOpen(false)} onChanged={updateDraft} /> : null}
  </div>;
}
