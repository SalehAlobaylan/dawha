import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Clipboard, LoaderCircle, Mail, ShieldCheck, Trash2, UserPlus, X } from "lucide-react";
import { FormEvent, useState } from "react";
import { ApiError, createInvitation, fetchCollaborators, fetchTreeActivity, removeCollaborator, revokeInvitation, updateCollaboratorPermission } from "../lib/api";
import { StatusBadge } from "./StatusBadge";
import type { TreeActivity, TreePermissionLevel } from "../types";

type CollaborationPanelProps = {
  treeId: string;
  canManage: boolean;
  permissionLevel: TreePermissionLevel | "";
};

type PermissionLevel = "view" | "edit" | "review";
type MemberLevel = "owner" | PermissionLevel;

export function CollaborationPanel({ treeId, canManage, permissionLevel }: CollaborationPanelProps) {
  const queryClient = useQueryClient();
  const [email, setEmail] = useState("");
  const [invitePermission, setInvitePermission] = useState<PermissionLevel>("edit");
  const [acceptLink, setAcceptLink] = useState("");
  const [message, setMessage] = useState("");
  const collaboratorsQuery = useQuery({
    queryKey: ["tree", treeId, "collaborators"],
    queryFn: () => fetchCollaborators(treeId),
    enabled: treeId !== "tree-demo",
  });
  const activityQuery = useQuery({
    queryKey: ["tree", treeId, "activity"],
    queryFn: () => fetchTreeActivity(treeId),
    enabled: treeId !== "tree-demo",
  });
  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ["tree", treeId, "collaborators"] });
    await queryClient.invalidateQueries({ queryKey: ["tree", treeId, "activity"] });
    await queryClient.invalidateQueries({ queryKey: ["tree", treeId] });
  };
  const inviteMutation = useMutation({
    mutationFn: () => createInvitation(treeId, { invitee_email: email, permission_level: invitePermission }),
    onSuccess: async (result) => {
      setAcceptLink(`${window.location.origin}${result.acceptPath}`);
      setEmail("");
      setMessage("أُنشئت الدعوة. شارك الرابط مرة واحدة مع الباحث.");
      await refresh();
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const permissionMutation = useMutation({
    mutationFn: ({ userId, level }: { userId: string; level: PermissionLevel }) => updateCollaboratorPermission(treeId, userId, { permission_level: level }),
    onSuccess: async () => {
      setMessage("حُدّثت صلاحية المتعاون.");
      await refresh();
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const removeMutation = useMutation({
    mutationFn: (userId: string) => removeCollaborator(treeId, userId),
    onSuccess: async () => {
      setMessage("أُلغيت صلاحية المتعاون.");
      await refresh();
    },
    onError: (error) => setMessage(errorMessage(error)),
  });
  const revokeMutation = useMutation({
    mutationFn: (invitationId: string) => revokeInvitation(treeId, invitationId),
    onSuccess: async () => {
      setMessage("أُلغيت الدعوة.");
      await refresh();
    },
    onError: (error) => setMessage(errorMessage(error)),
  });

  if (treeId === "tree-demo") {
    return <section className="tree-collaboration-panel"><div className="tree-collaboration-head"><div><div className="eyebrow">التعاون</div><h2>مشاركة تفسير محفوظ</h2></div><ShieldCheck size={19} /></div><p className="tree-collaboration-note">هذه شجرة تجريبية للقراءة. أنشئ شجرة حقيقية لتجربة الدعوات وصلاحيات التحرير.</p></section>;
  }

  const collaborators = collaboratorsQuery.data;
  const activity = activityQuery.data ?? [];
  const queryError = collaboratorsQuery.error ?? activityQuery.error;

  return (
    <section className="tree-collaboration-panel">
      <div className="tree-collaboration-head">
        <div><div className="eyebrow">التعاون</div><h2>من يشارك في تفسير هذه الشجرة؟</h2><p>صلاحية المشاركة تبقى داخل هذه الشجرة، ولا تمنح حق النشر تلقائياً.</p></div>
        <div className="collaboration-permission"><StatusBadge tone={permissionLevel === "edit" ? "interpretation" : "claim"}>{permissionLevel ? permissionLabel(permissionLevel) : "قراءة فقط"}</StatusBadge></div>
      </div>
      {queryError ? <div className="tree-collaboration-error" role="alert">{errorMessage(queryError)} {queryError instanceof ApiError && queryError.status === 401 ? <a href={`/login?returnTo=${encodeURIComponent(`/tree/${treeId}`)}`}>سجّل الدخول</a> : null}</div> : null}
      {message ? <div className="tree-collaboration-message" role="status">{message}</div> : null}
      {canManage ? <form className="collaboration-invite-form" onSubmit={(event: FormEvent<HTMLFormElement>) => { event.preventDefault(); inviteMutation.mutate(); }}>
        <div className="collaboration-invite-heading"><Mail size={16} /><div><strong>دعوة باحث</strong><small>أرسل رابطاً واحداً؛ لا يُخزن الرابط بعد إنشائه.</small></div></div>
        <div className="collaboration-invite-fields"><label className="composer-label">البريد الإلكتروني<input required type="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="researcher@example.com" /></label><label className="composer-label">الصلاحية<select value={invitePermission} onChange={(event) => setInvitePermission(event.target.value as PermissionLevel)}><option value="edit">تحرير المسودة</option><option value="view">قراءة فقط</option><option value="review">مراجعة</option></select></label><button className="primary-button" type="submit" disabled={inviteMutation.isPending}>{inviteMutation.isPending ? <LoaderCircle className="spin" size={15} /> : <UserPlus size={15} />} {inviteMutation.isPending ? "جارٍ الإنشاء…" : "أنشئ الدعوة"}</button></div>
      </form> : <p className="tree-collaboration-note">لديك صلاحية {permissionLevel ? permissionLabel(permissionLevel) : "قراءة"} في هذه الشجرة. صاحب الشجرة وحده يدير الدعوات والصلاحيات.</p>}
      {acceptLink ? <div className="collaboration-accept-link"><div><Check size={15} /><span>رابط قبول الدعوة</span></div><code>{acceptLink}</code><button className="icon-button" type="button" aria-label="نسخ رابط الدعوة" onClick={() => { void navigator.clipboard?.writeText(acceptLink); setMessage("نُسخ رابط الدعوة."); }}><Clipboard size={15} /></button></div> : null}
      {collaboratorsQuery.isPending ? <p className="tree-collaboration-note">جارٍ تحميل المتعاونين…</p> : collaborators ? <>
        <div className="collaboration-list-heading"><div><div className="detail-label">الأعضاء</div><small>{collaborators.collaborators.length} متعاونين</small></div><span>{collaborators.canManage ? "إدارة كاملة" : "عرض الصلاحيات"}</span></div>
        <div className="collaboration-member-list">
          <MemberRow name={collaborators.owner.displayName} detail="صاحب الشجرة" level="owner" />
          {collaborators.collaborators.map((member) => <MemberRow key={member.userId} name={member.displayName} detail={member.email ?? "عضو مسجل"} level={member.permissionLevel as PermissionLevel} canManage={canManage} pending={permissionMutation.isPending || removeMutation.isPending} onPermissionChange={(level) => permissionMutation.mutate({ userId: member.userId, level })} onRemove={() => removeMutation.mutate(member.userId)} />)}
        </div>
        {canManage && collaborators.invitations.length > 0 ? <div className="collaboration-invitation-list"><div className="collaboration-list-heading"><div><div className="detail-label">دعوات بانتظار القبول</div><small>{collaborators.invitations.length} دعوات</small></div></div>{collaborators.invitations.map((invitation) => <div className="collaboration-invitation-row" key={invitation.id}><div><strong>{invitation.inviteeEmail ?? "باحث مسجل"}</strong><small>{permissionLabel(invitation.permissionLevel)} · تنتهي {formatDate(invitation.expiresAt)}</small></div><button className="icon-button danger-icon" type="button" aria-label="إلغاء الدعوة" onClick={() => revokeMutation.mutate(invitation.id)} disabled={revokeMutation.isPending}><X size={15} /></button></div>)}</div> : null}
      </> : null}
      <div className="collaboration-activity"><div className="collaboration-list-heading"><div><div className="detail-label">سجل النشاط</div><small>آخر التغييرات الدلالية</small></div><span>{activity.length} أحداث</span></div>{activityQuery.isPending ? <p className="tree-collaboration-note">جارٍ تحميل النشاط…</p> : activity.length > 0 ? <div className="collaboration-activity-list">{activity.slice(0, 6).map((item) => <ActivityRow key={item.id} item={item} />)}</div> : <p className="tree-collaboration-note">لا يوجد نشاط مسجل بعد.</p>}</div>
    </section>
  );
}

function MemberRow({ name, detail, level, canManage = false, pending = false, onPermissionChange, onRemove }: { name: string; detail: string; level: MemberLevel; canManage?: boolean; pending?: boolean; onPermissionChange?: (level: PermissionLevel) => void; onRemove?: () => void }) {
  return <div className="collaboration-member-row"><div className="collaboration-member-avatar">{name.slice(0, 1)}</div><div className="collaboration-member-info"><strong>{name}</strong><small>{detail}</small></div>{level === "owner" ? <StatusBadge tone="interpretation">المالك</StatusBadge> : canManage ? <><select aria-label={`صلاحية ${name}`} value={level} onChange={(event) => onPermissionChange?.(event.target.value as PermissionLevel)} disabled={pending}><option value="view">قراءة</option><option value="edit">تحرير</option><option value="review">مراجعة</option></select><button className="icon-button danger-icon" type="button" aria-label={`إزالة ${name}`} onClick={onRemove} disabled={pending}><Trash2 size={14} /></button></> : <StatusBadge tone={level === "edit" ? "claim" : "source"}>{permissionLabel(level)}</StatusBadge>}</div>;
}

function ActivityRow({ item }: { item: TreeActivity }) {
  return <div className="collaboration-activity-row"><span className="collaboration-activity-dot" /><div><strong>{activityLabel(item.action)}</strong><small>{item.actorName ?? "مستخدم"} · {formatDate(item.createdAt)}</small></div></div>;
}

function permissionLabel(permission: TreePermissionLevel): string {
  if (permission === "owner") return "المالك";
  if (permission === "edit") return "تحرير";
  if (permission === "review") return "مراجعة";
  return "قراءة";
}

function activityLabel(action: string): string {
  const labels: Record<string, string> = { tree_created: "أُنشئت الشجرة", person_added: "أُضيف شخص", relationship_added: "أُضيفت علاقة", relationship_changed: "تغيّرت حالة علاقة", tree_version_published: "نُشرت نسخة", collaborator_invited: "أُنشئت دعوة", invitation_accepted: "قُبلت دعوة", invitation_revoked: "أُلغيت دعوة", collaborator_permission_changed: "تغيّرت صلاحية", collaborator_removed: "أُلغي تعاون" };
  return labels[action] ?? action;
}

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat("ar", { dateStyle: "medium" }).format(date);
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "تعذر إكمال عملية التعاون.";
}
