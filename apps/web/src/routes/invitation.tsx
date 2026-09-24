import { useMutation } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, Check, Link2, LoaderCircle } from "lucide-react";
import { useState } from "react";
import { acceptInvitation, ApiError } from "../lib/api";
import { BrandMark } from "../components/BrandMark";

type InvitationPageProps = {
  token?: string;
};

type AcceptedInvitation = {
  invitationId: string;
  treeId: string;
  treeName: string;
  permissionLevel: "view" | "edit" | "review";
};

export function InvitationPage({ token = "" }: InvitationPageProps) {
  const [accepted, setAccepted] = useState<AcceptedInvitation | null>(null);
  const acceptMutation = useMutation({
    mutationFn: () => acceptInvitation(token),
    onSuccess: (result) => setAccepted(result),
  });
  const loginPath = `/login?returnTo=${encodeURIComponent(`/invitation/${token}`)}`;
  const error = acceptMutation.error;

  return (
    <div className="auth-page">
      <div className="auth-topline"><Link to="/" className="auth-brand"><BrandMark /><strong>دَوْحة</strong></Link><Link to="/" className="auth-back">العودة إلى المساحة</Link></div>
      <main className="auth-card invitation-card">
        <div className="auth-card-intro"><div className="eyebrow">دعوة تعاون</div><h1>مشاركة تفسير، لا حقيقة نهائية</h1><p>ستُضاف صلاحية محدودة إلى هذه الشجرة فقط، وتظل الصلاحيات القابلة للتعديل من صاحبها.</p></div>
        {accepted ? <div className="invitation-success"><Check size={22} /><strong>قبلت الدعوة</strong><p>أصبحت «{accepted.treeName}» ضمن مساحة تعاونك بصلاحية {permissionLabel(accepted.permissionLevel)}.</p><Link className="primary-button" to="/tree/$treeId" params={{ treeId: accepted.treeId }}>فتح الشجرة <ArrowLeft size={16} /></Link></div> : <div className="invitation-action"><div className="invitation-icon"><Link2 size={22} /></div><p>تحقق من أن الرابط يخص مساحة دَوْحة، ثم اقبل الدعوة.</p>{error ? <div className="auth-message" role="alert">{error instanceof ApiError && error.status === 401 ? <>{error.message} <Link to={loginPath}>سجّل الدخول</Link></> : error.message}</div> : null}<button className="primary-button auth-submit" type="button" onClick={() => acceptMutation.mutate()} disabled={acceptMutation.isPending}>{acceptMutation.isPending ? <LoaderCircle className="spin" size={16} /> : <Link2 size={16} />} {acceptMutation.isPending ? "جارٍ التحقق…" : "قبول الدعوة"}</button></div>}
      </main>
      <div className="auth-bottom"><span>الصلاحية محصورة بالشجرة</span><span>الدعوة قابلة للإلغاء</span></div>
    </div>
  );
}

function permissionLabel(permission: "view" | "edit" | "review"): string {
  if (permission === "edit") return "تحرير";
  if (permission === "review") return "مراجعة";
  return "قراءة";
}
