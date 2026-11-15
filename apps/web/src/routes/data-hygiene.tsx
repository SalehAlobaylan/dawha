import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, BadgeCheck, ShieldAlert } from "lucide-react";
import { TopBar } from "../components/TopBar";
import { ApiError, fetchDataHygieneReport } from "../lib/api";

/**
 * The license dashboard. Its rule is the archive's rule for external data: a
 * fact imported from outside points back at the source that carried it, so a
 * source with no recorded license puts everything downstream of it outside the
 * terms of use. The page reads one moderator report and shows it; it fixes
 * nothing and hides the refusals — being without the role is an answer.
 */
export function DataHygienePage() {
  const report = useQuery({ queryKey: ["data-hygiene"], queryFn: fetchDataHygieneReport });

  if (report.isPending) {
    return (
      <div className="page-stack">
        <TopBar eyebrow="نظافة البيانات" title="لوحة تراخيص المصادر" description="يُقرأ تقرير التراخيص من المكتبة..." />
        <p className="hygiene-note">يُجهَّز التقرير…</p>
      </div>
    );
  }

  if (report.isError) {
    const status = report.error instanceof ApiError ? report.error.status : undefined;
    const copy = status === 401
      ? "هذه اللوحة تُقرأ بجلسة موثقة. افتح حسابك ثم عد."
      : status === 403
        ? "اللوحة بحاجة إلى دور المشرف أو المدير؛ طلبها بدور آخر هو طلب ما لا تملك صلاحية رؤيته."
        : "تعذر قراءة التقرير من المكتبة. خطأ حقيقي يُعاد محاولته، لا أرقام مصنوعة تُعرض بدلاً منه.";
    return (
      <div className="page-stack">
        <TopBar eyebrow="نظافة البيانات" title="لوحة تراخيص المصادر" description="من يقرأ التقرير ومن يُمنع جزء من القاعدة نفسها." />
        <section className="hygiene-refusal">
          <ShieldAlert size={18} />
          <p>{copy}</p>
        </section>
        <Link to="/sources" className="inline-link">إلى المكتبة <ArrowLeft size={14} /></Link>
      </div>
    );
  }

  const data = report.data;
  const documented = data.sources.filter((source) => source.has_license);
  const missing = data.sources.filter((source) => !source.has_license);

  return (
    <div className="page-stack">
      <TopBar
        eyebrow="نظافة البيانات"
        title="لوحة تراخيص المصادر"
        description="كل مصدر خارجي يحمل ترخيصه: ما يُسمح به لما تنزل عنه من فقرات وارتباطات."
      />
      <section className="hygiene-totals" aria-label="إجماليات التراخيص">
        <div className="hygiene-total">
          <b>{data.total_sources}</b>
          <span>إجمالي المصادر المقيّدة</span>
        </div>
        <div className={`hygiene-total${data.missing_license_count > 0 ? " hygiene-total-warning" : ""}`}>
          <b>{data.missing_license_count}</b>
          <span>مصادر بلا ترخيص</span>
        </div>
      </section>
      {data.truncated ? (
        <p className="hygiene-note">
          القائمة مقتطعة عند {data.limit} مصدراً؛ الإجماليات أعلاه محسوبة على كامل الجدول، فلا يغرق القطع الحقيقة.
        </p>
      ) : null}
      <section aria-label="صفوف المصادر">
        <table className="hygiene-table">
          <thead>
            <tr>
              <th>المصدر</th>
              <th>النوع</th>
              <th>الترخيص</th>
              <th>ملفات</th>
              <th>فقرات</th>
              <th>ارتباطات</th>
            </tr>
          </thead>
          <tbody>
            {data.sources.map((source) => (
              <tr key={source.id} className={source.has_license ? "" : "hygiene-row-missing"}>
                <td>
                  <Link to="/sources" search={{ sourceId: source.id }} className="inline-link">{source.title_ar}</Link>
                  {source.license_note ? <small> · {source.license_note}</small> : null}
                </td>
                <td>{sourceTypeLabel(source.source_type)}</td>
                <td>
                  {source.has_license ? (
                    <span className="hygiene-badge hygiene-badge-ok"><BadgeCheck size={13} /> {source.license}</span>
                  ) : (
                    <span className="hygiene-badge hygiene-badge-missing"><ShieldAlert size={13} /> بلا ترخيص</span>
                  )}
                </td>
                <td>{source.file_count}</td>
                <td>{source.passage_count}</td>
                <td>{source.association_count}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {data.sources.length === 0 ? <p className="hygiene-note">لا مصادر مقيّدة بعد؛ ستظهر هنا أول ما يُستورد.</p> : null}
      </section>
      <p className="hygiene-note">
        التراخيص تُقرأ من ملف المصدر نفسه، والإصلاح يتم من صفحة المصدر: هذه اللوحة تسمّي الدين ولا تسددها.
        الموثقة الآن: {documented.length} · الناقصة: {missing.length}.
      </p>
    </div>
  );
}

const sourceTypeLabels: Record<string, string> = {
  book: "كتاب",
  manuscript: "مخطوط",
  family_document: "وثيقة عائلة",
  archive_record: "سجل أرشيفي",
  newspaper: "صحيفة",
  article: "مقالة",
  research_paper: "بحث",
  oral_testimony: "شهادة شفهية",
  website: "موقع",
  published_tree: "شجرة منشورة",
  historical_registry: "سجل تاريخي",
  other: "أخرى",
};

function sourceTypeLabel(sourceType: string): string {
  return sourceTypeLabels[sourceType] ?? sourceType;
}
