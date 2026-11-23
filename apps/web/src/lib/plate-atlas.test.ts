import { describe, expect, it } from "vitest";

import type { AtlasPlateRegisterRow } from "./api";
import { plateNeatlineRing, plateForwardLonLat, plateSourceBounds, relationArcCollection, ringContainsPoint, type AtlasPlateLayer } from "./atlas";
import { ottomanHarita1910Territories, ottomanHaritaCollection, placePlateTribeLabels, plateHttpDefaultOpacity, plateLayerSpecs, plateTerritoryCollection } from "./plate-atlas";

/**
 * The atlas's plate lane tests (plans 032-A/B/C). The contract they pin: the
 * register decides renderability; the canvas draws ONLY what the stored
 * transform validates; every number is a number the server measured. The
 * server's per-pixel warp and its own inverse are tested in
 * `internal/regionreview` and `internal/geography` — their client-side
 * expectations stop where the server's proof begins.
 */

// The Admiralty plate's own artifact numbers (the seeded georef block), as
// the API's register row carries them.
const artifactTransform = {
  kind: "equidistant_conic",
  apexX: 3416.88,
  apexY: -23190.58,
  coneConstant: 0.444164,
  centralLongitude: 45.5126,
  latC0: -0.004322437,
  latC1: 140.780974604,
  pixelsPerDegree: 231.351,
};

const artifactFrame = {
  outerLeft: 370,
  innerLeft: 514.5,
  innerTop: 513.5,
  innerRight: 6604.5,
  outerRight: 6748.5,
  outerTop: 369,
  innerBottom: 6604.5,
  outerBottom: 6750,
};

const admiraltyRow: AtlasPlateRegisterRow = {
  sourceId: "b0000000-0000-0000-0000-00000000e004",
  plateKey: "admiralty-1916-tribal",
  titleAr: "خريطة قبائل الجزيرة العربية — هيئة الأركان البحريّة البريطانية ١٩١٦",
  license: "public_domain",
  plateDate: "2026-11-16",
  plateWidth: 6870,
  plateHeight: 7200,
  controlPoints: 20,
  transform: {
    method: "graticule_labels",
    transform: artifactTransform,
    frame: artifactFrame,
    fit: { lonRmsDeg: 0.02367, latRmsDeg: 0.06896, lonMaxDeg: 0.04141, latMaxDeg: 0.15166 },
    certainty: "approximate",
    needsReview: true,
    reviewReasons: ["interior_parallels_unverified_by_ink"],
  },
};

const dicksonRow: AtlasPlateRegisterRow = {
  sourceId: "b0000000-0000-0000-0000-00000000e005",
  plateKey: "dickson-1914-tribal",
  titleAr: "خريطة قبائل الجزيرة العربية التقريبية — هارولد ديكسون ١٩١٤",
  license: "cc_by_sa_3_0",
  controlPoints: 0,
  disabledCaption: "الطبقة تنتظر نقاط تحكم من ملاك المواضع",
};

const register = { plates: [admiraltyRow, dicksonRow], limit: 20, truncated: false };

describe("plate layer specs (plans 032-A/B)", () => {
  it("renders only the register's validated rows, in the caller's key order", () => {
    const specs = plateLayerSpecs(register, ["admiralty-1916-tribal"], 65);
    expect(specs).toHaveLength(1);
    expect(specs[0].key).toBe("admiralty-1916-tribal");
    expect(specs[0].transform).toEqual(artifactTransform);
    expect(specs[0].frame).toEqual(artifactFrame);
    expect(specs[0].opacity).toBe(0.65);
    expect(specs[0].needsReview).toBe(true);
  });

  it("never renders a plate without a transform, even when the URL names it", () => {
    const specs = plateLayerSpecs(register, ["dickson-1914-tribal", "admiralty-1916-tribal"], plateHttpDefaultOpacity);
    expect(specs).toHaveLength(1);
    expect(specs[0].key).toBe("admiralty-1916-tribal");
  });

  it("clamps the drawn opacity into the honest band (0.05..1)", () => {
    expect(plateLayerSpecs(register, ["admiralty-1916-tribal"], 120)[0].opacity).toBe(1);
    expect(plateLayerSpecs(register, ["admiralty-1916-tribal"], 2)[0].opacity).toBe(0.05);
  });

  it("an unvalidated register (fetch failed) renders nothing, not a guess", () => {
    expect(plateLayerSpecs(null, ["admiralty-1916-tribal"], 65)).toHaveLength(0);
  });

  it("projects localized names at the printed locations of the visible Admiralty plate", () => {
    const keys = ["admiralty-1916-tribal"];
    const [plate] = plateLayerSpecs(register, keys, 65);
    const ateibah = plate.tribeLabels?.find((label) => label.nameAr === "عتيبة");
    expect(ateibah).toMatchObject({
      printedName: "'ATEIBAH",
      coordinates: plateForwardLonLat(artifactTransform, 3140, 3561),
    });
    expect(plate.tribeLabels?.filter((label) => label.nameAr === "شمّر")).toHaveLength(2);
    expect(plateLayerSpecs(register, [], 65)).toEqual([]);
  });

  it("projects localized Arabic names on the georeferenced Dickson 1914 plate", () => {
    const dicksonAffine = {
      kind: "affine",
      affine: [0.018955080973840306, 0.0004665548175556769, 30.78100094900797, -2.413045233123656e-05, -0.016991240788847728, 41.58377432177165] as [number, number, number, number, number, number],
      apexX: 0, apexY: 0, coneConstant: 0, centralLongitude: 0, latC0: 0, latC1: 0, pixelsPerDegree: 58.85,
    };
    const dicksonWithTransform: AtlasPlateRegisterRow = {
      ...dicksonRow,
      transform: {
        method: "control_points",
        transform: dicksonAffine,
        frame: { outerLeft: 12, innerLeft: 12, outerRight: 1559, innerRight: 1559, outerTop: 12, innerTop: 12, outerBottom: 1749, innerBottom: 1749 },
        fit: { lonRmsDeg: 0.324, latRmsDeg: 0.224, lonMaxDeg: 0.605, latMaxDeg: 0.392 },
        certainty: "approximate",
        needsReview: true,
        reviewReasons: ["landmark_based_registration"],
      },
    };
    const [plate] = plateLayerSpecs({ plates: [dicksonWithTransform] }, ["dickson-1914-tribal"], 65);
    expect(plate.tribeLabels?.length).toBeGreaterThanOrEqual(90);
    const utaibah = plate.tribeLabels?.find((label) => label.nameAr === "عتيبة");
    expect(utaibah).toMatchObject({
      printedName: "'Utaibah (Many Section)",
      coordinates: plateForwardLonLat(dicksonAffine, 596, 1068),
    });
    const territories = plateTerritoryCollection([plate]);
    expect(territories.features.length).toBeGreaterThanOrEqual(70);
    const harb = territories.features.find((feature) => feature.properties.nameAr === "حرب");
    expect(harb).toBeDefined();
    expect(harb?.geometry.coordinates[0].length).toBeGreaterThanOrEqual(10);
  });

  it("exposes normalized Ottoman Harita 93677 (c. 1910) Najd & Arabia polygons with census metadata", () => {
    const collection = ottomanHaritaCollection();
    expect(ottomanHarita1910Territories.length).toBeGreaterThanOrEqual(30);
    expect(collection.features.length).toBeGreaterThanOrEqual(30);
    const harb = ottomanHarita1910Territories.find((item) => item.nameAr === "حرب");
    expect(harb?.population).toBe(80000);
    expect(harb?.tents).toBe(10000);
    const utaibah = ottomanHarita1910Territories.find((item) => item.nameAr === "عتيبة");
    expect(utaibah?.population).toBe(100000);
  });

  it("keeps nearby names legible and reveals both when they have room", () => {
    const [plate] = plateLayerSpecs(register, ["admiralty-1916-tribal"], 65);
    const labels = plate.tribeLabels?.slice(0, 2) ?? [];
    const project = ([longitude]: [number, number]) => ({ x: 100 + longitude, y: 100 });
    expect(placePlateTribeLabels(labels, project, 800, 200)).toHaveLength(1);
    const zoomed = ([longitude]: [number, number]) => ({ x: (longitude - 35) * 30, y: 100 });
    expect(placePlateTribeLabels(labels, zoomed, 800, 200)).toHaveLength(2);
  });
});

describe("the stored conic's own forward (client-side reads of the artifact)", () => {
  it("keeps the central meridian's plate pixel reading on lon0 (the artifact's own identity)", () => {
    // A pixel ON the central meridian line (below the apex) reads lon0,
    // and its latitude reads the artifact's own intercept minus its slope.
    const [lon, lat] = plateForwardLonLat(artifactTransform, artifactTransform.apexX, 3500);
    expect(lon).toBeCloseTo(45.5126, 6);
    expect(lat).toBeCloseTo(artifactTransform.latC0 * (3500 - artifactTransform.apexY) + artifactTransform.latC1, 6);
  });

  it("projects the frame's corners to a peninsula-sized WGS84 bbox — a reference frame, never a claim", () => {
    const [west, south, east, north] = plateSourceBounds(artifactTransform, artifactFrame);
    expect(north).toBeGreaterThan(36);
    expect(north).toBeLessThan(39);
    expect(south).toBeLessThan(14);
    expect(south).toBeGreaterThan(8);
    expect(west).toBeGreaterThan(22);
    expect(east).toBeLessThan(65);
  });

  it("builds a closed five-point neatline ring that names its plate", () => {
    const layer: AtlasPlateLayer = {
      key: "admiralty-1916-tribal",
      title: admiraltyRow.titleAr,
      needsReview: true,
      opacity: 0.65,
      transform: artifactTransform,
      frame: artifactFrame,
    };
    const ring = plateNeatlineRing(layer);
    expect(ring.properties?.plateKey).toBe("admiralty-1916-tribal");
    expect(ring.properties?.needsReview).toBe(true);
    const coordinates = ring.geometry.coordinates[0] as Array<[number, number]>;
    expect(coordinates).toHaveLength(5);
    expect(coordinates[0]).toEqual(coordinates[coordinates.length - 1]);
  });
});

describe("ringContainsPoint (the compare panel's dira read, plan 032-C)", () => {
  const square: Array<[number, number]> = [
    [0, 0],
    [10, 0],
    [10, 10],
    [0, 10],
  ];

  it("reads interior and exterior points correctly", () => {
    expect(ringContainsPoint(square, 5, 5)).toBe(true);
    expect(ringContainsPoint(square, 15, 5)).toBe(false);
  });

  it("a point inside the ring's own notch is truthfully outside", () => {
    const cShape: Array<[number, number]> = [[0, 0], [10, 0], [10, 10], [0, 10], [0, 3], [4, 3], [4, 7], [0, 7], [0, 10]];
    expect(ringContainsPoint(cShape, 2, 5)).toBe(false);
    expect(ringContainsPoint(cShape, 8, 5)).toBe(true);
  });

  it("a closed ring whose last vertex repeats the first stays readable", () => {
    const closedSquare: Array<[number, number]> = [...square, square[0]];
    expect(ringContainsPoint(closedSquare, 9, 9)).toBe(true);
    expect(ringContainsPoint(closedSquare, -1, 5)).toBe(false);
  });
});

describe("relation arcs (plan 032-E)", () => {
  const anchoredRelation = (id: string, predicate: string, subject: { longitude: number | null; latitude: number | null }, object: { longitude: number | null; latitude: number | null }) => ({
    id,
    predicate,
    status: "documented",
    subject: { entityType: "tribe", entityId: "s-" + id, label: "الجانب الأول", longitude: subject.longitude, latitude: subject.latitude },
    object: { entityType: "tribe", entityId: "o-" + id, label: "الجانب الثاني", longitude: object.longitude, latitude: object.latitude },
    validFrom: "",
    validTo: "",
  });

  it("draws only arcs whose BOTH ends anchored; unanchored and coincident draw nothing", () => {
    const collection = relationArcCollection([
      anchoredRelation("arc-1", "alliance", { longitude: 41, latitude: 31 }, { longitude: 39.5, latitude: 29.5 }),
      anchoredRelation("arc-2", "raid", { longitude: 41, latitude: 31 }, { longitude: null, latitude: null }),
      anchoredRelation("arc-3", "marriage", { longitude: 41, latitude: 31 }, { longitude: 41, latitude: 31 }),
    ]);
    expect(collection.features).toHaveLength(1);
    expect(collection.features[0].properties?.predicate).toBe("alliance");
    expect(collection.features[0].geometry.coordinates).toHaveLength(2);
  });

  it("carries the arrow's bearing toward the object — the record's own direction", () => {
    const collection = relationArcCollection([anchoredRelation("arc-4", "raid", { longitude: 44, latitude: 20 }, { longitude: 46, latitude: 24 })]);
    expect(collection.features).toHaveLength(1);
    const bearing = Number((collection.features[0].properties as { bearing: number }).bearing);
    expect(bearing).toBeCloseTo((Math.atan2(2, 4) * 180) / Math.PI, 6);
  });

  it("labels travel with the arc (the inspector reads back the recorded names)", () => {
    const collection = relationArcCollection([anchoredRelation("arc-5", "marriage", { longitude: 41, latitude: 31 }, { longitude: 42, latitude: 32 })]);
    const properties = collection.features[0].properties as { subjectLabel: string; objectLabel: string };
    expect(properties.subjectLabel).toBe("الجانب الأول");
    expect(properties.objectLabel).toBe("الجانب الثاني");
  });
});

describe("movement events + corridors (plan 033-A/B/C/D)", () => {
  it("the kind label reads the classical forms and refuses an invention", async () => {
    const { movementEventKindLabel, movementEventUnclassifiedLabel, movementModes, movementSeasonLabels } = await import("./movement-events");
    expect(movementEventKindLabel("ghazwa")).toBe("غزوة");
    expect(movementEventKindLabel("nafir")).toBe("نفير");
    expect(movementEventKindLabel(undefined)).toBe(movementEventUnclassifiedLabel);
    expect(movementEventKindLabel("radar_ping")).toBe(movementEventUnclassifiedLabel);
    expect(movementModes).toEqual(["camel", "horse", "foot", "sea"]);
    expect(movementSeasonLabels.summer).toBe("صيف");
  });

  it("the corridor's geometry is its own readable stops, ordered", async () => {
    const { corridorCollection } = await import("./movement-events");
    const collection = corridorCollection([
      {
        id: "corr-1",
        titleAr: "مرحلة الصيف",
        season: "summer",
        status: "interpreted",
        certainty: "approximate",
        stops: [
          { sequence: 2, placeId: "p2", placeName: "الرياض", longitude: 46.6, latitude: 24.7, dayCountFromPrev: 18, modeFromPrev: "camel" },
          { sequence: 1, placeId: "p1", placeName: "العلا", longitude: 38.1, latitude: 26.6, dayCountFromPrev: undefined, modeFromPrev: "" },
        ],
      },
    ]);
    expect(collection.features).toHaveLength(1);
    const sole = collection.features[0];
    // The stops' ORDER is the corridor's own: sequence one = the first place,
    // whatever order the rows arrived in.
    expect(sole.geometry.coordinates[0][0]).toBeCloseTo(38.1, 6);
    expect(sole.geometry.coordinates[1][0]).toBeCloseTo(46.6, 6);
    expect(sole.properties?.totalDays).toBe(18);
    expect(sole.properties?.firstMode).toBe("camel");
  });

  it("a corridor with one readable stop draws nothing (a route needs two)", async () => {
    const { corridorCollection } = await import("./movement-events");
    const collection = corridorCollection([
      {
        id: "corr-2",
        titleAr: "مهجور",
        status: "interpreted",
        certainty: "approximate",
        stops: [{ sequence: 1, placeId: "p1", placeName: "العلا", longitude: 38.1, latitude: 26.6, modeFromPrev: "" }],
      },
    ]);
    expect(collection.features).toHaveLength(0);
  });
});
