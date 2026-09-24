import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, GitCompareArrows, Link2, LoaderCircle, LockKeyhole, Plus, Send, Share2, SlidersHorizontal, UserPlus } from "lucide-react";
import { FormEvent, useMemo, useState } from "react";
import { addPerson, addRelationship, ApiError, createTree, demoTreeDetail, fetchPublicTrees, fetchTree, publishTree } from "../lib/api";
import { EvidenceMiniList, ResearchGraph } from "../components/ResearchGraph";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";
import type { AddPersonInput, AddRelationshipInput, TreeDetail, TreeNode } from "../types";

export function TreePage() {
  const queryClient = useQueryClient();
  const [activeTreeId, setActiveTreeId] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState("");
  const [showUnresolved, setShowUnresolved] = useState(true);
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [firstPerson, setFirstPerson] = useState("");
  const [visibility, setVisibility] = useState<"private" | "unlisted" | "public">("private");
  const [editOpen, setEditOpen] = useState(false);
  const [personName, setPersonName] = useState("");
  const [personGender, setPersonGender] = useState<"male" | "female" | "unknown">("unknown");
  const [birthDateFrom, setBirthDateFrom] = useState("");
  const [birthDateTo, setBirthDateTo] = useState("");
  const [deathDateFrom, setDeathDateFrom] = useState("");
  const [deathDateTo, setDeathDateTo] = useState("");
  const [parentNodeId, setParentNodeId] = useState("");
  const [childNodeId, setChildNodeId] = useState("");
  const [relationshipStatus, setRelationshipStatus] = useState<"interpreted" | "disputed" | "unresolved">("interpreted");
  const [message, setMessage] = useState("");

  const treesQuery = useQuery({ queryKey: ["trees"], queryFn: fetchPublicTrees });
  const availableTrees = treesQuery.data ?? [];
  const treeId = activeTreeId ?? availableTrees[0]?.id ?? "tree-demo";
  const treeQuery = useQuery({
    queryKey: ["tree", treeId],
    queryFn: () => fetchTree(treeId),
    enabled: Boolean(treeId),
  });
  const detail = treeQuery.data ?? demoTreeDetail;
  const graphNodes = useMemo(() => toGraphNodes(detail), [detail]);
  const visibleGraphNodes = showUnresolved ? graphNodes : graphNodes.filter((node) => node.tone !== "disputed" && node.tone !== "question");
  const visibleNodeIds = new Set(visibleGraphNodes.map((node) => node.id));
  const visibleRelationships = detail.relationships.filter((relationship) => visibleNodeIds.has(relationship.subjectNodeId) && visibleNodeIds.has(relationship.objectNodeId));
  const selected = visibleGraphNodes.find((node) => node.id === selectedId) ?? visibleGraphNodes[0];
  const canEditDraft = detail.tree.latestState === "draft" && detail.tree.id !== "tree-demo";

  const updateDetail = async (updated: TreeDetail) => {
    queryClient.setQueryData(["tree", updated.tree.id], updated);
    await queryClient.invalidateQueries({ queryKey: ["trees"] });
  };

  const createMutation = useMutation({
    mutationFn: createTree,
    onSuccess: async (created) => {
      setActiveTreeId(created.tree.id);
      queryClient.setQueryData(["tree", created.tree.id], created);
      await queryClient.invalidateQueries({ queryKey: ["trees"] });
      setCreateOpen(false);
      setName("");
      setDescription("");
      setFirstPerson("");
      setMessage(`أُنشئت «${created.tree.name}» كمسودة.`);
    },
    onError: (error) => setMessage(authMessage(error)),
  });

  const addPersonMutation = useMutation({
    mutationFn: (input: AddPersonInput) => addPerson(detail.tree.id, input),
    onSuccess: async (updated) => {
      await updateDetail(updated);
      setSelectedId(updated.nodes[updated.nodes.length - 1]?.id ?? "");
      setPersonName("");
      setBirthDateFrom("");
      setBirthDateTo("");
      setDeathDateFrom("");
      setDeathDateTo("");
      setMessage("أُضيف الشخص إلى المسودة الحالية.");
    },
    onError: (error) => setMessage(authMessage(error)),
  });

  const addRelationshipMutation = useMutation({
    mutationFn: (input: AddRelationshipInput) => addRelationship(detail.tree.id, input),
    onSuccess: async (updated) => {
      await updateDetail(updated);
      setParentNodeId("");
      setChildNodeId("");
      setMessage("أُضيفت العلاقة إلى تفسير المسودة.");
    },
    onError: (error) => setMessage(authMessage(error)),
  });

  const publishMutation = useMutation({
    mutationFn: () => publishTree(detail.tree.id, "نشر نسخة جديدة من التفسير"),
    onSuccess: async (published) => {
      queryClient.setQueryData(["tree", published.tree.id], published);
      await queryClient.invalidateQueries({ queryKey: ["trees"] });
      setMessage(`نُشرت النسخة ${published.tree.latestVersionNumber}، وفُتحت مسودة جديدة للتعديل.`);
    },
    onError: (error) => setMessage(authMessage(error)),
  });

  const submitCreate = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage("");
    createMutation.mutate({
      name_ar: name,
      description_ar: description,
      visibility,
      people: firstPerson ? [{ canonical_name_ar: firstPerson }] : [],
    });
  };

  const submitPerson = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage("");
    addPersonMutation.mutate({
      canonical_name_ar: personName,
      gender: personGender,
      birth_date_from: birthDateFrom || undefined,
      birth_date_to: birthDateTo || undefined,
      death_date_from: deathDateFrom || undefined,
      death_date_to: deathDateTo || undefined,
    });
  };

  const submitRelationship = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage("");
    addRelationshipMutation.mutate({
      subject_node_id: parentNodeId,
      object_node_id: childNodeId,
      predicate: "parent_of",
      status: relationshipStatus,
    });
  };

  return (
    <div className="page-stack">
      <TopBar
        eyebrow="شجرة بحث / تفسير محفوظ"
        title={detail.tree.name}
        description={detail.tree.description || "افتح علاقة لترى من أين جاءت، وما الذي ما زال غير محسوم."}
      />
      <div className="tree-page-toolbar">
        <div className="toolbar-breadcrumb"><span>الأشجار</span><b>/</b><strong>{detail.tree.name}</strong></div>
        <label className="tree-selector-label">الملف الحالي<select className="tree-selector" aria-label="اختيار الشجرة" value={treeId} onChange={(event) => { setActiveTreeId(event.target.value); setSelectedId(""); }}>
          {availableTrees.map((tree) => <option key={tree.id} value={tree.id}>{tree.name} · {tree.latestState === "published" ? "منشورة" : "مسودة"}</option>)}
        </select></label>
        <div className="toolbar-button-group">
          <button className="secondary-button" type="button" onClick={() => setCreateOpen((open) => !open)}><Plus size={15} /> شجرة جديدة</button>
          <button className="secondary-button" type="button" onClick={() => setEditOpen((open) => !open)}><UserPlus size={15} /> تحرير المسودة</button>
          <button className="secondary-button" type="button"><Share2 size={15} /> مشاركة</button>
          <button className="secondary-button" type="button"><GitCompareArrows size={15} /> مقارنة النسخ</button>
          <button className="primary-button" type="button" onClick={() => publishMutation.mutate()} disabled={publishMutation.isPending}>
            {publishMutation.isPending ? <LoaderCircle className="spin" size={15} /> : <Send size={15} />} نشر المسودة
          </button>
        </div>
      </div>

      {message ? <div className="tree-action-message" role="status">{message}{message.includes("تسجيل الدخول") ? <Link to="/login">فتح تسجيل الدخول</Link> : null}</div> : null}

      {createOpen ? (
        <form className="tree-create-panel" onSubmit={submitCreate}>
          <div className="tree-create-head"><div><div className="eyebrow">مسودة جديدة</div><h2>ابدأ شجرة تفسيرية</h2></div><StatusBadge tone="claim">غير محسومة بعد</StatusBadge></div>
          <div className="tree-create-grid">
            <label className="composer-label">اسم الشجرة<input required value={name} onChange={(event) => setName(event.target.value)} placeholder="مثال: شجرة بيت العنبر" /></label>
            <label className="composer-label">الوصف<textarea value={description} onChange={(event) => setDescription(event.target.value)} placeholder="ما الذي تحاول هذه الشجرة تفسيره؟" rows={2} /></label>
            <label className="composer-label">الظهور<select value={visibility} onChange={(event) => setVisibility(event.target.value as typeof visibility)}><option value="private">خاصة</option><option value="unlisted">غير مدرجة</option><option value="public">عامة</option></select></label>
            <label className="composer-label">أول شخص (اختياري)<input value={firstPerson} onChange={(event) => setFirstPerson(event.target.value)} placeholder="مثال: محمد بن سعد" /></label>
          </div>
          <div className="tree-create-actions"><button className="secondary-button" type="button" onClick={() => setCreateOpen(false)}>إلغاء</button><button className="primary-button" type="submit" disabled={createMutation.isPending}>{createMutation.isPending ? "جارٍ الحفظ…" : "احفظ كمسودة"}<ArrowLeft size={15} /></button></div>
        </form>
      ) : null}

      {editOpen ? (
        <section className="tree-edit-panel">
          <div className="tree-edit-head">
            <div><div className="eyebrow">تعديل تفسيري</div><h2>حرّر المسودة الحالية</h2><p>كل إضافة تُحفظ داخل نسخة الشجرة، ولا تتحول تلقائياً إلى حقيقة تاريخية.</p></div>
            <StatusBadge tone="claim">نسخة محفوظة</StatusBadge>
          </div>
          {!canEditDraft ? <p className="tree-edit-note">هذه العرض نسخة منشورة أو تجريبية. افتح مسودة مملوكة لك لتفعيل التعديل.</p> : (
            <div className="tree-edit-grid">
              <form className="tree-edit-form" onSubmit={submitPerson}>
                <div className="tree-edit-form-head"><div><h3>إضافة شخص</h3><p>أضف الاسم كما يظهر في السجل، مع تواريخ تقريبية عند توفرها.</p></div><UserPlus size={17} /></div>
                <div className="tree-edit-fields">
                  <label className="composer-label">الاسم بالعربية<input required value={personName} onChange={(event) => setPersonName(event.target.value)} placeholder="مثال: فاطمة بنت محمد" /></label>
                  <label className="composer-label">الجنس<select value={personGender} onChange={(event) => setPersonGender(event.target.value as typeof personGender)}><option value="unknown">غير محدد</option><option value="female">أنثى</option><option value="male">ذكر</option></select></label>
                  <label className="composer-label">من سنة الميلاد<input type="date" value={birthDateFrom} onChange={(event) => setBirthDateFrom(event.target.value)} /></label>
                  <label className="composer-label">إلى سنة الميلاد<input type="date" value={birthDateTo} onChange={(event) => setBirthDateTo(event.target.value)} /></label>
                  <label className="composer-label">من سنة الوفاة<input type="date" value={deathDateFrom} onChange={(event) => setDeathDateFrom(event.target.value)} /></label>
                  <label className="composer-label">إلى سنة الوفاة<input type="date" value={deathDateTo} onChange={(event) => setDeathDateTo(event.target.value)} /></label>
                </div>
                <div className="tree-edit-actions"><button className="primary-button" type="submit" disabled={addPersonMutation.isPending}>{addPersonMutation.isPending ? "جارٍ الحفظ…" : "أضف إلى المسودة"}<Plus size={15} /></button></div>
              </form>

              <form className="tree-edit-form" onSubmit={submitRelationship}>
                <div className="tree-edit-form-head"><div><h3>ربط أب وابن</h3><p>اختر ترتيب العلاقة؛ يبقى تصنيفها تفسيرياً داخل هذه الشجرة.</p></div><Link2 size={17} /></div>
                {detail.nodes.length < 2 ? <p className="tree-edit-note">أضف شخصين أولاً حتى يمكنك ربطهما.</p> : <div className="tree-edit-fields">
                  <label className="composer-label">الأب أو الوالد<select required value={parentNodeId} onChange={(event) => setParentNodeId(event.target.value)}><option value="">اختر الأب أو الوالد</option>{detail.nodes.map((node) => <option key={node.id} value={node.id}>{node.displayName}</option>)}</select></label>
                  <label className="composer-label">الابن أو الابنة<select required value={childNodeId} onChange={(event) => setChildNodeId(event.target.value)}><option value="">اختر الابن أو الابنة</option>{detail.nodes.map((node) => <option key={node.id} value={node.id}>{node.displayName}</option>)}</select></label>
                  <label className="composer-label">حالة التفسير<select value={relationshipStatus} onChange={(event) => setRelationshipStatus(event.target.value as typeof relationshipStatus)}><option value="interpreted">مفسر</option><option value="disputed">متنازع عليه</option><option value="unresolved">غير محسوم</option></select></label>
                </div>}
                <div className="tree-edit-actions"><button className="primary-button" type="submit" disabled={addRelationshipMutation.isPending || detail.nodes.length < 2}>{addRelationshipMutation.isPending ? "جارٍ الربط…" : "أضف العلاقة"}<Link2 size={15} /></button></div>
              </form>
            </div>
          )}
        </section>
      ) : null}

      <div className="tree-layout">
        <section className="tree-explorer-panel">
          <div className="tree-explorer-head">
            <div>
              <div className="tree-status-line"><StatusBadge tone={detail.tree.latestState === "published" ? "interpretation" : "claim"}>{detail.tree.latestState === "published" ? "منشورة" : "مسودة"}</StatusBadge><span>النسخة {detail.tree.latestVersionNumber} من {detail.versions.length || 1}</span></div>
              <h2>الشجرة كما يعرضها هذا التفسير</h2>
              <p>هذه العلاقات تشرح هذا العرض، ولا تمثل حقيقة تاريخية نهائية.</p>
            </div>
            <button className="icon-button" type="button" aria-label="فلترة الشجرة"><SlidersHorizontal size={17} /></button>
          </div>
          <div className="tree-filter-row">
            <button className={`filter-chip${showUnresolved ? " filter-chip-active" : ""}`} type="button" onClick={() => setShowUnresolved((visible) => !visible)}>
              <span className="filter-chip-dot filter-dot-disputed" /> إظهار غير المحسوم
            </button>
            <span className="filter-count">{detail.tree.people} أشخاص · {detail.tree.relationships} علاقات</span>
          </div>
          <ResearchGraph selected={selected?.id ?? ""} onSelect={(node) => setSelectedId(node.id)} nodes={visibleGraphNodes} relationships={visibleRelationships} label="رسم الشجرة الرقمية" />
          <div className="tree-canvas-footer"><LockKeyhole size={13} /> {detail.tree.latestState === "published" ? "النسخة المنشورة ثابتة. التعديلات الجديدة تنشئ نسخة مسودة." : "هذه مسودة قابلة للتعديل، ولا تمثل تفسيراً منشوراً."}</div>
        </section>

        <aside className="node-detail-panel">
          {selected ? <>
            <div className="node-detail-top">
              <div className="node-detail-avatar">{selected.name.slice(0, 1)}</div>
              <div><div className="eyebrow">شخص محدد</div><h2>{selected.name}</h2><p>{selected.years} · {selected.role}</p></div>
            </div>
            <div className="node-detail-alert"><StatusBadge tone={selected.tone}>{selected.tone === "disputed" ? "في مركز سؤال" : "ضمن التفسير"}</StatusBadge><span>{selected.sourceCount} إشارات مرتبطة</span></div>
            <div className="detail-block"><div className="detail-label">ملاحظة الباحث</div><p>{selected.note}</p></div>
            <div className="detail-block"><div className="detail-label">ما تمثله هذه الشجرة</div><p>تضع هذه النسخة {selected.name} داخل تفسيرها، مع إبقاء الخلافاتطبقة كما هي.</p></div>
            <div className="detail-block"><div className="detail-label">المصادر القريبة</div><EvidenceMiniList sourceCount={selected.sourceCount} /></div>
            <Link to="/research" className="detail-cta">افتح هذا الشخص في مكتب البحث <ArrowLeft size={15} /></Link>
          </> : <div className="node-empty-state"><h2>لا يوجد شخص بعد</h2><p>احفظ مسودة، ثم أضف أول شخص لتبدأ الشجرة.</p></div>}
        </aside>
      </div>

      <section className="version-strip">
        <div><div className="eyebrow">سجل النسخ</div><h2>ما الذي تغيّر بين الإصدارات؟</h2><p>الاختلاف هنا ليس خطأً؛ قد يكون بداية لسؤال جديد.</p></div>
        <div className="version-timeline">
          {detail.versions.slice(0, 3).map((version, index) => <div className={`version-item${index === 0 ? " version-item-current" : ""}`} key={version.id}><span>v{version.number}</span><div><strong>{version.state === "draft" ? "مسودة حالية" : index === 0 ? "النشر الحالي" : "نسخة منشورة"}</strong><small>{version.publicationNote || "دون ملاحظة نشر"}</small></div></div>)}
        </div>
      </section>
    </div>
  );
}

function toGraphNodes(detail: TreeDetail): TreeNode[] {
  return detail.nodes.map((node, index) => ({
    id: node.id,
    name: node.displayName,
    role: node.role,
    years: node.years,
    x: 14 + (index % 4) * 24,
    y: 24 + Math.floor(index / 4) * 30,
    tone: node.tone,
    sourceCount: node.sourceCount,
    note: node.note,
  }));
}

function authMessage(error: unknown): string {
  if (error instanceof ApiError && error.status === 401) {
    return "سجّل الدخول أولًا لإنشاء أو تعديل أو نشر شجرة.";
  }
  return error instanceof Error ? error.message : "تعذر إكمال العملية.";
}
