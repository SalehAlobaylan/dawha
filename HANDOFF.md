# Handoff — دَوْحة (Dawha) Atlas & Spatial Workbench

**Date:** 2026-11-23
**Repo:** `~/Desktop/MyRepos/dawha`

## 1. Current Atlas Surface (`/places`)

The historical atlas renders **9 georeferenced archival & regional map plates** plus an interactive **Ottoman Harita 93677 (c. 1910)** vector boundary & census overlay, alongside Dawha's documented places, migrations, corridors, relations, and approximate historical regions:

| Plate / Layer | Era | Coverage | Captions / Polygons |
|---|---|---|---|
| `arabia-600ce-tribes` (`e089`) | c. 600 CE | Arabian Peninsula | 45 Arabic captions |
| `berghaus-1835-arabia` (`e006`) | 1835 | Arabian Peninsula & Red Sea | 24 captions |
| `ottoman-harita-93677` (`e096`) | c. 1910 | Najd, Hejaz, Al-Hasa, Qatar, Kuwait | 36 WGS84 polygons + 32 Ottoman census witnesses |
| `dickson-1914-tribal` (`e005`) | 1914 | Arabian Peninsula | 92 captions + 91 digitized dotted tribal borders |
| `admiralty-1916-tribal` (`e004`) | 1916 | Arabian Peninsula | 67 captions |
| `admiralty-1916-districts` (`e007`) | 1916 | Arabian Peninsula (Districts & Towns) | 34 captions |
| `ottoman-asir-pre1919` (`e008`) | pre-1919 | Asir & Tihama (Ottoman Arabic script) | 22 Ottoman-script captions |
| `iraq-1930-hashimi` (`e009`) | 1930 | Central & Southern Iraq | 25 Arabic captions |
| `thomas-1931-rub-al-khali` (`e010`) | 1930–31 | Rub' al-Khali, Oman, Dhofar, UAE, Qatar | 28 captions |
| `yemen-south-arabia-tribes` (`e011`) | Regional ref. | Yemen, Najran, Asir, Hadhramaut, Mahrah | 48 Arabic captions |

### Key UX & Architecture Highlights
- **Interactive full-bleed `/places` workspace:** Collapsible filters and inspector rails, fullscreen canvas (`Esc` exits), toggleable legend, floating `طبقات العرض` layer menu, curtain/swipe plate comparison, point-comparison across plates, and a collapsible Map Directory (`الإشارات في الإطار`) with a `قبائل اللوحة` tab.
- **Click-first cross-source discovery:** Clicking any tribe label, Dickson dotted territory, Ottoman 1910 polygon, or place/region opens the docked inspector (`PlateCaptionPanel`, `OttomanHaritaPanel`, or `AtlasInspector`) with scan crops, dossier links, and automatic cross-source witnesses (`في مصادر أخرى`). Moderator proposal review lives on `/region-workbench`.
- **Register separation:** Historical plates, Dickson's fine dotted boundaries, and Ottoman Harita 93677 polygons stay in their own archival visual registers and never overwrite reviewed `historical_regions`.

## 2. Reproducing Local Plates & Seeds

```bash
# 1. Verify or download the 7 additional archival plates into .data/source-storage/sources/plates/
python3 infra/local/fetch-atlas-plates.py --verify-only

# 2. Apply base plate georefs (Admiralty 1916 + Dickson 1914) and numbered seeds (002–011)
docker exec -i dawha-db-1 psql -U dawha -d dawha < infra/local/seed-plate-georefs.sql
make db-seed
```

## 3. Environment Contract

| Var | Value |
|---|---|
| `DATABASE_URL` | `postgres://dawha:dawha_local@localhost:55432/dawha?sslmode=disable` |
| `SOURCE_STORAGE_DIR` | Absolute writable path (`.data/source-storage` in dev, `/tmp/dawha-e2e-storage` in e2e) |
| `AI_RESEARCH_URL` | `http://localhost:8000` in dev, `http://localhost:8182` in e2e |
| `CORE_API_PORT` | `8080` in dev, `8181` in e2e |
