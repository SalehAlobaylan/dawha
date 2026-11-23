# مصادر الجغرافيا القبلية — Tribal Territory External Sources

**Status:** research & integrated plates (updated 2026-11-23) · **Owner:** research · **Related:** `docs/external-data-sources.md`, `plans/028-external-data-leverage.md`, `plans/029-geo-atlas-corrections.md`

نطاق هذا التوثيق: مصادر خارجية مفتوحة أو قابلة للتحقق عن **قبائل الجزيرة العربية وسواها** (حرب، شمّر، عنزة، قحطان، مطير، ربيعة/بكر بن وائل…) وكيفية تحويلها إلى `places` + `historical_regions` + `geographic_associations` وفق قواعد المنصة (كل مستخلص خارجي يهبط بحالة `platform_inferred` حتى يُراجع).

---

## Integrated georeferenced plates & GIS layers (2026-11-23)

All plates below are stored locally in `.data/source-storage/sources/plates/` with verified SHA-256 checksums, held-out georeferencing checks (`infra/local/fit-atlas-affine.py` or `build-plate-georef.py`), normalized Arabic `plate_captions`, and cross-source indexing (`db/seeds/010_additional_atlas_plates.sql`, `db/seeds/011_ottoman_harita_1910_sources.sql`):

| Plate / Layer key | Period | Coverage | License | Registration RMS / Max | Normalized captions / polygons |
|---|---|---|---|---|---|
| `arabia-600ce-tribes` (`e089`) | c. 600 CE | Arabian Peninsula | CC BY-SA 4.0 (Michel Bakni / Commons) | 37.1 km / 66.0 km | 45 Arabic captions |
| `berghaus-1835-arabia` (`e006`) | 1835 | Arabian Peninsula & Red Sea (Gotha) | Public Domain | 45.7 km / 60.2 km | 24 captions |
| `ottoman-harita-93677` (`e096`) | c. 1910 | Najd, Hejaz, Al-Hasa, Qatar, Kuwait | Academic GIS (`opengulf/ottoman-map`, *JOTSA* 2022) | WGS84 QGIS shapefiles | 36 vector polygons + 32 Ottoman census witnesses |
| `dickson-1914-tribal` (`e005`) | 1914 | Arabian Peninsula | CC BY-SA 3.0 | 39.7 km / 62.9 km | 92 captions + 91 digitized dotted borders |
| `admiralty-1916-tribal` (`e004`) | 1916 | Arabian Peninsula | Public Domain | 8.6 km conic / 17.4 km towns | 67 captions |
| `admiralty-1916-districts` (`e007`) | 1916 | Arabian Peninsula (Districts & Towns) | Public Domain | 35.4 km / 45.4 km | 34 captions |
| `ottoman-asir-pre1919` (`e008`) | pre-1919 | Asir & Tihama (Ottoman Arabic script) | Public Domain | 20.4 km / 30.4 km | 22 Ottoman-script captions |
| `iraq-1930-hashimi` (`e009`) | 1930 | Central & Southern Iraq (Taha al-Hashimi) | CC0 | 26.4 km / 43.7 km | 25 Arabic captions |
| `thomas-1931-rub-al-khali` (`e010`) | 1930–31 | Rub' al-Khali, Oman, Dhofar, UAE, Qatar (RGS) | Public Domain | 13.4 km / 24.4 km | 28 captions |
| `yemen-south-arabia-tribes` (`e011`) | Regional ref. | Yemen, Najran, Asir, Hadhramaut, Mahrah | CC0 | 32.1 km / 41.2 km | 48 Arabic captions |

Also normalized from `opengulf/ottoman-map`: 11 regional/settlement witnesses from Ibrahim Fasih al-Haydari's *'Unwan al-Majd* (1869, `e097`).

---

## Arabic maps and archival comparisons checked 2026-11-21

The [normalization and integration design](atlas-source-normalization.md) traces six names from figure 6 into Dawha's caption catalogue and public dossiers, with a proposed import and review workflow for the sources below.

For the first comparison, open al-Qahtani's map and Dickson's 1929 sketch side by side at the Kuwait–al-Ahsa coast. Both name مطير، العوازم، العجمان. Record where each name appears, the period it describes, and the kind of feature the map depicts. Al-Qahtani's printed outline encloses the study region. It supplies no individual boundary for those three tribes. His source note cites documents used in the study and Hamad al-Jasser. The degree of independence between the two maps remains unestablished.

### Arabic lettering on the map

| Coverage and time represented | What the source provides | Locator and access |
|---|---|---|
| Eastern Arabia, 1871–1913 | حمد محمد القحطاني, *الأوضاع الاقتصادية والاجتماعية في إقليم الأحساء*, Kuwait University, 2010. Figure 6 prints العوازم، مطير، العجمان، بنو هاجر، بنو خالد، آل مرة. It has graticule ticks and a scale. Use the printed forms and their approximate positions. | [PDF p. 108, figure 6 and source note 9](https://www.alhasa-turath.org/wp-content/uploads/2020/10/الأوضاع-الاقتصادية-والاجتماعية-في-إقليم-الأحساء-١٢٨٨ـ١٣٣١-هـ-١٨٧١ـ١٩١٣-م.pdf#page=108). The publication reserves rights to the university center. Link to the page for review. |
| Saudi Arabia, undated teaching map | بسمة الرحيلي, *جغرافية المملكة العربية السعودية*. The color overview prints شمر، حرب، مطير، عتيبة and others. The text on the same page describes Najd, with other regions on the next page. The figure lacks a dated survey or cartographer credit. | [Umm al-Qura University PDF, p. 35](https://drive.uqu.edu.sa/_/gmqurbeh/files/Gegr%20Of%20KSA%20Intisab.pdf#page=35). Use for finding names. Reuse permission for the embedded figure is unverified. |
| Arabian Peninsula, early twentieth-century travel | أمين الريحاني, *ملوك العرب*, introduction. Peninsula-wide Arabic map with settlements and travel routes. It provides place-name context for tribal captions. The heavy route lines are identified in its legend. | [Hindawi edition, PDF p. 20](https://downloads.hindawi.org/books/83158373.pdf#page=20). A separate [archived map from تاريخ نجد, f. 258r](https://www.qdl.qa/en/archive/81055/vdc_100122625235.0x000079) depicts Ibn Saud's frontier in 1926. |
| Asir highlands, reconstruction of historical geography | *القول المكتوب في تاريخ الجنوب*, vol. 20, study on عنز بن وائل. The author credits the drawing to himself and explains his reading of al-Hamdani immediately above it. The sketch relates سراة عنز بن وائل to سراة جنب، رجال الحجر and شهران. | [Author's map, PDF p. 8, printed p. 502](https://prof-ghithan.com/wp-content/uploads/2022/03/312.pdf#page=8). Use for named relationships and consult the passage for the author's interpretation. |
| Oman and the Gulf, early Islamic period | *مجلة العرب*, appendix to the article on بني سامة. The figure uses numbers keyed to eight tribal names beneath it. The article studies the late ninth and early tenth centuries CE. | [Hamad al-Jasser website, PDF p. 32, printed p. 609](https://hamadaljasser.com/wp-content/uploads/2024/01/ربيع-الآخر-جماى-الآخرة-1445هـ.pdf#page=32). Reference for the early Islamic atlas. |
| Arabian Peninsula, circa 600 CE | Arabic SVG by Michel Bakni and subsequent contributors. The file cites حسين مؤنس, *أطلس تاريخ الإسلام*, pp. 54–55, and محمد حسين هيكل, *الصديق أبو بكر*, pp. 388–390. Revision history records corrections to names. | [Source file, credits, revisions and CC BY-SA 4.0 terms](https://commons.wikimedia.org/wiki/File:Map_of_Arabia_600_AD-ar.svg). Preserve attribution, license and revision when adapting this early-period map. |

### Arabic print reference to locate

The fifth volume of *البدو*, published by دار الوراق, contains genealogies and geographic maps of tribal distribution. The Arabic edition credits **ماجد شبر** with map preparation. Volume 3 covers northern and central Arabia and southern Iraq. The catalogue and contents establish the existence of the map supplement. A full public scan of the fifth volume and terms for its maps remain unverified. [Library catalogue](https://www.drabusulaimanlibrary.com/bib/3001) · [Five-volume contents](https://torathyah.com/البدو---ماكس-فرايهير-فون-أوبنهايم/p1272469955)

### Dated comparisons and Arabic spellings

| Region | Source and locator | What to compare |
|---|---|---|
| Kuwait–al-Ahsa coast | [Dickson report sketch, 3 August 1929, British Library IOR/R/15/1/586, f. 228](https://www.qdl.qa/en/archive/81055/vdc_100023488853.0x000001) | English archival sheet locating مطير، العوازم، العجمان. The red line marks the old Kuwait boundary. QDL provides IIIF access and marks this item public domain. |
| Eastern Arabia and the Empty Quarter | [Dickson sketch, 1935, British Library IOR/R/15/1/605, f. 195B](https://www.qdl.qa/en/archive/81055/vdc_100048679713.0x000002) | An English sketch of the approximate dīra of آل مرة compiled from Bedouin sources. The red route belongs to Bertram Thomas. QDL lists both a public-domain entry and an Open Government Licence entry for the folio. |
| Northern Arabia and Najd | [Musil's 1926 map set, Amman Institute Library](https://digitalrepository.cbrl.ac.uk/musil-northern-arabia-1926-american-geographic-society-maps) · [Eran Laor Map Collection copy](https://www.nli.org.il/en/maps/NNL_ALEPH990027073430205171/NLI) | Companion cartography for *Northern Neǧd* and three other volumes. Use to locate routes and localities. The Amman copy specifies noncommercial use. The Laor record marks its copy public domain and requests credit. |
| مطير and the reading of BARRIYAH | فؤاد حمزة, *قلب جزيرة العرب*, Mecca, 1933. [First-edition scan, printed p. 192](https://sites.dlib.nyu.edu/viewer/books/aub_aco001825/218?lang=ar) · [p. 193](https://sites.dlib.nyu.edu/viewer/books/aub_aco001825/219?lang=ar) | حمزة prints **بريه** in his entry for مطير. The following page lists branches under that heading. Cite the printed spelling alongside the atlas reading **برية**. This gives a separately authored Arabic reference for the name. |

### Public social posts traced to the image or broadcast

| Post | What can be checked |
|---|---|
| [Ottoman2Archive on X, 17 May 2025](https://x.com/Ottoman2Archive/status/1923658725685973069) | The [Tabuk image](https://pbs.twimg.com/media/GrI1aeAX0AARzQW.jpg?name=orig) visibly pairs تبوك with بني عطية and a historical spelling for الحويطات. The [Medina image](https://pbs.twimg.com/media/GrI1aeAX0AEbeZh.jpg?name=orig) places عشائر حرب beside the Medina–Yanbu area. These are two crops of a reported Ottoman sheet. The post date is known. The sheet's date, complete legend, language of the full sheet and archival call number are unverified. |
| [Rotana Khalijia, في الصورة, December 2021, 33:10](https://www.youtube.com/watch?v=rT1EUgxsf-Y&t=1990s) | تركي بن طلال explains Asir's terrain and names tribes on the map. Use as a dated spoken regional account. [Contemporaneous reporting](https://al-hadath.com/local-news/19552/الأمير-تركي-بن-طلال-يستعرض-خريطة-منطقة-عسير-ويعدد-أسماء-القبائل-التي-تسكنها) |
| [Arabic map discussion on r/saudiarabia](https://www.reddit.com/r/saudiarabia/comments/1tktztc/) | The post and replies flag disputed placements and overlapping ranges. The original maker, survey date and rights are unverified. Use the objections as questions for checking against a dated source. |

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
- Musil, *Arabia Petraea* (1907–08 — DLI scans present, incl. vol. 3 ethnographic) and *Manners and Customs of the Rwala Bedouins* (1928, archive.org scan present); *Northern Negd* with its map-atlas remains a **lead, not yet located** on archive.org.
- Admiralty **Handbook of Arabia** Vol. I General (1920; `handbkarabiavol1`) — tribal chapter text.
- Peake Pasha, *A History of Trans-Jordan and its Tribes* (1934; McGill scans vol 1+2) — map fold-outs; **licensing unresolved (author died 1970, so UK life+70 has not expired)** — research reference only, never republication.

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
