# مصادر الجغرافيا القبلية — Tribal Territory External Sources

**Status:** research (live-verified 2026-11-13) · **Owner:** research · **Related:** `docs/external-data-sources.md`, `plans/028-external-data-leverage.md`, `plans/029-geo-atlas-corrections.md`

نطاق هذا التوثيق: مصادر خارجية مفتوحة أو قابلة للتحقق عن **قبائل الجزيرة العربية وسواها** (حرب، شمّر، عنزة، قحطان، مطير، ربيعة/بكر بن وائل…) وكيفية تحويلها إلى `places` + `historical_regions` + `geographic_associations` وفق قواعد المنصة (كل مستخلص خارجي يهبط بحالة `platform_inferred` حتى يُراجع).

---

## 1. What was live-verified (not just cited)

| Check | Result |
|---|---|
| Wikidata `P31 = Q133311` (tribe) | **2,150 tribe items** globally |
| Wikidata tribes tied to an Arab country (`P17`) | **110 items** |
| Wikidata tribes with any `P131` located-in | **147 items** |
| Arabic Wikipedia category `تصنيف:قبائل عربية` | **86 subcategories** (per-country, per-tribe trees incl. `قبيلة حرب`, `قبيلة شمر`, `قبيلة عتيبة`, `قبيلة مطير`) |
| Wikimedia Commons tribal maps | 5+ freely licensed maps downloaded & vision-read (§4) |
| archive.org scans of Lorimer / Admiralty Handbook / Peake | confirmed present (§2.2) |

**Analytic conclusion:** structured (Wikidata) coverage carries names but almost no territory; territory must be mined from *maps* (raster, PD-era) and *text* (gazetteers, handbooks, classical genealogies) and reconciled through the review pipeline.

---

## 2. Source inventory (all URL-verified)

### 2.1 Maps — the territory carriers

| Asset | License | Resolution | Value |
|---|---|---|---|
| Admiralty War Staff, «Tribal Map of Arabia» (1916) | Public domain (Crown, pre-1957) | 7163×7170 | Core plate: Ḥarb, ʿAteibah, Shammar, Qaḥṭān, Mutayr, ʿAnazah, Dhafeer, Muntefiq, Ḥawāzin… Positioned relative to Ḥāʾil/Boreidah/Hofuf. |
| H.R.P. Dickson, «Rough Tribal Map of Arabia» (ca. 1930s) | CC0 | ~1572×1761 | Fine-dotted *internal tribal boundaries* (rare!) incl. ʿAnizah's Dhana-Muslim vs Bishr split; sections noted (Bani Ḥajjāz? Muṭayr al-ʿIlwa/Burayth variants). |
| Wikimedians' modern Saudi tribal map | CC BY-SA 4.0 | 1170×985 | Modern community outline — secondary corroboration only, never sole source. |
| Commons Jordanian «خارطة العشائر الأردنية» | CC0 | 3855×4096 | Levant-side coverage for Shammar/Rwala/Bani Ṣakhr overlap. |
| Ottoman SW-Arabia tribal map (آل سعد/بللسمر) | Public domain | 858×1068 | Asir/Tihama clan detail. |

**More via IIIF/mirrors** (Cloudflare-protected for bots; use browser or their IIIF endpoints):
- **Gazetteer of Arabia** (War Office geographic handbooks, 7 vols, 1916-1917 — QDL mirror `20201031_20201031_1145` on archive.org, 16.5k downloads already).
- **Gazetteer of the Persian Gulf, Oman and Central Arabia** (Lorimer, 1908/1915 — 4 archive.org scans, DLI + full photo reproduction); Vol. II Geographical & Statistical tribal entries.
- Musil, *Arabia Petraea*, Vol. III + *Northern Negd* (maps as separate atlases; scans exist).
- Admiralty **Handbook of Arabia** Vol. I General (1920; `handbkarabiavol1`) — tribal chapter text.
- Peake Pasha, *A History of Trans-Jordan and its Tribes* (1934; McGill scans vol 1+2) — map fold-outs, caution ©-ambiguous pre-1930 (UK term of author d. 1970 → PD in UK; US renewal unverified).

### 2.2 Texts

| Source | Status | Dawha landing table |
|---|---|---|
| Yāqūt, *Muʿjam al-Buldān* (Wüstenfeld-scale scans + OpenITI) | Open texts | `historical_place_names` (وروده بأسماء عربية) |
| al-Hamdānī, *Ṣifat Jazīrat al-ʿArab*; *al-Iklīl* | critical-edition scans on QDL/archive | claim passages for clan territories |
| Ibn Ḥazm, *Jamharat Ansāb al-ʿArab* (Lévi-Provençal ed. scan) | archive.org `alfirdwsiy2018_gmail_…` | `branches` lineage skeleton |
| al-Balādhurī, *Ansāb al-Ashrāf* (OpenITI has it) | OpenITI corpus | same |
| EI1 (pre-1929 articles: «Badw», tribal entries) | US-PD portions scanned; the 1987 Brill reprint of *EI1* is © — **quote-scan the scan, not the reprint** | reference layer |

### 2.3 Name-authority layer

- **Wikidata** is a *label index*, not geography: of 2,150 tribe items only ~110 claim `P17` (Arab countries), ~147 have `P131`. Use for canonical names/aliases/descriptions; harvest Arabic aliases via `build-name-variants.py` extension (P1705/`aliases`+`MUL` labels), then match against ar-wiki page titles.

### 2.4 LLM-vision extraction (demo RESULTS)

| Map (source) | Extracted (sample) | Notes |
|---|---|---|
| Admiralty 1916 tribal map | Ḥarb between Medina & Najd; Shammar around Ḥāʾil; Qaḥṭān S. of Riyadh; Ateibah E/S of Mecca; Mutayr E of Qasim; ʿAnazah N of Qasim into Iraq/Syria; Dhafeer E of Qasim→Kuwait; Muntefiq + Bani Lam N. of Basra; Ajman ‘E. of Riyadh (Ḥasā steppe)’; Bani Khālid along Gulf littoral | anchors = towns (Hail, Boreidah, Hofuf…) → approximate region; adequate seed for `historical_regions` polygons |
| Dickson rough tribal map | boundary dots + subtribe sections («ʿAnizah: Dhana Muslim vs Bishr», «Ḥarb: sections») | richest boundary depiction available FOC |
| 2011 Saudi community map (CC BY-SA 4.0) | 40+ labeled polygons incl. Qaḥṭān, ʿUtayba, Subayʿ, ʿAnizah, Mutayr, Bani Khālid, ʿAjamān (العجمان) | modern check for 2011 footing |
| Jamhara-lineage maps (Arabic wikimedia `شجرة نسب قبائل يام…`) | CC0 lineage charts convert directly into `branches` | text-parseable start |

**Confidence for each derived polygon must be `approximate`; via review only then `documented`.**

---

## 3. Gap list (what is still missing)

1. No machine-readable polygon **shapes** come from these maps — regions must be georeferenced (control-point registration, e.g. via Allmaps/MapWarper export) to derive GeoJSON.
2. Manipulating 1934 Peake fold-out maps at full size is constrained by scan quality — flag if we need Cormorant-grade zoomable IIIF.
3. Tribal *residence* vs *raid* range conflation in colonial plates (Dickson warns about this in text—he shows where they trebled). Encode as `relation_type: associated_with` not `lived_in` unless classical passage exists.

## 4. Next actions

1. Add `docs/external-data-sources.md` cross-link to this doc.
2. Extend `infra/local/build-name-variants.py` to pull tribe items from Wikidata SPARQL `P31 Q133311 ∩ P17(SetArabCountries)`.
3. Georeference the 1916 Admiralty plate (Allmaps → GeoJSON) → seed initial `historical_regions` with `certainty='approximate'`.
4. Run the vision pipeline (`services/ai-research/app/chart_vision.py` adapter, generalized beyond family charts) at tile level on each Dickson/Admiralty plate tile; each polygon becomes a `platform_inferred` claim awaiting review.
