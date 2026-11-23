import { AtlasOccurrenceOverlay } from "../components/AtlasSourceSearch";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import {
  ChevronDown,
  ChevronLeft,
  ChevronUp,
  CircleAlert,
  Columns2,
  Crosshair,
  Eye,
  EyeOff,
  Layers3,
  LoaderCircle,
  MapPinned,
  Maximize2,
  Minimize2,
  PanelLeftClose,
  PanelLeftOpen,
  PanelRightClose,
  PanelRightOpen,
  Plus,
  Printer,
  RotateCcw,
  SearchX,
  ShieldQuestion,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { AddPlaceDialog } from "../components/AddPlaceDialog";
import { AtlasFiltersPanel } from "../components/AtlasFilters";
import { fetchAtlasPlateRegister } from "../lib/api";
import { plateLayerSpecs, plateHttpDefaultOpacity, plateSourceBounds, type OttomanHaritaTerritory } from "../lib/plate-atlas";
import { corridorCollection } from "../lib/movement-events";
import { fetchAtlasRelations, fetchSubjectCorridors } from "../lib/api";
import { useDebouncedKeyed, useDebouncedScalar } from "../lib/useDebounced";
import { AtlasInferenceOverlay } from "../components/AtlasInferenceOverlay";
import { useAtlasInference } from "../lib/atlas-inference";
import { AtlasInspector } from "../components/AtlasInspector";
import { AtlasMap } from "../components/AtlasMap";
import { normalizeArabicName } from "../lib/arabicNames";
import { OttomanHaritaPanel, PlateCaptionPanel } from "../components/PlateCaptionPanel";
import { PlateComparePanel } from "../components/PlateComparePanel";
import { TopBar } from "../components/TopBar";
import { fetchMapFeatures, fetchResearchAccess, fetchPersonMovement } from "../lib/api";
import {
  activeFilterCount,
  aggregateFeatures,
  atlasFilterValues,
  defaultAtlasLayerVisibility,
  encodeAtlasFilter,
  featureLabel,
  featurePoint,
  filterFeatures,
  type AtlasLayerVisibility,
  type AtlasMarker,
  type AtlasPlateTribeLabel,
  type AtlasSearch,
  type AtlasViewport,
  type PeriodPreset,
  type AtlasFilters,
  yearBounds,
} from "../lib/atlas";
import {
  formatMapCount,
  mapKindLabel,
  mapKinds,
  mapStatusCopy,
  mapStatuses,
  mapStatusVocabulary,
  mapUndatedRegionCopy,
  type MapKind,
  type MapStatus,
} from "../lib/map-vocabulary";
import type { MapFeature } from "../types";

const ATLAS_LIMIT = 5000;

type AtlasRow = {
  key: string;
  label: string;
  status: MapStatus;
  kind: MapFeature["kind"];
  meta: string;
  detail?: string;
  count: number;
  marker?: AtlasMarker;
  feature?: MapFeature;
};

/**
 * The Atlas.
 *
 * One canvas, one source of features (`GET /api/v1/map`), and one vocabulary.
 * Every marker, line, polygon, count and inspector sentence is derived from the
 * response; the component contributes layout and grammar, never facts. Filters
 * act on data already in memory so a pill can never claim to filter while only
 * changing its own colour, and the URL carries the research position so a map
 * state can be linked, reloaded and revisited.
 */
export function PlacesPage() {
  const search = useSearch({ from: "/places" });
  const navigate = useNavigate();
  const [queryDraft, setQueryDraft] = useState(search.q ?? "");
  const [viewport, setViewport] = useState<AtlasViewport | null>(null);
  // Content-addressed, not identity-addressed: a fresh viewport object arrives
  // only when a move ends, and the key beneath is what the debounce reads.
  const viewportKey = viewport
    ? `${viewport.minLongitude},${viewport.minLatitude},${viewport.maxLongitude},${viewport.maxLatitude}`
    : "none";
  const debouncedViewport = useDebouncedKeyed(viewport, 420, viewportKey);
  const [knownBounds, setKnownBounds] = useState<{ min: number; max: number } | null>(null);
  const [focus, setFocus] = useState<{ longitude: number; latitude: number; key: string } | null>(null);
  const [fitBoundsRequest, setFitBoundsRequest] = useState<{ bounds: [number, number, number, number]; key: string } | null>(null);
  const [resetSignal, setResetSignal] = useState(0);
  const [showAllRows, setShowAllRows] = useState(false);
  const [resultsOpen, setResultsOpen] = useState(true);
  const [resultsTab, setResultsTab] = useState<"all" | "place" | "migration" | "region" | "tribes">("all");
  const [addOpen, setAddOpen] = useState(false);
  const [notice, setNotice] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const [inferenceOpen, setInferenceOpen] = useState(false);
  const [selectedSealKey, setSelectedSealKey] = useState<string | null>(null);
  const [filtersCollapsed, setFiltersCollapsed] = useState(false);
  const [inspectorCollapsed, setInspectorCollapsed] = useState(false);
  const [canvasExpanded, setCanvasExpanded] = useState(false);
  const [legendOpen, setLegendOpen] = useState(true);
  const [layersMenuOpen, setLayersMenuOpen] = useState(false);
  const [layerVisibility, setLayerVisibility] = useState<AtlasLayerVisibility>(defaultAtlasLayerVisibility);
  const [swipePercent, setSwipePercent] = useState<number | null>(null);
  const deepLinkFocused = useRef(false);
  const lastQuery = useRef(search.q ?? "");

  useEffect(() => {
    if (!canvasExpanded) return undefined;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setCanvasExpanded(false);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [canvasExpanded]);

  const access = useQuery({ queryKey: ["research-access"], queryFn: fetchResearchAccess, staleTime: 60000, retry: false });

  // سجل اللوحات (register, plan 032-A): يُقرأ مرة واحدة؛ الصفوف القابلة
  // للعرض هي فقط التي تحمل محوّلاً صالحاً — البقية تبقى في السجل بشرحها.
  const plateRegisterQuery = useQuery({ queryKey: ["atlas-plate-register"], queryFn: () => fetchAtlasPlateRegister(), staleTime: 300000, retry: false });

  const plateRegister = plateRegisterQuery.data ?? null;
  const plateKeys = useMemo(() => (search.plates ?? "").split(",").filter(Boolean), [search.plates]);
  const plateOpacity = search.plateOpacity ?? plateHttpDefaultOpacity;
  const plateLayers = useMemo(() => plateLayerSpecs(plateRegister, plateKeys, plateOpacity), [plateRegister, plateKeys, plateOpacity]);
  const plateView = search.view === "plate";
  const captions = useMemo(() => plateLayers.flatMap((plate) => plate.tribeLabels ?? []), [plateLayers]);
  const selectedCaption = captions.find((caption) => caption.id === search.caption);
  const selectedTerritory = useMemo(() => {
    if (!selectedCaption) return undefined;
    for (const plate of plateLayers) {
      const hit = plate.tribeTerritories?.find(
        (territory) => territory.captionId === selectedCaption.id || (territory.plateKey === selectedCaption.plateKey && territory.nameAr === selectedCaption.nameAr),
      );
      if (hit) return hit;
    }
    return undefined;
  }, [plateLayers, selectedCaption]);
  const captionPlate = plateRegister?.plates.find((plate) => plate.plateKey === selectedCaption?.plateKey);
  const captionNeedle = normalizeArabicName(queryDraft).toLowerCase().replace(/[^\p{L}\p{N}]/gu, "");
  const captionMatches = useMemo(() => (captionNeedle ? captions.filter((label) =>
    [label.nameAr, label.printedName].some((name) =>
      normalizeArabicName(name).toLowerCase().replace(/[^\p{L}\p{N}]/gu, "").includes(captionNeedle))) : []), [captionNeedle, captions]);
  const selectedCaptionID = selectedCaption?.id;
  const captionLongitude = selectedCaption?.coordinates[0];
  const captionLatitude = selectedCaption?.coordinates[1];
  useEffect(() => {
    if (selectedCaptionID && captionLongitude !== undefined && captionLatitude !== undefined) {
      setFocus({ key: selectedCaptionID, longitude: captionLongitude, latitude: captionLatitude });
    }
  }, [selectedCaptionID, captionLongitude, captionLatitude]);
  const [selectedOttomanTerritory, setSelectedOttomanTerritory] = useState<OttomanHaritaTerritory | null>(null);
  const selectCaption = (caption: AtlasPlateTribeLabel) => {
    setSelectedOttomanTerritory(null);
    setInspectorCollapsed(false);
    navigateSearch({
      caption: caption.id, compare: undefined, place: undefined, feature: undefined,
    });
  };
  const selectOttomanTerritory = (territory: OttomanHaritaTerritory) => {
    setSelectedOttomanTerritory(territory);
    setInspectorCollapsed(false);
    navigateSearch({ caption: undefined, compare: undefined, place: undefined, feature: undefined });
  };
  const toggleOttomanHarita = () => {
    setLayerVisibility((current) => {
      const next = !current.ottomanHarita1910;
      if (!next) setSelectedOttomanTerritory(null);
      return { ...current, ottomanHarita1910: next };
    });
  };
  const focusOttomanHarita = () => {
    setLayerVisibility((current) => ({ ...current, ottomanHarita1910: true }));
    setFitBoundsRequest({ bounds: [36.0, 20.5, 51.5, 32.0], key: `ottoman-1910:${Date.now()}` });
  };
  const togglePlateView = () => {
    if (plateView) {
      navigateSearch({ view: undefined });
      return;
    }
    const first = plateRegister?.plates.find((plate) => plate.transform);
    if (first) navigateSearch({ view: "plate", plates: search.plates || first.plateKey });
  };
  const togglePlate = (plateKey: string) => {
    const next = plateKeys.includes(plateKey) ? plateKeys.filter((key) => key !== plateKey) : [...plateKeys, plateKey];
    if (next.length === 0) setSwipePercent(null);
    navigateSearch({ plates: next.length ? next.join(",") : undefined, caption: undefined });
  };
  const focusPlate = (plateKey: string) => {
    const row = plateRegister?.plates.find((plate) => plate.plateKey === plateKey);
    if (!row?.transform) return;
    if (!plateKeys.includes(plateKey)) {
      navigateSearch({ plates: [...plateKeys, plateKey].join(",") });
    }
    const plateBounds = plateSourceBounds(row.transform.transform, row.transform.frame);
    setFitBoundsRequest({ bounds: plateBounds, key: `${plateKey}:${Date.now()}` });
  };
  const toggleLayerVisibility = (key: keyof AtlasLayerVisibility) => {
    setLayerVisibility((current) => ({ ...current, [key]: !current[key] }));
  };
  const toggleSwipe = () => {
    if (plateLayers.length === 0) return;
    setSwipePercent((current) => (current === null ? 50 : null));
  };
  const setPlateOpacity = (value: number) => {
    navigateSearch({ plateOpacity: value === plateHttpDefaultOpacity ? undefined : value });
  };

  // The fetch window is the URL's window, raw: the bounds-clamped version
  // below only ever narrows the inspector's caption, never the server
  // question. The two halves are debounced separately as primitives: a
  // window object would be a fresh identity every render, and the debounce
  // hook keys its effect on the value — `Object.is` would fire on every
  // render for as long as the URL carried a window, re-rendering the page
  // forever. Narrowing the hook to primitives makes that unrepresentable.
  const fetchEraFrom = search.from ?? null;
  const fetchEraTo = search.to ?? null;
  const debouncedEraFrom = useDebouncedScalar(fetchEraFrom, 300);
  const debouncedEraTo = useDebouncedScalar(fetchEraTo, 300);

  // مقارنة النقطة (plan 032-C): URL-first — `compare=arm` waits for a map
  // click, then `compare=lon,lat` holds the point the panel reads. The map
  // itself stays dumb: it reports a click, the URL carries the choice.
  const compareArmed = search.compare === "arm";
  const comparePoint = useMemo(() => {
    if (!search.compare || search.compare === "arm") return null;
    const [lonRaw, latRaw] = search.compare.split(",");
    const longitude = Number(lonRaw);
    const latitude = Number(latRaw);
    if (!Number.isFinite(longitude) || !Number.isFinite(latitude)) return null;
    return { longitude, latitude };
  }, [search.compare]);
  const armCompare = () => navigateSearch({ compare: compareArmed ? undefined : "arm", feature: undefined, place: undefined });
  // The movement's own filters (plan 033-C): one word each, the URL's own.
  const toggleMovementMode = (mode: string) => navigateSearch({ mode: search.mode === mode ? undefined : mode });
  const toggleSeason = (season: string) => navigateSearch({ season: search.season === season ? undefined : season });
  const clearCompare = () => {
    navigateSearch({ compare: undefined });
  };

  // الأروابط (plan 032-E): the era chips' own window scopes the read; the
  // layer is the relationships' RECORD, bounded by the server.
  const relationArcsQuery = useQuery({
    queryKey: ["atlas-relations", debouncedEraFrom ?? 0, debouncedEraTo ?? 0],
    queryFn: () => fetchAtlasRelations(debouncedEraFrom ?? 0, debouncedEraTo ?? 0),
    staleTime: 120000,
    retry: false,
  });
  const relationArcs = useMemo(() => relationArcsQuery.data?.relations ?? [], [relationArcsQuery.data]);

  // The subject layer (plan 028-D): one person's documented مسالِك, fetched
  // only when the URL names a subject. It is converted into the same
  // migration features the canvas already draws — status-colored segments
  // with the shared arrowhead grammar — so no second route renderer exists.
  const subjectID = search.subject;
  // The subject type is the reader's honest emptiness check: person-first, then tribe.

  const movementQuery = useQuery({
    queryKey: ["person-movement", subjectID ?? null],
    enabled: Boolean(subjectID),
    retry: false,
    staleTime: 30000,
    refetchOnWindowFocus: false,
    queryFn: ({ signal }) => fetchPersonMovement(subjectID!, signal),
  });
  // ممرات (plan 033-B): when the subject layer holds a SUBJECT, the corridors
  // ride with it; the season/mode chips (033-C) narrow the read honestly. The
  // subject type follows the movement query's own resolution: its first
  // attempt is the person's, and the corridors read the TRIBE when the person
  // read came back empty of names (the 031 trials' machine: a tribe id is not
  // a person id, and the honest answer is the tribe's own corridors).
  const corridorsQuery = useQuery({
    queryKey: ["subject-corridors", subjectID ?? null, search.season ?? "", search.mode ?? ""],
    queryFn: ({ signal }) => fetchSubjectCorridors("tribe", subjectID ?? "", { season: search.season, mode: search.mode, signal }),
    enabled: Boolean(subjectID) && (!movementQuery.data || movementQuery.data.events.length === 0),
    staleTime: 120000,
    retry: false,
  });
  const corridorsData = useMemo(() => corridorCollection((corridorsQuery.data?.corridors ?? []).map((corridor) => ({
    id: corridor.id,
    titleAr: corridor.titleAr,
    season: corridor.season,
    status: corridor.status,
    certainty: corridor.certainty,
    stops: corridor.stops.map((stop) => ({
      sequence: stop.sequence,
      placeId: stop.placeId,
      placeName: stop.placeName,
      longitude: stop.longitude,
      latitude: stop.latitude,
      dayCountFromPrev: stop.dayCountFromPrev,
      modeFromPrev: stop.modeFromPrev,
    })),
  }))), [corridorsQuery.data]);

  const debouncedEraWindow = debouncedEraFrom !== null || debouncedEraTo !== null
    ? { from: debouncedEraFrom ?? undefined, to: debouncedEraTo ?? undefined }
    : null;

  const mapQuery = useQuery({
    queryKey: ["atlas-map", debouncedViewport?.minLongitude ?? null, debouncedViewport?.minLatitude ?? null, debouncedViewport?.maxLongitude ?? null, debouncedViewport?.maxLatitude ?? null, debouncedEraWindow?.from ?? null, debouncedEraWindow?.to ?? null, search.mode ?? null, search.season ?? null],
    queryFn: ({ signal }) => fetchMapFeatures({
      viewport: debouncedViewport ?? undefined,
      fromYear: debouncedEraWindow?.from,
      toYear: debouncedEraWindow?.to,
      movementMode: search.mode,
      season: search.season,
      limit: ATLAS_LIMIT,
      signal,
    }),
    placeholderData: (previous) => previous,
    staleTime: 30000,
    refetchOnWindowFocus: false,
  });
  const dataFeatures = mapQuery.data?.features;
  const features = useMemo(() => dataFeatures ?? [], [dataFeatures]);
  const inference = useAtlasInference(inferenceOpen && Boolean(access.data?.canReview), features);

  const subjectFeatures = useMemo<MapFeature[]>(() => {
    const response = movementQuery.data;
    if (!response) return [];
    return response.events.flatMap((event) => {
      if (event.fromPlace?.longitude === undefined || event.fromPlace?.latitude === undefined) return [];
      if (event.toPlace?.longitude === undefined || event.toPlace?.latitude === undefined) return [];
      const status = event.status as MapFeature["status"];
      return [{
        id: event.id,
        kind: "migration" as const,
        placeId: event.toPlace.id,
        placeName: event.toPlace.name ?? event.fromPlace.name,
        entityType: "person",
        entityId: subjectID,
        entityName: response.personName,
        relationType: "migration",
        status,
        certainty: event.certainty || "approximate",
        eventKind: event.kind,
        timeFrom: event.timeFrom,
        timeTo: event.timeTo,
        period: event.period,
        sourceId: event.sourceId,
        sourceTitle: event.sourceTitle,
        evidenceText: event.notes,
        fromLongitude: event.fromPlace.longitude,
        fromLatitude: event.fromPlace.latitude,
        toLongitude: event.toPlace.longitude,
        toLatitude: event.toPlace.latitude,
      }];
    });
  }, [movementQuery.data, subjectID]);
  const subjectPersonName = movementQuery.data?.personName;

  const statuses = useMemo(() => atlasFilterValues<MapStatus>(search.status, mapStatuses), [search.status]);
  const kinds = useMemo(() => atlasFilterValues<MapKind>(search.kind, mapKinds), [search.kind]);

  // A window can arrive before the first response, and a viewport response can
  // hold a narrower set of years than the previous one. The scrubber therefore
  // grows over every response this page has seen, never shrinks under the
  // reader's hands, and falls back to "no year filter" when nothing is dated.
  useEffect(() => {
    const next = yearBounds(features);
    if (!next) return;
    setKnownBounds((previous) => previous
      ? { min: Math.min(previous.min, next.min), max: Math.max(previous.max, next.max) }
      : next);
  }, [features]);

  const bounds = knownBounds;
  // The window the reader chose, raw from the URL. It is deliberately not
  // clamped to the scrubber's bounds: an era window outside the dated
  // evidence (demo evidence is 1180–1210; the first Saudi state is
  // 1727–1818) is still the question the server was asked, and clamping it
  // to the bounds would answer a different question while the chip claims
  // the named one. The scrubber clamps its own thumb on change; the readout
  // shows the window as chosen, which is what makes the chip stay lit.
  const fromYear = search.from ?? bounds?.min ?? 0;
  const toYear = search.to ?? bounds?.max ?? 0;

  // A window the reader chose (a chip, a preset, a drag) is the era question the
  // server answers with the region counters. Without it the fetch stays
  // windowless: counts computed against a guessed window would read as "in the
  // era" while no era was asked for. The fetch debounce (`debouncedEraWindow`
  // above) already carries this same raw window, so the inspector caption and
  // the server counters can never claim different windows.
  const eraWindowRequested = search.from !== undefined || search.to !== undefined;
  const eraWindow = eraWindowRequested ? { from: fromYear, to: toYear } : null;

  const filters = useMemo<AtlasFilters>(() => ({
    statuses,
    kinds,
    fromYear,
    toYear,
    query: queryDraft,
  }), [statuses, kinds, fromYear, toYear, queryDraft]);

  const filtered = useMemo(() => filterFeatures(features, filters), [features, filters]);
  const markers = useMemo(() => aggregateFeatures(filtered), [filtered]);
  // The subject layer rides the migration layer — same canvas, same grammar,
  // the data origin differs — and the banner below says so. It answers to the
  // same filters the main layer does: a status pill, era chip or search term
  // that hides a documented association cannot leave a person's مسالِك on the
  // map while claiming to have filtered the atlas.
  const subjectFiltered = useMemo(() => filterFeatures(subjectFeatures, filters), [subjectFeatures, filters]);
  // A documented مسالِك event is already a migration feature the main layer draws
  // (it is a migration_events row), so appending the subject layer's copies
  // whole double-draws every shared segment AND puts two rows in the list
  // sharing one feature id — duplicate React keys, which is undefined
  // reconciliation territory and once stranded stale rows in the DOM after a
  // data change. The subject layer contributes only what the main layer does
  // not already draw; the banner below still counts the layer's own matches.
  const filteredIDs = useMemo(() => new Set(filtered.map((feature) => feature.id)), [filtered]);
  const subjectOnly = useMemo(() => subjectFiltered.filter((feature) => !filteredIDs.has(feature.id)), [subjectFiltered, filteredIDs]);
  const migrations = useMemo(() => [...filtered.filter((feature) => feature.kind === "migration"), ...subjectOnly], [filtered, subjectOnly]);
  const regions = useMemo(() => filtered.filter((feature) => feature.kind === "region"), [filtered]);

  const selectedMarker = useMemo(() => {
    if (!search.feature && !search.place) return undefined;
    return markers.find((marker) => marker.key === search.feature)
      ?? markers.find((marker) => marker.members.some((member) => member.id === search.feature))
      ?? (search.place ? markers.find((marker) => marker.placeId === search.place) : undefined);
  }, [markers, search.feature, search.place]);

  const selectedFeature = useMemo(() => {
    const all = [...features, ...subjectFeatures];
    if (search.feature) {
      const exact = all.find((feature) => feature.id === search.feature);
      if (exact) return exact;
    }
    if (search.place) return all.find((feature) => feature.id === search.place || feature.placeId === search.place);
    return undefined;
  }, [features, subjectFeatures, search.feature, search.place]);

  // A deep link to a route or region is a claim about that route, not about
  // the places it touches: the reader asked for the line itself, so the sheet
  // they get is the line. The place-level aggregate stays preferred for
  // point-like (place / association) features, whose members carry the
  // evidence count.
  const exactlyLinkedFeature = useMemo(() => (search.feature ? [...features, ...subjectFeatures].find((feature) => feature.id === search.feature) : undefined), [features, subjectFeatures, search.feature]);
  const preferExactFeature = Boolean(exactlyLinkedFeature && (exactlyLinkedFeature.kind === "migration" || exactlyLinkedFeature.kind === "region"));
  const inspectorMarker = preferExactFeature ? undefined : selectedMarker;
  const inspectorFeature = preferExactFeature ? exactlyLinkedFeature : selectedMarker ? undefined : selectedFeature;
  const inspectorSelectedKey = (preferExactFeature ? exactlyLinkedFeature?.id : selectedMarker?.key) ?? selectedFeature?.id ?? ""

  const selectedKey = inspectorSelectedKey;

  useEffect(() => {
    const next = search.q ?? "";
    if (next === lastQuery.current) return;
    lastQuery.current = next;
    setQueryDraft(next);
  }, [search.q]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      const next = queryDraft.trim();
      if (next === (search.q ?? "")) return;
      lastQuery.current = next;
      void navigate({ to: "/places", search: compactSearch({ ...search, q: next || undefined }), replace: true, resetScroll: false });
    }, 320);
    return () => window.clearTimeout(timer);
  }, [queryDraft, search, navigate]);

  useEffect(() => {
    if (deepLinkFocused.current || !mapQuery.data) return;
    if (!search.feature && !search.place) return;
    const point = selectedMarker
      ? [selectedMarker.longitude, selectedMarker.latitude] as [number, number]
      : selectedFeature
        ? featurePoint(selectedFeature)
        : null;
    if (!point) return;
    deepLinkFocused.current = true;
    setFocus({ longitude: point[0], latitude: point[1], key: selectedKey });
  }, [mapQuery.data, search.feature, search.place, selectedMarker, selectedFeature, selectedKey]);

  useEffect(() => {
    if (selectedMarker) {
      setAnnouncement(`${selectedMarker.label}، ${mapStatusCopy(selectedMarker.status).label}، ${formatMapCount(selectedMarker.count)} إشارة`);
      return;
    }
    if (selectedFeature) {
      setAnnouncement(`${featureLabel(selectedFeature)}، ${mapStatusCopy(selectedFeature.status).label}`);
      return;
    }
    setAnnouncement("");
  }, [selectedMarker, selectedFeature]);

  const navigateSearch = (patch: Partial<AtlasSearch>) => {
    void navigate({ to: "/places", search: compactSearch({ ...search, ...patch }), replace: true, resetScroll: false });
  };

  const selectMarker = (marker: AtlasMarker, shouldFocus: boolean) => {
    if (shouldFocus) setFocus({ longitude: marker.longitude, latitude: marker.latitude, key: marker.key });
    navigateSearch({ feature: marker.key, place: marker.placeId });
  };

  const selectFeature = (feature: MapFeature) => {
    const point = featurePoint(feature);
    if (point) setFocus({ longitude: point[0], latitude: point[1], key: feature.id });
    navigateSearch({ feature: feature.id, place: feature.placeId });
  };

  const toggleStatus = (status: MapStatus) => {
    const next = statuses.includes(status) ? statuses.filter((item) => item !== status) : [...statuses, status];
    navigateSearch({ status: encodeAtlasFilter(next, mapStatuses), feature: undefined, place: undefined });
  };

  const toggleKind = (kind: MapKind) => {
    const next = kinds.includes(kind) ? kinds.filter((item) => item !== kind) : [...kinds, kind];
    navigateSearch({ kind: encodeAtlasFilter(next, mapKinds), feature: undefined, place: undefined });
  };

  const changeYears = (from: number, to: number) => {
    // Keep the window as chosen, even when it extends beyond the known
    // bounds. Clearing an edge that reaches past the bounds used to turn
    // "1727–1818" into "1727–∞" (the server then matches every dated row),
    // which is why an era chip outside the dated evidence could never stay
    // lit. The one exception is a window that covers the whole known range:
    // it is the same question as no window, so it normalises away.
    //
    // The selection deliberately survives: the inspector reads its feature
    // from the unfiltered set, so a reader checking "was this in the era?"
    // nudges the window and watches the chamber's era readouts answer in
    // place, instead of losing the thing they were checking.
    const coversAll = bounds !== null && from <= bounds.min && to >= bounds.max;
    navigateSearch({
      from: coversAll ? undefined : from,
      to: coversAll ? undefined : to,
    });
  };

  const applyPreset = (preset: PeriodPreset) => changeYears(preset.from, preset.to);

  const resetFilters = () => {
    setQueryDraft("");
    lastQuery.current = "";
    void navigate({ to: "/places", search: {}, replace: true, resetScroll: false });
  };

  const resetArea = () => {
    setViewport(null);
    setResetSignal((value) => value + 1);
    navigateSearch({ place: undefined });
  };

  const openAddPlace = () => {
    if (access.data?.signedIn) {
      setAddOpen(true);
      return;
    }
    setNotice("سجّل الدخول لإضافة موضع. المواضع الجديدة تُحفظ كمسودة خاصة بك، لا على الخريطة العامة.");
  };

  const visibleRows = useMemo<AtlasRow[]>(() => {
    const markerRows: AtlasRow[] = markers.map((marker) => {
      const entity = marker.members.find((item) => item.entityName)?.entityName;
      const source = marker.members.find((item) => item.sourceTitle)?.sourceTitle;
      const detail = entity || source || (marker.count > 1 ? `${formatMapCount(marker.count)} إشارات مجمّعة` : undefined);
      return {
        key: marker.key,
        label: marker.label,
        status: marker.status,
        kind: marker.kind,
        meta: marker.members[0]?.period ?? "غير محددة",
        detail,
        count: marker.count,
        marker,
      };
    });
    const featureRows: AtlasRow[] = [...migrations, ...regions].map((feature) => {
      let detail: string | undefined;
      if (feature.kind === "region") {
        const parts: string[] = [];
        if (feature.placeCount && feature.placeCount > 0) parts.push(`${formatMapCount(feature.placeCount)} مواضع موثّقة`);
        if (eraWindowRequested && feature.associationCount && feature.associationCount > 0) parts.push(`${formatMapCount(feature.associationCount)} شواهد في العصر`);
        if (parts.length > 0) detail = parts.join(" · ");
      } else if (feature.kind === "migration") {
        detail = feature.entityName || feature.sourceTitle || undefined;
      }
      return {
        key: feature.id,
        label: featureLabel(feature),
        status: feature.status,
        kind: feature.kind,
        meta: feature.period ?? "غير محددة",
        detail,
        count: 1,
        feature,
      };
    });
    return [...markerRows, ...featureRows].sort((left, right) => left.label.localeCompare(right.label, "ar"));
  }, [markers, migrations, regions, eraWindowRequested]);

  const directoryTribes = useMemo(() => {
    const list = captionNeedle ? captionMatches : captions;
    return [...list].sort((left, right) => left.nameAr.localeCompare(right.nameAr, "ar"));
  }, [captionNeedle, captionMatches, captions]);

  const effectiveResultsTab = resultsTab === "tribes" && captions.length === 0 ? "all" : resultsTab;

  const tabCounts = useMemo(() => {
    let place = 0;
    let migration = 0;
    let region = 0;
    for (const row of visibleRows) {
      if (row.kind === "migration") migration += 1;
      else if (row.kind === "region") region += 1;
      else place += 1;
    }
    return { all: visibleRows.length, place, migration, region, tribes: directoryTribes.length };
  }, [visibleRows, directoryTribes.length]);

  const tabbedRows = useMemo(() => {
    if (effectiveResultsTab === "all" || effectiveResultsTab === "tribes") return visibleRows;
    if (effectiveResultsTab === "place") return visibleRows.filter((row) => row.kind === "place" || row.kind === "association");
    return visibleRows.filter((row) => row.kind === effectiveResultsTab);
  }, [visibleRows, effectiveResultsTab]);

  // The fold is an explicit display bound, not a data bound: the response is already
  // bounded by MaxMapFeatures, so this only keeps a very long list from wall-of-text.
  // It sits above the whole demo vocabulary, so the atlas shows what it has.
  const rows = showAllRows ? tabbedRows : tabbedRows.slice(0, 24);
  const shownTribes = showAllRows ? directoryTribes : directoryTribes.slice(0, 24);
  const activeFilters = activeFilterCount(search) > 0 || queryDraft.trim() !== "";
  const initialError = mapQuery.isError && !mapQuery.data;
  const refreshError = mapQuery.isError && Boolean(mapQuery.data);
  const noFeatures = !mapQuery.isPending && !initialError && features.length === 0;
  const filteredEmpty = !mapQuery.isPending && !initialError && features.length > 0 && filtered.length === 0;

  return (
    <div className="page-stack atlas-page">
      <TopBar
        eyebrow="الخريطة الجغرافية / الأطلس"
        title="الموضع جزء من السؤال"
        description="الحضور، والهجرة، والحدود — كلها درجات حضور، وليست خطوطاً قاطعة."
      />

      <section className="atlas-toolbar" aria-label="شريط الأطلس">
        <div className="atlas-breadcrumb">
          <MapPinned size={16} />
          <span>فهرس المواضع</span>
          <ChevronLeft size={14} />
          <strong>{viewport ? "الإطار الحالي" : "كل المنطقة"}</strong>
        </div>
        <div className="atlas-toolbar-actions">
          <span className="atlas-toolbar-count">{formatMapCount(features.length)} إشارة في الذاكرة</span>
          <button type="button" className="icon-button" onClick={() => window.print()} title="طباعة / حفظ PDF" aria-label="طباعة / حفظ PDF">
            <Printer size={15} />
          </button>
          <button
            type="button"
            className="secondary-button atlas-inference-toggle"
            aria-pressed={inferenceOpen}
            title={access.data?.canReview ? "طبقة الاستنتاجات المحفوظة من آخر تحليل" : "طبقة الاستنتاجات تتطلب صلاحية باحث"}
            onClick={() => {
              if (access.data?.canReview) {
                setInferenceOpen((value) => !value);
                return;
              }
              setNotice("طبقة الاستنتاجات تتطلب صلاحية باحث. سجّل الدخول لعرضها.");
            }}
          >
            <ShieldQuestion size={15} />
            الطبقة الاستنتاجية
            {inferenceOpen && inference.run ? <b>{formatMapCount(inference.seals.length)}</b> : null}
          </button>
          <button type="button" className="primary-button" onClick={openAddPlace}>
            <Plus size={15} />
            إضافة موضع
          </button>
        </div>
      </section>

      {notice ? (
        <div className="atlas-notice" role="status">
          <span>{notice}</span>
          <button type="button" className="atlas-notice-close" onClick={() => setNotice("")} aria-label="أغلق التنبيه">×</button>
        </div>
      ) : null}

      <section
        className={`atlas-workspace${filtersCollapsed && inspectorCollapsed ? " atlas-workspace-canvas-only" : filtersCollapsed ? " atlas-workspace-no-filters" : inspectorCollapsed ? " atlas-workspace-no-inspector" : ""}`}
        aria-label="مساحة الأطلس"
      >
        {!filtersCollapsed ? (
          <AtlasFiltersPanel
            features={features}
            filteredCount={filtered.length}
            statuses={statuses}
            kinds={kinds}
            bounds={bounds}
            fromYear={fromYear}
            toYear={toYear}
            query={queryDraft}
            active={activeFilters}
            onToggleStatus={toggleStatus}
            onToggleKind={toggleKind}
            onYearChange={changeYears}
            onPreset={applyPreset}
            onQueryChange={setQueryDraft}
            onReset={resetFilters}
            plateRows={plateRegisterQuery.data?.plates ?? []}
            plateKeys={plateKeys}
            plateOpacity={plateOpacity}
            onTogglePlate={togglePlate}
            onFocusPlate={focusPlate}
            movementMode={search.mode ?? ""}
            season={search.season ?? ""}
            onToggleMovementMode={toggleMovementMode}
            onToggleSeason={toggleSeason}
            onPlateOpacity={setPlateOpacity}
            plateRegisterPending={plateRegisterQuery.isPending}
            plateRegisterFailed={plateRegisterQuery.isError}
            ottomanHaritaOn={layerVisibility.ottomanHarita1910}
            onToggleOttomanHarita={toggleOttomanHarita}
            onFocusOttomanHarita={focusOttomanHarita}
          />
        ) : null}

        <div className="atlas-stage">
          <div className="atlas-map-panel">
            <div className="atlas-map-head">
              <div className="atlas-map-head-copy">
                <div className="eyebrow">خريطة البيانات المحفوظة</div>
                <h2>حضور، هجرة، ومناطق تقريبية</h2>
                <p>لا خلفية بحدود اليوم. كل ما تراه مشتق من إشارات مسجلة، والمسافة بينها وبين اليقين ظاهرة.</p>
              </div>
              <div className="atlas-map-stats" aria-label="ملخص ما على الخريطة">
                <div><strong>{formatMapCount(markers.length)}</strong><span>نقطة مجمّعة</span></div>
                <div><strong>{formatMapCount(migrations.length)}</strong><span>هجرة</span></div>
                <div><strong>{formatMapCount(regions.length)}</strong><span>منطقة</span></div>
                <div><strong>{formatMapCount(relationArcs.length)}</strong><span>علاقة مسجّلة</span></div>
              </div>
            </div>

            <div className="atlas-map-actions" role="toolbar" aria-label="أدوات مساحة الخريطة">
              <div className="atlas-map-actions-group">
                <button
                  type="button"
                  className="secondary-button"
                  aria-pressed={!filtersCollapsed}
                  title={filtersCollapsed ? "إظهار لوحة التصفية" : "طي لوحة التصفية"}
                  onClick={() => setFiltersCollapsed((value) => !value)}
                >
                  {filtersCollapsed ? <PanelRightOpen size={14} /> : <PanelRightClose size={14} />}
                  التصفية
                </button>
                <button
                  type="button"
                  className="secondary-button"
                  aria-pressed={!inspectorCollapsed}
                  title={inspectorCollapsed ? "إظهار الفاحصة" : "طي الفاحصة"}
                  onClick={() => setInspectorCollapsed((value) => !value)}
                >
                  {inspectorCollapsed ? <PanelLeftOpen size={14} /> : <PanelLeftClose size={14} />}
                  الفاحصة
                </button>
                <button
                  type="button"
                  className="secondary-button"
                  aria-pressed={legendOpen}
                  title={legendOpen ? "إخفاء مفتاح الخريطة" : "إظهار مفتاح الخريطة"}
                  onClick={() => setLegendOpen((value) => !value)}
                >
                  {legendOpen ? <EyeOff size={14} /> : <Eye size={14} />}
                  المفتاح
                </button>
              </div>
              <div className="atlas-map-actions-group">
                <button type="button" className="secondary-button" aria-pressed={plateView}
                  disabled={!plateRegister?.plates.some((plate) => plate.transform)} onClick={togglePlateView}>
                  {plateView ? "عرض الأدلة واللوحة" : "قراءة اللوحة"}
                </button>
                <button
                  type="button"
                  className="secondary-button"
                  aria-pressed={swipePercent !== null}
                  disabled={plateLayers.length === 0}
                  title={plateLayers.length === 0 ? "فعّل لوحة مرجعية أولاً لاستخدام ستارة المقارنة" : "سحب ستارة لمقارنة اللوحة التاريخية مع الأطلس الموثّق"}
                  onClick={toggleSwipe}
                >
                  <Columns2 size={14} />
                  ستارة اللوحة
                </button>
                <button
                  type="button"
                  className="secondary-button"
                  aria-pressed={compareArmed}
                  disabled={(plateRegisterQuery.data?.plates ?? []).every((row) => !row.transform)}
                  title={(plateRegisterQuery.data?.plates ?? []).every((row) => !row.transform) ? "لا لوحة صالحة للمقارنة بعد" : undefined}
                  onClick={armCompare}
                >
                  <Crosshair size={14} />
                  {compareArmed ? "انقر الخريطة…" : "مقارنة نقطة"}
                </button>
              </div>
            </div>

            {legendOpen ? (
              <section className="atlas-legend" aria-label="مفتاح الحالات والأنواع">
                {mapStatuses.map((status) => {
                  const copy = mapStatusVocabulary[status];
                  return <span key={status} role="img" aria-label={copy.label} className={`atlas-legend-status${copy.seal ? " atlas-legend-seal" : ""}${copy.hollow ? " atlas-legend-hollow" : ""}`}><i aria-hidden="true" style={{ color: copy.stroke }}>{copy.glyph}</i>{copy.label}</span>;
                })}
                <span className="atlas-legend-kind" role="img" aria-label="ساحل مرجع طبيعي"><i aria-hidden="true">≈</i> ساحل مرجع طبيعي</span>
                <span className="atlas-legend-kind" role="img" aria-label="أسماء مدن وخلجان مرجعية"><i aria-hidden="true">·</i> أسماء مرجعية (مدن وخلجان)</span>
                <span className="atlas-legend-kind" role="img" aria-label="هجرة بسهم"><i aria-hidden="true">↗</i> هجرة بسهم</span>
                <span className="atlas-legend-kind" role="img" aria-label="منطقة تقريبية"><i aria-hidden="true">▱</i> منطقة تقريبية</span>
                <span className="atlas-legend-kind" role="img" aria-label={mapUndatedRegionCopy.noteInsideWindow}><i aria-hidden="true">{mapUndatedRegionCopy.legendGlyph}</i> {mapUndatedRegionCopy.label}</span>
              </section>
            ) : null}

            {inferenceOpen ? (
              <AtlasInferenceOverlay
                state={inference}
                onClose={() => setInferenceOpen(false)}
                onFocus={(key, longitude, latitude) => {
                  setSelectedSealKey(key);
                  setFocus({ longitude, latitude, key });
                }}
              />
            ) : null}

            {subjectID ? (
              <div className="atlas-subject-banner" role="status">
                <strong>{subjectPersonName ? `مسار ${subjectPersonName}` : "طبقة الموضوع"}</strong>
                <span>{movementQuery.isPending ? "يُقرأ المسار…" : movementQuery.data && movementQuery.data.events.length === 0 ? "لا مسار موثّق بعد" : `${formatMapCount(subjectFiltered.length)} حدث موثّق`}</span>
                <span>الطبقة تابعة للتصفية الحالية.</span>
                {movementQuery.data?.truncated ? <span>السجل أطول من الحد؛ عُرض المقتطع الأول فقط.</span> : null}
                <button type="button" className="text-button" onClick={() => navigateSearch({ subject: undefined })}>إغلاق المسار</button>
              </div>
            ) : null}

            {captionMatches.length > 0 && (
              <section aria-label="أسماء مطابقة على اللوحات" className="atlas-caption-results">
                {captionMatches.map((label) => (
                  <button key={label.id} type="button" className="secondary-button" onClick={() => selectCaption(label)}>
                    {label.nameAr} <bdi lang="en">{label.printedName}</bdi>
                  </button>
                ))}
              </section>
            )}
            <div className={`atlas-canvas-wrap${canvasExpanded ? " atlas-canvas-wrap-expanded" : ""}`}>
              <div className="atlas-canvas-controls" role="toolbar" aria-label="أدوات عرض الخريطة">
                <button
                  type="button"
                  className="atlas-canvas-control-btn"
                  aria-pressed={canvasExpanded}
                  title={canvasExpanded ? "إنهاء ملء الشاشة (Esc)" : "ملء الشاشة"}
                  aria-label={canvasExpanded ? "إنهاء ملء الشاشة" : "ملء الشاشة"}
                  onClick={() => setCanvasExpanded((value) => !value)}
                >
                  {canvasExpanded ? <Minimize2 size={15} /> : <Maximize2 size={15} />}
                </button>
                <div className="atlas-layers-menu-wrap">
                  <button
                    type="button"
                    className="atlas-canvas-control-btn"
                    aria-expanded={layersMenuOpen}
                    title="طبقات العرض"
                    aria-label="طبقات العرض"
                    onClick={() => setLayersMenuOpen((value) => !value)}
                  >
                    <Layers3 size={15} />
                  </button>
                  {layersMenuOpen ? (
                    <div className="atlas-layers-popover" role="group" aria-label="تبديل طبقات العرض">
                      <div className="atlas-layers-popover-title">طبقات العرض</div>
                      {([
                        ["regions", "المناطق التقريبية"],
                        ["relations", "أقواس العلاقات"],
                        ["corridors", "ممرات الحركة"],
                        ["reference", "المدن والخلجان المرجعية"],
                        ["graticule", "خطوط الطول والعرض"],
                        ["plateLabels", "أسماء اللوحة العربية"],
                        ["plateTerritories", "حدود القبائل على اللوحة"],
                        ["ottomanHarita1910", "حدود خريطة ٩٣٦٧٧ العثمانية (١٩١٠)"],
                      ] as const).map(([key, label]) => (
                        <label key={key} className="atlas-layer-check">
                          <input
                            type="checkbox"
                            checked={layerVisibility[key]}
                            onChange={() => toggleLayerVisibility(key)}
                          />
                          <span>{label}</span>
                        </label>
                      ))}
                    </div>
                  ) : null}
                </div>
              </div>
              <AtlasMap
                markers={plateView ? [] : markers}
                migrations={plateView ? [] : migrations}
                regions={plateView ? [] : regions}
                selectedKey={selectedKey}
                onSelectMarker={(marker) => {
                  setSelectedOttomanTerritory(null);
                  setInspectorCollapsed(false);
                  selectMarker(marker, false);
                }}
                onSelectFeature={(feature) => {
                  setSelectedOttomanTerritory(null);
                  setInspectorCollapsed(false);
                  selectFeature(feature);
                }}
                onClearSelection={() => {
                  setSelectedOttomanTerritory(null);
                  navigateSearch({ feature: undefined, place: undefined });
                }}
                onViewportChange={setViewport}
                plateLayers={plateLayers}
                selectedCaptionId={selectedCaption?.id}
                selectedCaptionNameAr={selectedCaption?.nameAr}
                selectedOttomanId={selectedOttomanTerritory?.id ?? ""}
                onSelectOttomanTerritory={compareArmed ? undefined : selectOttomanTerritory}
                relations={plateView ? [] : relationArcs}
                corridors={plateView ? undefined : corridorsData}
                compareArmed={compareArmed}
                onComparePick={(longitude, latitude) => {
                  setInspectorCollapsed(false);
                  navigateSearch({ caption: undefined, compare: `${longitude.toFixed(4)},${latitude.toFixed(4)}` });
                }}
                comparePoint={comparePoint}
                onSelectCaption={compareArmed ? undefined : selectCaption}
                focus={focus}
                fitBoundsRequest={fitBoundsRequest}
                layerVisibility={layerVisibility}
                swipePercent={swipePercent}
                onSwipeChange={setSwipePercent}
                /* A deep link trains the first flight on its feature: fitting the
                   peninsula first and then flying to the selection is two opposite
                   flights, and the link's answer is the feature. */
                skipInitialFit={Boolean(search.feature || search.place) && !deepLinkFocused.current}
                resetSignal={resetSignal}
                inferences={!plateView && inferenceOpen ? inference.seals : []}
                selectedInferenceKey={selectedSealKey ?? ""}
                onSelectInference={(seal) => {
                  setSelectedSealKey(seal.key);
                  setFocus({ longitude: seal.longitude, latitude: seal.latitude, key: seal.key });
                }}
              />
              {mapQuery.isPending ? <div className="atlas-map-state" role="status"><LoaderCircle size={18} className="atlas-spin" /> جارٍ تحميل طبقات الخريطة…</div> : null}
              {initialError ? <div className="atlas-map-state atlas-map-error" role="alert"><CircleAlert size={19} /><strong>تعذر تحميل طبقات الخريطة</strong><span>{errorMessage(mapQuery.error)}</span><button type="button" className="secondary-button" onClick={() => void mapQuery.refetch()}>أعد المحاولة</button></div> : null}
              {noFeatures ? <div className="atlas-map-state atlas-map-empty" role="status"><MapPinned size={19} /><strong>لم تُسجَّل إشارات في هذا الإطار بعد</strong><span>وسّع الإطار أو أعد ضبطه؛ لا نعرض مواضع توضيحية مكان الواقع.</span><button type="button" className="secondary-button" onClick={resetArea}><RotateCcw size={14} /> اعرض كل المنطقة</button></div> : null}
              {filteredEmpty ? <div className="atlas-map-state atlas-map-empty" role="status"><SearchX size={19} /><strong>لا توجد إشارات تطابق التصفية</strong><span>الإشارات موجودة، لكن ما اخترته لا يطابقها. أعد ضبط التصفية لتعود.</span><button type="button" className="secondary-button" onClick={resetFilters}><RotateCcw size={14} /> أعد ضبط التصفية</button></div> : null}
              {canvasExpanded && selectedCaption && captionPlate && !comparePoint ? (
                <PlateCaptionPanel
                  floating
                  label={selectedCaption}
                  plate={captionPlate}
                  territorySpanAr={selectedTerritory?.spanAr}
                  hasTerritory={Boolean(selectedTerritory)}
                  canModerate={Boolean(access.data?.canMerge)}
                  onClose={() => navigateSearch({ caption: undefined })}
                />
              ) : null}
              {canvasExpanded && selectedOttomanTerritory && !selectedCaption && !comparePoint ? (
                <OttomanHaritaPanel
                  floating
                  territory={selectedOttomanTerritory}
                  onClose={() => setSelectedOttomanTerritory(null)}
                />
              ) : null}
              {canvasExpanded && search.occurrence && !selectedCaption && !comparePoint ? (
                <AtlasOccurrenceOverlay
                  floating
                  first={search.occurrence}
                  second={search.compareOccurrence}
                  canModerate={Boolean(access.data?.canMerge)}
                  onClose={() => navigateSearch({ occurrence: undefined, compareOccurrence: undefined })}
                />
              ) : null}
              {canvasExpanded && comparePoint ? (
                <PlateComparePanel
                  floating
                  register={plateRegisterQuery.data?.plates ?? []}
                  longitude={comparePoint.longitude}
                  latitude={comparePoint.latitude}
                  onClose={clearCompare}
                />
              ) : null}
            </div>

            <div className="atlas-map-foot">
              <div className="atlas-map-note"><CircleAlert size={14} /><span>الخطوط المنقطة استنتاجات، والمضلعات مناطق تقريبية. لا شيء هنا حدّ تاريخي مؤكد. الخط الساحلي مرجع طبيعي مبسط (Natural Earth 50م)؛ المضلعات مناطق تقريبية معلنة، والمدن والخلجان أسماء مرجعية لا سجل.</span></div>
              <div className="atlas-map-state-line" aria-live="polite">
                {mapQuery.isFetching && mapQuery.data ? <span><LoaderCircle size={12} className="atlas-spin" /> جارٍ تحديث الإطار…</span> : null}
                <span>{viewport ? "داخل الإطار الحالي" : "كل المنطقة"}</span>
              </div>
            </div>

            {refreshError ? <div className="atlas-truncation" role="status"><CircleAlert size={14} /><span>تعذر تحديث هذا الإطار. ما تراه هو آخر قراءة ناجحة.</span><button type="button" className="text-button" onClick={() => void mapQuery.refetch()}>أعد المحاولة</button></div> : null}
            {mapQuery.data?.truncated ? (
              <div className="atlas-truncation" role="status">
                <CircleAlert size={14} />
                <span>أُعيدت أول {formatMapCount(features.length)} إشارة ضمن الحد. قرّب الإطار أو استخدم التصفية لرؤية الباقي.</span>
              </div>
            ) : null}
          </div>

          <section className={`atlas-results${resultsOpen ? "" : " atlas-results-collapsed"}`} aria-label="الإشارات في الإطار">
            <div className="atlas-results-head">
              <div className="atlas-results-head-copy">
                <div className="eyebrow">فهرس الإطار</div>
                <h2>الإشارات في الإطار</h2>
              </div>
              <div className="atlas-results-head-meta">
                <span className="atlas-results-summary-inline">
                  {formatMapCount(tabCounts.place)} مواضع · {formatMapCount(tabCounts.migration)} هجرة · {formatMapCount(tabCounts.region)} منطقة
                  {captions.length > 0 ? ` · ${formatMapCount(tabCounts.tribes)} قبيلة على اللوحة` : ""}
                </span>
                <span className="atlas-results-count">
                  {effectiveResultsTab === "tribes"
                    ? `${formatMapCount(directoryTribes.length)} قبيلة`
                    : `${formatMapCount(tabbedRows.length)} صفاً`}
                </span>
                <button
                  type="button"
                  className="atlas-results-toggle"
                  aria-expanded={resultsOpen}
                  onClick={() => setResultsOpen((value) => !value)}
                >
                  {resultsOpen ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
                  <span>{resultsOpen ? "طي الفهرس" : "فتح الفهرس"}</span>
                </button>
              </div>
            </div>
            {resultsOpen ? (
              <>
                {visibleRows.length > 0 || captions.length > 0 ? (
                  <div className="atlas-results-tabs" role="tablist" aria-label="تصنيف قائمة الإشارات">
                    {([
                      ["all", "الكل", tabCounts.all],
                      ["place", "مواضع", tabCounts.place],
                      ["migration", "هجرات", tabCounts.migration],
                      ["region", "مناطق", tabCounts.region],
                      ...(captions.length > 0 ? [["tribes", "قبائل اللوحة", tabCounts.tribes] as const] : []),
                    ] as const).map(([key, label, count]) => (
                      <button
                        key={key}
                        type="button"
                        role="tab"
                        aria-selected={effectiveResultsTab === key}
                        className={`atlas-results-tab${effectiveResultsTab === key ? " atlas-results-tab-active" : ""}`}
                        onClick={() => { setResultsTab(key); setShowAllRows(false); }}
                      >
                        <span>{label}</span>
                        <b>{formatMapCount(count)}</b>
                      </button>
                    ))}
                  </div>
                ) : null}
                {effectiveResultsTab === "tribes" ? (
                  directoryTribes.length > 0 ? (
                    <>
                      <div className="atlas-row-grid">
                        {shownTribes.map((tribe) => {
                          const active = selectedCaption?.id === tribe.id;
                          return (
                            <button
                              type="button"
                              key={tribe.id}
                              className={`atlas-row atlas-row-tribe${active ? " atlas-row-active" : ""}`}
                              aria-pressed={active}
                              onClick={() => selectCaption(tribe)}
                            >
                              <div className="atlas-row-top">
                                <span className="atlas-row-tribe-dot" aria-hidden="true" />
                                <strong className="atlas-row-title">{tribe.nameAr}</strong>
                                <ChevronLeft size={14} className="atlas-row-chevron" aria-hidden="true" />
                              </div>
                              <div className="atlas-row-meta">
                                <span className="atlas-row-printed"><bdi lang="en">{tribe.printedName}</bdi></span>
                                {tribe.tribeId ? <span className="atlas-row-dossier-tag">سجل قبيلة</span> : <span className="atlas-row-period">على اللوحة</span>}
                              </div>
                            </button>
                          );
                        })}
                      </div>
                      {directoryTribes.length > 24 ? (
                        <button type="button" className="atlas-more" onClick={() => setShowAllRows((value) => !value)}>
                          {showAllRows ? "أظهر أقل" : `أظهر كل ${formatMapCount(directoryTribes.length)} قبيلة`}
                        </button>
                      ) : null}
                    </>
                  ) : (
                    <p className="atlas-results-empty">لا قبائل على اللوحة تطابق البحث الحالي.</p>
                  )
                ) : tabbedRows.length > 0 ? (
                  <>
                    <div className="atlas-row-grid">
                      {rows.map((row) => {
                        const copy = mapStatusCopy(row.status);
                        const dated = row.meta !== "غير محددة";
                        return (
                          <button
                            type="button"
                            key={row.key}
                            className={`atlas-row${selectedKey === row.key ? " atlas-row-active" : ""}`}
                            aria-pressed={selectedKey === row.key}
                            onClick={() => {
                              setInspectorCollapsed(false);
                              if (row.marker) selectMarker(row.marker, true);
                              else if (row.feature) selectFeature(row.feature);
                            }}
                          >
                            <div className="atlas-row-top">
                              <span className={`atlas-row-glyph atlas-row-${row.status}${copy.seal ? " atlas-row-seal" : ""}${copy.hollow ? " atlas-row-hollow" : ""}`} aria-hidden="true" style={{ color: copy.stroke }}>{copy.glyph}</span>
                              <strong className="atlas-row-title">{row.label}</strong>
                              {row.count > 1 ? <b className="atlas-row-count" title={`${formatMapCount(row.count)} إشارات مجمّعة`}>{formatMapCount(row.count)}</b> : null}
                              <ChevronLeft size={14} className="atlas-row-chevron" aria-hidden="true" />
                            </div>
                            {row.detail ? <div className="atlas-row-detail">{row.detail}</div> : null}
                            <div className="atlas-row-meta">
                              <span className="atlas-row-kind-chip">{mapKindLabel(row.kind)}</span>
                              <span className={`atlas-row-period${dated ? " atlas-row-period-dated" : ""}`}>{row.meta}</span>
                            </div>
                          </button>
                        );
                      })}
                    </div>
                    {tabbedRows.length > 24 ? (
                      <button type="button" className="atlas-more" onClick={() => setShowAllRows((value) => !value)}>
                        {showAllRows ? "أظهر أقل" : `أظهر كل ${formatMapCount(tabbedRows.length)}`}
                      </button>
                    ) : null}
                  </>
                ) : (
                  <p className="atlas-results-empty">لا صفوف تطابق التصفية الحالية.</p>
                )}
              </>
            ) : null}
          </section>
        </div>

        {!inspectorCollapsed ? (
          comparePoint ? (
            <PlateComparePanel
              register={plateRegisterQuery.data?.plates ?? []}
              longitude={comparePoint.longitude}
              latitude={comparePoint.latitude}
              onClose={clearCompare}
            />
          ) : selectedCaption && captionPlate ? (
            <PlateCaptionPanel
              label={selectedCaption}
              plate={captionPlate}
              territorySpanAr={selectedTerritory?.spanAr}
              hasTerritory={Boolean(selectedTerritory)}
              canModerate={Boolean(access.data?.canMerge)}
              onClose={() => navigateSearch({ caption: undefined })}
            />
          ) : selectedOttomanTerritory ? (
            <OttomanHaritaPanel
              territory={selectedOttomanTerritory}
              onClose={() => setSelectedOttomanTerritory(null)}
            />
          ) : search.occurrence ? (
            <AtlasOccurrenceOverlay
              first={search.occurrence}
              second={search.compareOccurrence}
              canModerate={Boolean(access.data?.canMerge)}
              onClose={() => navigateSearch({ occurrence: undefined, compareOccurrence: undefined })}
            />
          ) : (
            <AtlasInspector marker={inspectorMarker} feature={inspectorFeature} onClose={() => navigateSearch({ feature: undefined, place: undefined })} window={eraWindow} canModerate={Boolean(access.data?.canMerge)} />
          )
        ) : null}
      </section>

      <section className="map-method-note" aria-label="منهجية العرض">
        <Layers3 size={18} />
        <div>
          <strong>كيف نعرض عدم اليقين؟</strong>
          <span>نقطة لكل موضع وحالة، وخط هجرة بسهم، ومنطقة تقريبية. العدّاد يقول كم إشارة جُمعت، ولا يكبّر النقطة ليوحي بيقين أكبر.</span>
        </div>
        <Link to="/research">اقرأ منهجية البحث <ChevronLeft size={14} /></Link>
      </section>

      <div className="atlas-live" aria-live="polite">{announcement}</div>

      <AddPlaceDialog
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onCreated={(place) => {
          setNotice(`أُضيف «${place.nameAr}» كمسودة خاصة. سيظهر على خريطتك بعد التحديث إن كان لك وصول إليه.`);
          setViewport(null);
          setResetSignal((value) => value + 1);
          void mapQuery.refetch();
        }}
      />
    </div>
  );
}

function compactSearch(search: AtlasSearch): AtlasSearch {
  const next: AtlasSearch = {};
  if (search.place) next.place = search.place;
  if (search.feature) next.feature = search.feature;
  if (search.subject) next.subject = search.subject;
  if (search.from !== undefined) next.from = search.from;
  if (search.to !== undefined) next.to = search.to;
  if (search.status) next.status = search.status;
  if (search.kind) next.kind = search.kind;
  if (search.q) next.q = search.q;
  // The 032/033 lanes' own state rides the same compact contract: the plate
  // layer's ON keys + opacity, the compare point's arm/hold, and the
  // movement's own filters (mode/season).
  if (search.plates) next.plates = search.plates;
  if (search.plateOpacity !== undefined) next.plateOpacity = search.plateOpacity;
  if (search.compare) next.compare = search.compare;
  if (search.caption) next.caption = search.caption;
  if (search.occurrence) next.occurrence = search.occurrence;
  if (search.compareOccurrence) next.compareOccurrence = search.compareOccurrence;
  if (search.view) next.view = search.view;
  if (search.mode) next.mode = search.mode;
  if (search.season) next.season = search.season;
  return next;
}

/**
 * Debounces an object whose per-render identity is untrustworthy. The key
 * exists because `Object.is` on a per-render object is a render loop: the era
 * window once arrived as a fresh object every render, and a debounce keyed on
 * the value itself rescheduled every 300 ms for as long as the URL carried a
 * window. A stable, content-addressed key makes that unrepresentable: any
 * number of renders with the same content reschedule nothing, and a content
 * change schedules exactly once.
 */
function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "تعذر الاتصال بالخدمة.";
}
