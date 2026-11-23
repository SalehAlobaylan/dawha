import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ApiError, reviewAtlasBoundary } from "../lib/api";

export function AtlasBoundaryReview({ candidateId }: { candidateId: string }) {
  const client = useQueryClient();
  const save = useMutation({ mutationFn: (form: FormData) => reviewAtlasBoundary(candidateId, {
    sourceId: String(form.get("sourceId") ?? "").trim(),
    occurrenceId: String(form.get("occurrenceId") ?? "").trim(),
    locator: String(form.get("locator") ?? "").trim(),
    validFrom: String(form.get("validFrom") ?? ""),
    validTo: String(form.get("validTo") ?? ""),
    wkt: String(form.get("wkt") ?? "").trim(),
  }) });
  const publish = useMutation({
    mutationFn: () => reviewAtlasBoundary(candidateId),
    onSuccess: () => client.invalidateQueries({ queryKey: ["atlas-map"] }),
  });
  return (
    <details>
      <summary>مراجعة الحدّ المرسوم</summary>
      <p>أرفق موضع الحد في مصدره والفترة التي يشهد لها. يلزم اتفاق مشرفَين قبل نشر النطاق.</p>
      <form className="atlas-source-query" onSubmit={(event) => {
        event.preventDefault();
        save.mutate(new FormData(event.currentTarget));
      }}>
        <label>معرّف المصدر <input name="sourceId" dir="ltr" required maxLength={36} /></label>
        <label>معرّف الشاهد (اختياري) <input name="occurrenceId" dir="ltr" maxLength={36} /></label>
        <label>الصفحة أو موضع الشاهد <input name="locator" required maxLength={500} /></label>
        <label>من <input name="validFrom" type="date" /></label>
        <label>إلى <input name="validTo" type="date" /></label>
        <label>المضلع بصيغة WKT (خط الطول ثم العرض) <textarea name="wkt" dir="ltr" required maxLength={12000} /></label>
        <button type="submit" className="secondary-button" disabled={save.isPending || publish.isPending || publish.isSuccess}>احفظ مراجعة الحد</button>
      </form>
      {save.isSuccess && <p role="status">حُفظت مراجعتك.</p>}
      {save.isError && <p role="alert">تعذّر حفظ الحد. راجع المصدر والتواريخ والمضلع.</p>}
      <button type="button" className="text-button" disabled={save.isPending || publish.isPending || publish.isSuccess} onClick={() => publish.mutate()}>انشر النطاق بعد الاتفاق</button>
      {publish.isSuccess && <p role="status">نُشر النطاق في الأطلس.</p>}
      {publish.isError && <p role="alert">{publish.error instanceof ApiError && publish.error.status === 400
        ? "يلزم اتفاق مشرفَين على المصدر والموضع والحدّ والفترة." : "تعذّر نشر النطاق."}</p>}
    </details>
  );
}
