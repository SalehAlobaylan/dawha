import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { featurePoint } from "./atlas";
import {
  fetchGeospatialFindings,
  fetchLatestGeospatialIntelligenceRun,
  ApiError,
} from "./api";
import type {
  GeospatialCluster,
  GeospatialFinding,
  GeospatialIntelligenceRun,
  MapFeature,
} from "../types";

/**
 * The inference layer, as a hook.
 *
 * The Atlas answers "what is in the record?" with the canvas and answers "what
 * did the platform infer?" here, behind a toggle. Everything the layer draws is
 * derived from one run — the latest one this account can access — plus the
 * features already in memory. A finding whose places the map has not loaded is
 * listed without a pin; coordinates are never invented.
 *
 * Stamp-ring grammar only: the seals are olive rotated squares, never the disc
 * grammar documented features use, because a sealed cluster is a hypothesis and
 * a finding is a request for review, not a verdict.
 */
export interface AtlasInferenceSeal {
  key: string;
  longitude: number;
  latitude: number;
  label: string;
  kind: "cluster" | "finding";
  count?: number;
}

export interface AtlasInferenceState {
  run: GeospatialIntelligenceRun | null;
  findings: GeospatialFinding[];
  seals: AtlasInferenceSeal[];
  enabled: boolean;
  isPending: boolean;
  isError: boolean;
  forbidden: boolean;
}

export function useAtlasInference(enabled: boolean, features: MapFeature[]): AtlasInferenceState {
  const runQuery = useQuery({
    queryKey: ["atlas-inference-latest"],
    queryFn: () => fetchLatestGeospatialIntelligenceRun({}),
    enabled,
    staleTime: 30000,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const run = enabled ? runQuery.data ?? null : null;
  const findingsQuery = useQuery({
    queryKey: ["atlas-inference-findings", run?.id ?? ""],
    queryFn: () => fetchGeospatialFindings(run!.id),
    enabled: Boolean(enabled && run && run.findingCount > 0),
    staleTime: 30000,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const findings = useMemo(() => findingsQuery.data ?? [], [findingsQuery.data]);

  const seals = useMemo<AtlasInferenceSeal[]>(() => {
    if (!run) return [];
    const pointByPlace = new Map<string, [number, number]>();
    for (const feature of features) {
      if (!feature.placeId || pointByPlace.has(feature.placeId)) continue;
      const point = featurePoint(feature);
      if (point) pointByPlace.set(feature.placeId, point);
    }
    const out: AtlasInferenceSeal[] = [];
    for (const cluster of run.report.clusters) {
      out.push(clusterSeal(cluster));
    }
    for (const finding of findings) {
      if (finding.status !== "needs_review" && finding.status !== "investigating") continue;
      for (const placeID of finding.placeIds) {
        const point = pointByPlace.get(placeID);
        if (!point) continue;
        out.push({
          key: `finding-${finding.id}-${placeID}`,
          longitude: point[0],
          latitude: point[1],
          label: finding.titleAr,
          kind: "finding",
        });
      }
    }
    return out;
  }, [run, features, findings]);

  const forbidden = runQuery.isError && runQuery.error instanceof ApiError
    && (runQuery.error.status === 401 || runQuery.error.status === 403);

  return {
    run,
    findings,
    seals,
    enabled,
    isPending: enabled && (runQuery.isPending || Boolean(run && run.findingCount > 0 && findingsQuery.isPending)),
    isError: runQuery.isError && !forbidden,
    forbidden,
  };
}

function clusterSeal(cluster: GeospatialCluster): AtlasInferenceSeal {
  const places = cluster.placeNames.length > 0 ? cluster.placeNames.join("، ") : "تجمع بلا اسم";
  return {
    key: `cluster-${cluster.id}`,
    longitude: cluster.centerLongitude,
    latitude: cluster.centerLatitude,
    label: `تجمع استنتاجي: ${places}`,
    kind: "cluster",
    count: cluster.associationIds.length,
  };
}
