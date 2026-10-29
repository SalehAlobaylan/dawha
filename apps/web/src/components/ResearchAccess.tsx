import { useQuery } from "@tanstack/react-query";
import { fetchResearchAccess } from "../lib/api";

export function ResearchAccess({ children }: { children: React.ReactNode }) {
  const query = useQuery({ queryKey: ["research-access"], queryFn: fetchResearchAccess, staleTime: 30000, retry: false });
  if (query.isPending) return <p role="status">جارٍ التحقق من صلاحية المراجعة…</p>;
  if (query.error) return <div className="entity-resolution-error" role="alert">تعذر التحقق من صلاحية المراجعة. <button className="inline-link" onClick={() => void query.refetch()} type="button">إعادة المحاولة</button></div>;
  if (!query.data?.canReview) return <div className="research-access-note"><h3>مراجعة سجلات المنصة</h3><p>{query.data?.signedIn ? "تحتاج هذه الأدوات إلى صلاحية باحث أو مشرف. يمكنك متابعة أشجارك وأسئلتك ومراجعة أسماء مشجراتك من مساحتك." : "سجّل الدخول بحساب لديه صلاحية باحث أو مشرف للوصول إلى مراجعة الهوية والتعارضات."}</p>{!query.data?.signedIn ? <a className="primary-button" href="/login?returnTo=%2Fresearch%3Fview%3Dreview">تسجيل الدخول</a> : null}</div>;
  return children;
}
