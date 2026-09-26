import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, Focus, GitCompareArrows, GitFork, History, Link2, LoaderCircle, LockKeyhole, Maximize2, Plus, Send, Share2, SlidersHorizontal, Tag, Trash2, UserPlus } from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { addPerson, addPersonAlias, addRelationship, ApiError, createTree, deletePersonAlias, demoTreeDetail, fetchPublicTrees, fetchTree, fetchTreeVersion, listPersonAliases, publishTree, updateRelationship } from "../lib/api";
import { filterUnresolvedRelationships, focusLineage } from "../lib/tree-view";
import { EvidenceMiniList, ResearchGraph } from "../components/ResearchGraph";
import { CollaborationPanel } from "../components/CollaborationPanel";
import { ForkDiffPanel } from "../components/ForkDiffPanel";
import { StatusBadge } from "../components/StatusBadge";
import { SuggestionPanel } from "../components/SuggestionPanel";
import { TopBar } from "../components/TopBar";
import type { AddPersonInput, AddRelationshipInput, PersonAlias, PersonAliasInput, PersonAliasType, RelationshipStatus, TreeDetail, TreeNode, TreeVersionRecord, UpdateRelationshipInput } from "../types";

export type TreePageProps = {
  routeTreeId?: string;
  routeVersionId?: string;
};

export function TreePage({ routeTreeId, routeVersionId }: TreePageProps = {}) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [selectedPersonId, setSelectedPersonId] = useState("");
  const [focusedPersonId, setFocusedPersonId] = useState<string | null>(null);
  const [showUnresolved, setShowUnresolved] = useState(true);
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [firstPerson, setFirstPerson] = useState("");
  const [visibility, setVisibility] = useState<"private" | "unlisted" | "public">("private");
  const [editOpen, setEditOpen] = useState(false);
  const [collaborationOpen, setCollaborationOpen] = useState(false);
  const [forkOpen, setForkOpen] = useState(false);
  const [diffOpen, setDiffOpen] = useState(false);
  const [personName, setPersonName] = useState("");
  const [personGender, setPersonGender] = useState<"male" | "female" | "unknown">("unknown");
  const [birthDateFrom, setBirthDateFrom] = useState("");
  const [birthDateTo, setBirthDateTo] = useState("");
  const [deathDateFrom, setDeathDateFrom] = useState("");
  const [deathDateTo, setDeathDateTo] = useState("");
  const [parentNodeId, setParentNodeId] = useState("");
  const [childNodeId, setChildNodeId] = useState("");
  const [relationshipStatus, setRelationshipStatus] = useState<RelationshipStatus>("interpreted");
  const [relationshipEdits, setRelationshipEdits] = useState<Record<string, { status: RelationshipStatus; reason: string }>>({});
  const [aliasPersonId, setAliasPersonId] = useState("");
  const [aliasValue, setAliasValue] = useState("");
  const [aliasType, setAliasType] = useState<PersonAliasType>("alternative_name");
  const [aliasReason, setAliasReason] = useState("");
  const [message, setMessage] = useState("");

  const treesQuery = useQuery({ queryKey: ["trees"], queryFn: fetchPublicTrees });
  const availableTrees = treesQuery.data ?? [];
  const treeId = routeTreeId ?? availableTrees[0]?.id ?? "";
  const latestQuery = useQuery({
    queryKey: ["tree", treeId],
    queryFn: () => fetchTree(treeId),
    enabled: Boolean(treeId && !routeVersionId),
  });
  const latestDetail = latestQuery.data;
  const versionQuery = useQuery({
    queryKey: ["tree", treeId, "version", routeVersionId ?? ""],
    queryFn: () => fetchTreeVersion(treeId, routeVersionId ?? ""),
    enabled: Boolean(routeTreeId && routeVersionId),
  });
  const loadedDetail = routeVersionId ? versionQuery.data : latestDetail;
  const detail = loadedDetail ?? demoTreeDetail;
  const resourcePending = routeTreeId
    ? routeVersionId
      ? versionQuery.isPending
      : latestQuery.isPending
    : treesQuery.isPending || (Boolean(treeId) && latestQuery.isPending);
  const resourceError = routeTreeId
    ? routeVersionId
      ? versionQuery.error
      : latestQuery.error
    : treesQuery.error ?? latestQuery.error;
  const emptyResource = !routeTreeId && !resourcePending && !resourceError && !treeId;
  const selectedVersion = detail.selectedVersion;
  const graphNodes = useMemo(() => toGraphNodes(detail), [detail]);
  const statusRelationships = useMemo(() => filterUnresolvedRelationships(detail.relationships, showUnresolved), [detail.relationships, showUnresolved]);
  const lineageFocus = useMemo(() => focusedPersonId ? focusLineage(detail.nodes, statusRelationships, focusedPersonId) : null, [detail.nodes, focusedPersonId, statusRelationships]);
  const visibleGraphNodes = lineageFocus ? graphNodes.filter((node) => lineageFocus.nodeIds.has(node.id)) : graphNodes;
  const visibleRelationships = lineageFocus ? statusRelationships.filter((relationship) => lineageFocus.relationshipIds.has(relationship.id)) : statusRelationships;
  const selected = visibleGraphNodes.find((node) => node.personId === selectedPersonId) ?? visibleGraphNodes[0];
  const selectedRelationships = selected ? detail.relationships.filter((relationship) => relationship.subjectNodeId === selected.id || relationship.objectNodeId === selected.id) : [];
  const versionReady = !resourcePending && !resourceError && loadedDetail !== undefined;
  const currentDraft = selectedVersion.state === "draft" && selectedVersion.id === detail.tree.latestVersionId;
  const canEdit = versionReady && currentDraft && detail.permissions.canEdit;
  const canPublish = versionReady && currentDraft && detail.permissions.canPublish;
  const canFork = versionReady && selectedVersion.state === "published" && detail.tree.id !== "tree-demo";
  const canCompareWithUpstream = versionReady && Boolean(detail.tree.parentTreeId && detail.tree.parentVersionId);
  // The aliases of the selected person, read through the identity API. The query
  // is enabled only when there is a person and only inside the draft editor: an
  // alias is a global identity record, and the draft panel is where this workspace
  // already does its recording.
  const aliasPerson = detail.nodes.find((node) => node.personId === aliasPersonId) ?? detail.nodes[0];
  const aliasPersonKey = aliasPerson?.personId ?? "";
  const aliasesQuery = useQuery({ queryKey: ["person-aliases", aliasPersonKey], queryFn: () => listPersonAliases(aliasPersonKey), enabled: Boolean(aliasPersonKey && editOpen && canEdit) });
  const personAliasesQuery = useQuery({ queryKey: ["person-aliases", selected?.personId ?? ""], queryFn: () => listPersonAliases(selected?.personId ?? ""), enabled: Boolean(selected && canEdit) });
  const queryError = resourceError;
  const showResource = !resourcePending && !resourceError && !emptyResource && loadedDetail !== undefined;

  useEffect(() => {
    if (selectedPersonId && !detail.nodes.some((node) => node.personId === selectedPersonId)) {
      setSelectedPersonId("");
    }
    if (focusedPersonId && !detail.nodes.some((node) => node.personId === focusedPersonId)) {
      setFocusedPersonId(null);
    }
  }, [detail.nodes, focusedPersonId, selectedPersonId]);

  useEffect(() => {
    setRelationshipEdits({});
  }, [selectedVersion.id, treeId]);

  const navigateToTreeVersion = (nextTreeId: string, nextVersionId: string) => {
    void navigate({ to: "/tree/$treeId/versions/$versionId", params: { treeId: nextTreeId, versionId: nextVersionId } });
  };

  const updateDetail = async (updated: TreeDetail) => {
    queryClient.setQueryData(["tree", updated.tree.id], updated);
    queryClient.setQueryData(["tree", updated.tree.id, "version", updated.selectedVersion.id], updated);
    await queryClient.invalidateQueries({ queryKey: ["trees"] });
  };

  const handleForked = (forked: TreeDetail) => {
    void updateDetail(forked).then(() => {
      navigateToTreeVersion(forked.tree.id, forked.selectedVersion.id);
      setForkOpen(false);
      setDiffOpen(false);
      setMessage("أُنشئ التفريع كمسودة مستقلة، وبقي الأصل دون تغيير.");
    });
  };

  const createMutation = useMutation({
    mutationFn: createTree,
    onSuccess: async (created) => {
      navigateToTreeVersion(created.tree.id, created.selectedVersion.id);
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
      navigateToTreeVersion(updated.tree.id, updated.selectedVersion.id);
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
      navigateToTreeVersion(updated.tree.id, updated.selectedVersion.id);
      setParentNodeId("");
      setChildNodeId("");
      setMessage("أُضيفت العلاقة إلى تفسير المسودة.");
    },
    onError: (error) => setMessage(authMessage(error)),
  });

  const updateRelationshipMutation = useMutation({
    mutationFn: ({ relationshipId, input }: { relationshipId: string; input: UpdateRelationshipInput }) => updateRelationship(detail.tree.id, relationshipId, input),
    onSuccess: async (updated) => {
      await updateDetail(updated);
      navigateToTreeVersion(updated.tree.id, updated.selectedVersion.id);
      setRelationshipEdits({});
      setMessage("حُدّثت حالة العلاقة داخل تفسير المسودة.");
    },
    onError: (error) => setMessage(authMessage(error)),
  });

  const addAliasMutation = useMutation({
    mutationFn: (input: { personId: string; alias: PersonAliasInput }) => addPersonAlias(input.personId, input.alias),
    onSuccess: async (_created, input) => {
      // The alias is a person record rather than a tree record, so the tree detail
      // does not change; the alias list and the dictionary entry that counts and
      // shows the names are what has to be re-read.
      await queryClient.invalidateQueries({ queryKey: ["person-aliases", input.personId] });
      await queryClient.invalidateQueries({ queryKey: ["dictionary"] });
      setAliasValue("");
      setAliasReason("");
      setMessage("سُجّل اللقب على سجل الهوية، خارج تفسير هذه الشجرة.");
    },
    onError: (error) => setMessage(authMessage(error)),
  });

  const deleteAliasMutation = useMutation({
    mutationFn: (input: { aliasId: string; personId: string; reasonAr: string }) => deletePersonAlias(input.aliasId, input.reasonAr),
    onSuccess: async (_deleted, input) => {
      await queryClient.invalidateQueries({ queryKey: ["person-aliases", input.personId] });
      await queryClient.invalidateQueries({ queryKey: ["dictionary"] });
      setMessage("حُذف اللقب من سجل الهوية.");
    },
    onError: (error) => setMessage(authMessage(error)),
  });

  const publishMutation = useMutation({
    mutationFn: () => publishTree(detail.tree.id, "نشر نسخة جديدة من التفسير"),
    onSuccess: async (published) => {
      // Publishing seals the latest draft and opens a new one, so the version the
      // response selects is the draft that was just opened, not the version that
      // was published. The number is read from the sealed version the response
      // itself carries, and falls back to the draft this mutation started from -
      // which is the draft the API seals - rather than to a guessed number.
      const publishedNumber = newestPublishedVersion(published)?.number ?? selectedVersion.number;
      await updateDetail(published);
      navigateToTreeVersion(published.tree.id, published.selectedVersion.id);
      setMessage(`نُشرت النسخة ${publishedNumber}، وفُتحت مسودة جديدة للتعديل.`);
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

  const submitRelationshipStatus = (event: FormEvent<HTMLFormElement>, relationshipId: string, fallbackStatus: RelationshipStatus) => {
    event.preventDefault();
    const edit = relationshipEdits[relationshipId] ?? { status: fallbackStatus, reason: "" };
    if (!edit.reason.trim()) {
      setMessage("اكتب سبب تغيير حالة العلاقة قبل الحفظ.");
      return;
    }
    setMessage("");
    updateRelationshipMutation.mutate({
      relationshipId,
      input: { status: edit.status, expected_version_id: selectedVersion.id, reason_ar: edit.reason },
    });
  };

  const submitAlias = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!aliasPersonKey) {
      setMessage("أضف شخصاً إلى المسودة أولاً حتى تسجل له لقباً.");
      return;
    }
    setMessage("");
    addAliasMutation.mutate({ personId: aliasPersonKey, alias: { value_ar: aliasValue, alias_type: aliasType, reason_ar: aliasReason || undefined } });
  };

  const selectTree = (nextTreeId: string) => {
    setSelectedPersonId("");
    setFocusedPersonId(null);
    setMessage("");
    void navigate({ to: "/tree/$treeId", params: { treeId: nextTreeId } });
  };

  return (
    <div className="page-stack">
      <TopBar
        eyebrow="شجرة بحث / تفسير محفوظ"
        title={loadedDetail?.tree.name ?? (resourcePending ? "جارٍ فتح الشجرة" : "شجرة بحث")}
        description={loadedDetail?.tree.description || "افتح علاقة لترى من أين جاءت، وما الذي ما زال غير محسوم."}
      />
      <div className="tree-page-toolbar">
        <div className="toolbar-breadcrumb"><span>الأشجار</span><b>/</b><strong>{loadedDetail?.tree.name ?? (resourcePending ? "جارٍ الفتح" : "شجرة بحث")}</strong></div>
        <label className="tree-selector-label">الملف الحالي<select className="tree-selector" aria-label="اختيار الشجرة" value={treeId} onChange={(event) => selectTree(event.target.value)}>
          {availableTrees.length > 0 ? availableTrees.map((tree) => <option key={tree.id} value={tree.id}>{tree.name} · {tree.latestState === "published" ? "منشورة" : "مسودة"}</option>) : <option value={treeId}>{loadedDetail?.tree.name ?? "شجرة بحث"}</option>}
        </select></label>
        <div className="toolbar-button-group">
          <button className="secondary-button" type="button" onClick={() => setCreateOpen((open) => !open)}><Plus size={15} /> شجرة جديدة</button>
          <button className="secondary-button" type="button" onClick={() => setEditOpen((open) => !open)} disabled={!canEdit}><UserPlus size={15} /> تحرير المسودة</button>
          <button className="secondary-button" type="button" onClick={() => setCollaborationOpen((open) => !open)} disabled={detail.tree.id === "tree-demo"}><Share2 size={15} /> مشاركة</button>
          <button className="secondary-button" type="button" onClick={() => { setForkOpen((open) => !open); setDiffOpen(false); }} disabled={!canFork}><GitFork size={15} /> تفريع</button>
          <button className="secondary-button" type="button" onClick={() => { setDiffOpen((open) => !open); setForkOpen(false); }} disabled={!canCompareWithUpstream}><GitCompareArrows size={15} /> مقارنة بالأصل</button>
          <button className="primary-button" type="button" onClick={() => publishMutation.mutate()} disabled={!canPublish || publishMutation.isPending}>
            {publishMutation.isPending ? <LoaderCircle className="spin" size={15} /> : <Send size={15} />} نشر المسودة
          </button>
        </div>
      </div>

      {queryError ? <div className="tree-action-message tree-action-error" role="alert">{authMessage(queryError)}</div> : null}
      {message ? <div className="tree-action-message" role="status">{message}{message.includes("تسجيل الدخول") ? <Link to="/login">فتح تسجيل الدخول</Link> : null}</div> : null}

      {showResource ? <>
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

      {collaborationOpen ? <CollaborationPanel key={detail.tree.id} treeId={detail.tree.id} canManage={detail.permissions.canManageCollaborators} permissionLevel={detail.permissions.permissionLevel} /> : null}
      {forkOpen ? <ForkDiffPanel key={`fork-${detail.tree.id}-${selectedVersion.id}`} detail={detail} mode="fork" onForked={handleForked} onClose={() => setForkOpen(false)} /> : null}
      {diffOpen ? <ForkDiffPanel key={`diff-${detail.tree.id}-${selectedVersion.id}`} detail={detail} mode="diff" onForked={handleForked} onClose={() => setDiffOpen(false)} /> : null}
      {selected && selectedVersion.state === "published" && detail.tree.visibility === "public" ? <SuggestionPanel key={`${detail.tree.id}-${selectedVersion.id}-${selected.id}`} treeId={detail.tree.id} versionId={selectedVersion.id} versionNumber={selectedVersion.number} nodeId={selected.id} nodeName={selected.name} canReview={Boolean(detail.permissions.permissionLevel)} /> : null}

      {editOpen ? (
        <section className="tree-edit-panel">
          <div className="tree-edit-head">
            <div><div className="eyebrow">تعديل تفسيري</div><h2>حرّر المسودة الحالية</h2><p>كل إضافة تُحفظ داخل نسخة الشجرة، ولا تتحول تلقائياً إلى حقيقة تاريخية.</p></div>
            <StatusBadge tone="claim">نسخة محفوظة</StatusBadge>
          </div>
          {!canEdit ? <p className="tree-edit-note">هذه نسخة منشورة أو تجريبية. التحرير متاح على المسودة الحالية لصاحب الشجرة أو لمتعاون بصلاحية تحرير.</p> : (
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

              <form className="tree-edit-form" onSubmit={submitAlias}>
                <div className="tree-edit-form-head"><div><h3>تسجيل لقب أو اسم آخر</h3><p>اللقب يُحفظ على سجل الهوية نفسه، لا على تفسير هذه الشجرة، ويظهر في صفحة القاموس.</p></div><Tag size={17} /></div>
                {detail.nodes.length < 1 ? <p className="tree-edit-note">أضف شخصاً أولاً حتى تسجل له لقباً.</p> : <div className="tree-edit-fields">
                  <label className="composer-label">الشخص<select value={aliasPersonKey} onChange={(event) => setAliasPersonId(event.target.value)}>{detail.nodes.map((node) => <option key={node.id} value={node.personId}>{node.displayName}</option>)}</select></label>
                  <label className="composer-label">اللقب بالعربية<input required value={aliasValue} onChange={(event) => setAliasValue(event.target.value)} placeholder="مثال: أبو بكر" /></label>
                  <label className="composer-label">نوع اللقب<select value={aliasType} onChange={(event) => setAliasType(event.target.value as PersonAliasType)}><option value="alternative_name">اسم آخر</option><option value="kunyah">كنية</option><option value="laqab">لقب</option><option value="nisbah">نسبة</option><option value="source_spelling">رسم من مصدر</option></select></label>
                  <label className="composer-label">سبب التسجيل<input value={aliasReason} onChange={(event) => setAliasReason(event.target.value)} placeholder="مثال: ورد في السجل المختلط" /></label>
                </div>}
                {aliasesQuery.isError ? <p className="tree-edit-note">تعذر قراءة الألقاب المسجلة لهذا الشخص.</p> : null}
                <AliasChipList
                  aliases={aliasPersonKey === selected?.personId ? personAliasesQuery.data ?? [] : aliasesQuery.data ?? []}
                  onDelete={(alias) => deleteAliasMutation.mutate({ aliasId: alias.id, personId: aliasPersonKey, reasonAr: aliasReason || "حذف لقب" })}
                  disabled={deleteAliasMutation.isPending}
                />
                <div className="tree-edit-actions"><button className="primary-button" type="submit" disabled={addAliasMutation.isPending || detail.nodes.length < 1}>{addAliasMutation.isPending ? "جارٍ الحفظ…" : "سجّل اللقب"}<Tag size={15} /></button></div>
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
            <div className="relationship-list">
              <div className="detail-label">العلاقات في هذه النسخة</div>
              <p className="relationship-scope-note">تغيير الحالة يخص تفسير هذه الشجرة، ولا يحسم ادعاءً أو مصدراً عالمياً.</p>
              {selectedRelationships.length > 0 ? selectedRelationships.map((relationship) => {
                const relationshipEdit = relationshipEdits[relationship.id] ?? { status: relationship.status, reason: "" };
                const subject = detail.nodes.find((node) => node.id === relationship.subjectNodeId)?.displayName ?? "—";
                const object = detail.nodes.find((node) => node.id === relationship.objectNodeId)?.displayName ?? "—";
                return <form className="relationship-item" key={relationship.id} onSubmit={(event) => submitRelationshipStatus(event, relationship.id, relationship.status)}>
                  <div className="relationship-item-head"><div><strong>{subject} <ArrowLeft size={11} /> {object}</strong><small>{relationshipPredicateLabel(relationship.predicate)}</small></div><StatusBadge tone={relationship.status === "disputed" ? "disputed" : relationship.status === "unresolved" ? "question" : "interpretation"}>{relationshipStatusLabel(relationship.status)}</StatusBadge></div>
                  {canEdit ? <div className="relationship-editor"><select aria-label="حالة العلاقة" value={relationshipEdit.status} onChange={(event) => setRelationshipEdits((current) => ({ ...current, [relationship.id]: { status: event.target.value as RelationshipStatus, reason: current[relationship.id]?.reason ?? "" } }))}><option value="interpreted">مفسر</option><option value="disputed">متنازع عليه</option><option value="unresolved">غير محسوم</option></select><input required aria-label="سبب تغيير الحالة" value={relationshipEdit.reason} onChange={(event) => setRelationshipEdits((current) => ({ ...current, [relationship.id]: { status: current[relationship.id]?.status ?? relationship.status, reason: event.target.value } }))} placeholder="سبب تغيير التفسير" /><button className="primary-button" type="submit" disabled={updateRelationshipMutation.isPending}>{updateRelationshipMutation.isPending ? "جارٍ الحفظ…" : "احفظ الحالة"}</button></div> : <small className="relationship-readonly">النسخة الحالية للقراءة فقط؛ لا يمكن تغيير حالة العلاقة هنا.</small>}
                </form>;
              }) : <p className="relationship-empty">لا توجد علاقات مرتبطة بهذا الشخص في النسخة المختارة.</p>}
            </div>
            {canEdit ? <div className="detail-block"><div className="detail-label">الألقاب المسجلة على الهوية</div><AliasChipList
              aliases={personAliasesQuery.data ?? []}
              onDelete={(alias) => deleteAliasMutation.mutate({ aliasId: alias.id, personId: selected.personId, reasonAr: aliasReason || "حذف لقب" })}
              disabled={deleteAliasMutation.isPending}
            /></div> : null}
            <div className="detail-block"><div className="detail-label">ملاحظة الباحث</div><p>{selected.note}</p></div>
            <div className="detail-block"><div className="detail-label">ما تمثله هذه الشجرة</div><p>تضع هذه النسخة {selected.name} داخل تفسيرها، مع إبقاء الخلافات طبقة كما هي.</p></div>
            <div className="detail-block"><div className="detail-label">المصادر القريبة</div><EvidenceMiniList sourceCount={selected.sourceCount} /></div>
            <Link to="/research" search={{ entityType: "person", entityId: selected.personId, treeId, treeVersionId: selectedVersion.id }} className="detail-cta">افتح هذا الشخص في مكتب البحث <ArrowLeft size={15} /></Link>
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
              return <Link to="/tree/$treeId/versions/$versionId" params={{ treeId, versionId: version.id }} className={`version-item${current ? " version-item-current" : ""}`} key={version.id} onClick={() => { setEditOpen(false); setMessage(""); }} aria-current={current ? "true" : undefined} role="listitem">
                <span>v{version.number}</span><div><strong>{version.state === "draft" ? "مسودة حالية" : current ? "نسخة معاينة" : latest ? "النشر الحالي" : "نسخة منشورة"}</strong><small>{version.publicationNote || "دون ملاحظة نشر"}</small></div>
              </Link>;
            })}
          </div>
          <div className="version-readonly-note"><History size={13} /> {selectedVersion.state === "draft" ? "المسودة الحالية قابلة للتحرير من صاحبها." : "هذه النسخة للقراءة فقط."}</div>
          {detail.tree.parentTreeId && detail.tree.parentVersionId ? <div className="version-upstream-note"><GitCompareArrows size={13} /> هذا التفريع مستقل عن نسخة الأصل، ويمكنك فتح المقارنة الدلالية.</div> : null}
        </div>
      </section>
      </> : (
        <div className="tree-load-state" role={resourceError ? "alert" : "status"}>
          <strong>{resourcePending ? "جارٍ فتح الشجرة…" : resourceError ? "تعذر فتح هذه الشجرة" : "لا توجد أشجار متاحة"}</strong>
          <span>{resourcePending ? "يتم الآن تحميل النسخة المطلوبة من العنوان." : resourceError ? authMessage(resourceError) : "أنشئ شجرة جديدة أو اطلب من صاحبها مشاركة رابطها."}</span>
          {resourceError instanceof ApiError && resourceError.status === 401 ? <Link to="/login">فتح تسجيل الدخول</Link> : null}
        </div>
      )}
    </div>
  );
}

/**
 * The version a publish response sealed: the newest published version it lists.
 * Publishing always closes the latest draft, so after a publish the highest
 * published number is the version that was just published - which is not the
 * version the response selects, because that is the draft it opened.
 */
function newestPublishedVersion(detail: TreeDetail): TreeVersionRecord | null {
  return detail.versions.reduce<TreeVersionRecord | null>(
    (newest, version) => (version.state === "published" && (newest === null || version.number > newest.number) ? version : newest),
    null,
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

/**
 * The recorded names of a person, with a delete button each. It is read-only
 * markup on purpose: this workspace has no identity admin surface, and the only
 * identity write it offers is the one a researcher actually reaches for - a name
 * the same person appears under in another record.
 */
function AliasChipList({ aliases, onDelete, disabled }: { aliases: PersonAlias[]; onDelete: (alias: PersonAlias) => void; disabled: boolean }) {
  if (aliases.length === 0) {
    return <p className="tree-edit-note">لا توجد ألقاب مسجلة على هذا الشخص بعد.</p>;
  }
  return <div className="dictionary-tag-row">{aliases.map((alias) => <span className="dictionary-tag" key={alias.id}>{alias.valueAr}<small>{aliasTypeLabel(alias.aliasType)}</small><button className="icon-button" type="button" aria-label={`حذف اللقب ${alias.valueAr}`} disabled={disabled} onClick={() => onDelete(alias)}><Trash2 size={12} /></button></span>)}</div>;
}

function aliasTypeLabel(aliasType: PersonAliasType): string {
  if (aliasType === "kunyah") return "كنية";
  if (aliasType === "laqab") return "لقب";
  if (aliasType === "nisbah") return "نسبة";
  if (aliasType === "source_spelling") return "رسم من مصدر";
  return "اسم آخر";
}

function relationshipStatusLabel(status: RelationshipStatus): string {
  if (status === "disputed") return "متنازع عليه";
  if (status === "unresolved") return "غير محسوم";
  return "مفسر";
}

function relationshipPredicateLabel(predicate: string): string {
  if (predicate === "spouse_of") return "علاقة زواج";
  if (predicate === "sibling_of") return "علاقة إخوة";
  return "علاقة أبوة";
}

function authMessage(error: unknown): string {
  if (error instanceof ApiError && error.status === 401) {
    return "سجّل الدخول أولًا لإنشاء أو تعديل أو نشر شجرة.";
  }
  if (error instanceof ApiError && error.status === 409) {
    return error.message || "تغيرت نسخة الشجرة. حدّث الصفحة ثم أعد المحاولة.";
  }
  if (error instanceof ApiError && error.status === 403) {
    return "هذا الإجراء على سجل الهوية يحتاج دور باحث أو متعامل أو مشرف.";
  }
  return error instanceof Error ? error.message : "تعذر إكمال العملية.";
}
