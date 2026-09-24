import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, Focus, GitCompareArrows, History, Link2, LoaderCircle, LockKeyhole, Maximize2, Plus, Send, Share2, SlidersHorizontal, UserPlus } from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { addPerson, addRelationship, ApiError, createTree, demoTreeDetail, fetchPublicTrees, fetchTree, fetchTreeVersion, publishTree } from "../lib/api";
import { filterUnresolvedRelationships, focusLineage } from "../lib/tree-view";
import { EvidenceMiniList, ResearchGraph } from "../components/ResearchGraph";
import { StatusBadge } from "../components/StatusBadge";
import { TopBar } from "../components/TopBar";
import type { AddPersonInput, AddRelationshipInput, TreeDetail, TreeNode } from "../types";

export function TreePage() {
  const queryClient = useQueryClient();
  const [activeTreeId, setActiveTreeId] = useState<string | null>(null);
  const [selectedVersionId, setSelectedVersionId] = useState("");
  const [selectedPersonId, setSelectedPersonId] = useState("");
  const [focusedPersonId, setFocusedPersonId] = useState<string | null>(null);
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
  const latestQuery = useQuery({
    queryKey: ["tree", treeId],
    queryFn: () => fetchTree(treeId),
    enabled: Boolean(treeId),
  });
  const latestDetail = latestQuery.data;
  const requestedVersionId = selectedVersionId || latestDetail?.selectedVersion.id || "";
  const versionQuery = useQuery({
    queryKey: ["tree", treeId, "version", requestedVersionId],
    queryFn: () => fetchTreeVersion(treeId, requestedVersionId),
    enabled: Boolean(requestedVersionId && requestedVersionId !== latestDetail?.selectedVersion.id),
  });
  const detail = versionQuery.data ?? latestDetail ?? demoTreeDetail;
  const selectedVersion = detail.selectedVersion;
  const graphNodes = useMemo(() => toGraphNodes(detail), [detail]);
  const statusRelationships = useMemo(() => filterUnresolvedRelationships(detail.relationships, showUnresolved), [detail.relationships, showUnresolved]);
  const lineageFocus = useMemo(() => focusedPersonId ? focusLineage(detail.nodes, statusRelationships, focusedPersonId) : null, [detail.nodes, focusedPersonId, statusRelationships]);
  const visibleGraphNodes = lineageFocus ? graphNodes.filter((node) => lineageFocus.nodeIds.has(node.id)) : graphNodes;
  const visibleRelationships = lineageFocus ? statusRelationships.filter((relationship) => lineageFocus.relationshipIds.has(relationship.id)) : statusRelationships;
  const selected = visibleGraphNodes.find((node) => node.personId === selectedPersonId) ?? visibleGraphNodes[0];
  const versionReady = !versionQuery.isFetching && !versionQuery.isError && (!selectedVersionId || selectedVersion.id === requestedVersionId);
  const canEdit = versionReady && detail.permissions.canEdit && selectedVersion.state === "draft";
  const canPublish = versionReady && detail.permissions.canPublish && selectedVersion.state === "draft";
  const queryError = treesQuery.error ?? latestQuery.error ?? versionQuery.error;

  useEffect(() => {
    if (selectedPersonId && !detail.nodes.some((node) => node.personId === selectedPersonId)) {
      setSelectedPersonId("");
    }
    if (focusedPersonId && !detail.nodes.some((node) => node.personId === focusedPersonId)) {
      setFocusedPersonId(null);
    }
  }, [detail.nodes, focusedPersonId, selectedPersonId]);

  const updateDetail = async (updated: TreeDetail) => {
    queryClient.setQueryData(["tree", updated.tree.id], updated);
    queryClient.setQueryData(["tree", updated.tree.id, "version", updated.selectedVersion.id], updated);
    await queryClient.invalidateQueries({ queryKey: ["trees"] });
  };

  const createMutation = useMutation({
    mutationFn: createTree,
    onSuccess: async (created) => {
      setActiveTreeId(created.tree.id);
      setSelectedVersionId(created.selectedVersion.id);
      setSelectedPersonId("");
      setFocusedPersonId(null);
      queryClient.setQueryData(["tree", created.tree.id], created);
      queryClient.setQueryData(["tree", created.tree.id, "version", created.selectedVersion.id], created);
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
      setSelectedVersionId(updated.selectedVersion.id);
      setSelectedPersonId(updated.nodes[updated.nodes.length - 1]?.personId ?? "");
      setFocusedPersonId(null);
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
      setSelectedVersionId(updated.selectedVersion.id);
      setParentNodeId("");
      setChildNodeId("");
      setMessage("أُضيفت العلاقة إلى تفسير المسودة.");
    },
    onError: (error) => setMessage(authMessage(error)),
  });

  const publishMutation = useMutation({
    mutationFn: () => publishTree(detail.tree.id, "نشر نسخة جديدة من التفسير"),
    onSuccess: async (published) => {
      await updateDetail(published);
      setSelectedVersionId(published.selectedVersion.id);
      setMessage(`نُشرت النسخة ${published.selectedVersion.number}، وفُتحت مسودة جديدة للتعديل.`);
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

  const selectTree = (nextTreeId: string) => {
    setActiveTreeId(nextTreeId);
    setSelectedVersionId("");
    setSelectedPersonId("");
    setFocusedPersonId(null);
    setMessage("");
  };

  const selectVersion = (versionId: string) => {
    setSelectedVersionId(versionId);
    setEditOpen(false);
    setMessage("");
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
        <label className="tree-selector-label">الملف الحالي<select className="tree-selector" aria-label="اختيار الشجرة" value={treeId} onChange={(event) => selectTree(event.target.value)}>
          {availableTrees.length > 0 ? availableTrees.map((tree) => <option key={tree.id} value={tree.id}>{tree.name} · {tree.latestState === "published" ? "منشورة" : "مسودة"}</option>) : <option value={treeId}>{detail.tree.name}</option>}
        </select></label>
        <div className="toolbar-button-group">
          <button className="secondary-button" type="button" onClick={() => setCreateOpen((open) => !open)}><Plus size={15} /> شجرة جديدة</button>
          <button className="secondary-button" type="button" onClick={() => setEditOpen((open) => !open)} disabled={!canEdit}><UserPlus size={15} /> تحرير المسودة</button>
          <button className="secondary-button" type="button"><Share2 size={15} /> مشاركة</button>
          <button className="secondary-button" type="button"><GitCompareArrows size={15} /> مقارنة النسخ</button>
          <button className="primary-button" type="button" onClick={() => publishMutation.mutate()} disabled={!canPublish || publishMutation.isPending}>
            {publishMutation.isPending ? <LoaderCircle className="spin" size={15} /> : <Send size={15} />} نشر المسودة
          </button>
        </div>
      </div>

      {queryError ? <div className="tree-action-message tree-action-error" role="alert">{authMessage(queryError)}</div> : null}
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
          {!canEdit ? <p className="tree-edit-note">هذه نسخة منشورة أو تجريبية. لا يتم تفعيل التحرير إلا على مسودة صاحب الشجرة الحالية.</p> : (
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
              <div className="tree-status-line"><StatusBadge tone={selectedVersion.state === "published" ? "interpretation" : "claim"}>{selectedVersion.state === "published" ? "منشورة" : "مسودة"}</StatusBadge><span>النسخة {selectedVersion.number} من {detail.tree.latestVersionNumber}</span>{versionQuery.isFetching ? <span className="version-loading">جارٍ فتح النسخة…</span> : null}</div>
              <h2>الشجرة كما يعرضها هذا التفسير</h2>
              <p>هذه العلاقات تشرح هذا العرض، ولا تمثل حقيقة تاريخية نهائية.</p>
            </div>
            <button className="icon-button" type="button" aria-label="فلترة الشجرة"><SlidersHorizontal size={17} /></button>
          </div>
          <div className="tree-filter-row">
            <button className={`filter-chip${showUnresolved ? " filter-chip-active" : ""}`} type="button" onClick={() => setShowUnresolved((visible) => !visible)}>
              <span className="filter-chip-dot filter-dot-disputed" /> إظهار غير المحسوم
            </button>
            {focusedPersonId ? <button className="filter-chip filter-chip-active" type="button" onClick={() => setFocusedPersonId(null)}><Maximize2 size={13} /> عرض السلالة كاملة</button> : null}
            <span className="filter-count">{detail.tree.people} أشخاص · {detail.tree.relationships} علاقات</span>
          </div>
          <ResearchGraph selected={selected?.id ?? ""} onSelect={(node) => setSelectedPersonId(node.personId)} nodes={visibleGraphNodes} relationships={visibleRelationships} label="رسم الشجرة الرقمية" />
          <div className="tree-canvas-footer"><LockKeyhole size={13} /> {selectedVersion.state === "draft" ? "هذه مسودة قابلة للتعديل، ولا تمثل تفسيراً منشوراً." : selectedVersion.id === detail.tree.latestVersionId ? "النسخة المنشورة ثابتة. التعديلات الجديدة تنشئ نسخة مسودة." : "هذه نسخة تاريخية للقراءة فقط، وليست تفسيراً منشوراً حالياً."}</div>
        </section>

        <aside className="node-detail-panel">
          {selected ? <>
            <div className="node-detail-top">
              <div className="node-detail-avatar">{selected.name.slice(0, 1)}</div>
              <div><div className="eyebrow">شخص محدد</div><h2>{selected.name}</h2><p>{selected.years} · {selected.role}</p></div>
            </div>
            <div className="node-detail-alert"><StatusBadge tone={selected.tone}>{selected.tone === "disputed" ? "في مركز سؤال" : "ضمن التفسير"}</StatusBadge><span>{selected.sourceCount} إشارات مرتبطة</span></div>
            <div className="node-detail-actions"><button className="secondary-button" type="button" onClick={() => setFocusedPersonId(selected.personId)}><Focus size={14} /> ركّز السلالة</button>{focusedPersonId ? <button className="secondary-button" type="button" onClick={() => setFocusedPersonId(null)}><Maximize2 size={14} /> عرض الكل</button> : null}</div>
            <div className="detail-block"><div className="detail-label">ملاحظة الباحث</div><p>{selected.note}</p></div>
            <div className="detail-block"><div className="detail-label">ما تمثله هذه الشجرة</div><p>تضع هذه النسخة {selected.name} داخل تفسيرها، مع إبقاء الخلافات طبقة كما هي.</p></div>
            <div className="detail-block"><div className="detail-label">المصادر القريبة</div><EvidenceMiniList sourceCount={selected.sourceCount} /></div>
            <Link to="/research" className="detail-cta">افتح هذا الشخص في مكتب البحث <ArrowLeft size={15} /></Link>
          </> : <div className="node-empty-state"><h2>لا يوجد شخص بعد</h2><p>احفظ مسودة، ثم أضف أول شخص لتبدأ الشجرة.</p></div>}
        </aside>
      </div>

      <section className="version-strip">
        <div><div className="eyebrow">سجل النسخ</div><h2>اختر نسخة الشجرة</h2><p>النسخة الحالية قد تكون منشورة، بينما تبقى المسودة الحالية قابلة للتعديل.</p></div>
        <div>
          <div className="version-timeline" role="list" aria-label="نسخ الشجرة">
            {detail.versions.map((version) => {
              const current = version.id === selectedVersion.id;
              const latest = version.id === detail.tree.latestVersionId;
              return <button type="button" className={`version-item${current ? " version-item-current" : ""}`} key={version.id} onClick={() => selectVersion(version.id)} aria-current={current ? "true" : undefined} role="listitem">
                <span>v{version.number}</span><div><strong>{version.state === "draft" ? "مسودة حالية" : current ? "نسخة معاينة" : latest ? "النشر الحالي" : "نسخة منشورة"}</strong><small>{version.publicationNote || "دون ملاحظة نشر"}</small></div>
              </button>;
            })}
          </div>
          <div className="version-readonly-note"><History size={13} /> {selectedVersion.state === "draft" ? "المسودة الحالية قابلة للتحرير من صاحبها." : "هذه النسخة للقراءة فقط."}</div>
        </div>
      </section>
    </div>
  );
}

function toGraphNodes(detail: TreeDetail): TreeNode[] {
  return detail.nodes.map((node, index) => ({
    id: node.id,
    personId: node.personId,
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
