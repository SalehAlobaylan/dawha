-- Seed 010: Seven additional georeferenced archival & regional historical plates
-- covering 600 CE, 1835, 1916 (Districts), pre-1919 (Ottoman Asir), 1930 (Iraq),
-- 1930-31 (Bertram Thomas Rub' al-Khali / Oman), and Yemen & South Arabia (CC0).
-- All affine registrations passed held-out landmark checks via infra/local/fit-atlas-affine.py.
-- Idempotent: safe to re-run.

BEGIN;

-- arabia-600ce-tribes (45 captions)
INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from, citation_ar, location_ar, visibility, metadata)
VALUES (
  'b0000000-0000-0000-0000-00000000e089',
  'مواطن القبائل في الجزيرة العربية نحو سنة ٦٠٠م',
  'ميشيل بكّني ومساهمو ويكيميديا كومنز', 'other', '2020-06-15',
  'تعريب خريطة Map of Arabia 600 AD (عن حسين مؤنس، أطلس تاريخ الإسلام ص ٥٤–٥٥، ومحمد حسين هيكل، الصديق أبو بكر) · CC BY-SA 4.0',
  'الجزيرة العربية', 'public',
  $json${"license": "cc_by_sa_4_0", "license_note": "Wikimedia Commons File:Map_of_Arabia_600_AD-ar.svg rendered to raster under CC BY-SA 4.0; attribution to Michel Bakni and contributors required.", "archive_title": "File:Map of Arabia 600 AD-ar.svg", "fetched_at": "2026-11-23", "file_sha256": "1ba4b92f1f97280a0849c468a5cfcf3030dead14ca3d280b513fe537513a1aa6", "plate_key": "arabia-600ce-tribes", "plate_language": "ar", "depicted_from_year": 600, "depicted_to_year": 600, "file_bytes": 906327, "generator": "infra/local/fit-atlas-affine.py", "georef": {"method": "control_points", "reason": "", "model": {"kind": "affine", "affine": [0.007574258785380416, 0.0003069363912036909, 32.21036937741917, 0.00012930852385485323, -0.006763247589478697, 33.314932487930875]}, "frame": {"outer_left": 10.0, "inner_left": 10.0, "outer_right": 3830.0, "inner_right": 3830.0, "outer_top": 10.0, "inner_top": 10.0, "outer_bottom": 3123.0, "inner_bottom": 3123.0}, "fit": {"lonRmsDeg": 0.27082043377222764, "latRmsDeg": 0.22457874303000783, "lonMaxDeg": 0.6058989788183169, "latMaxDeg": 0.355152609222408, "heldOutCount": 6, "maxErrorKm": 65.95313435370474, "maxAllowedKm": 95.0, "rmsKm": 37.14275313567254, "coverage": {"fit": {"xSpanFraction": 0.9219895287958115, "ySpanFraction": 0.8294249919691615}, "check": {"xSpanFraction": 0.6424083769633507, "ySpanFraction": 0.63989720526823}}, "intendedScale": "peninsula_overview"}, "certainty": "approximate", "needsReview": true, "reviewReasons": ["landmark_based_registration"]}}$json$::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  title_ar = EXCLUDED.title_ar, author_ar = EXCLUDED.author_ar,
  citation_ar = EXCLUDED.citation_ar, location_ar = EXCLUDED.location_ar,
  metadata = EXCLUDED.metadata, updated_at = now();

INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
SELECT 'b0000000-0000-0000-0000-00000000e089', 'sources/plates/arabia-600ce-tribes.png', 'arabia-600ce-tribes.png',
       'image/png', 906327, '1ba4b92f1f97280a0849c468a5cfcf3030dead14ca3d280b513fe537513a1aa6'
WHERE NOT EXISTS (SELECT 1 FROM source_files WHERE storage_key = 'sources/plates/arabia-600ce-tribes.png');

INSERT INTO plate_captions (source_id, scan_sha256, x, y, printed_name, name_ar, tribe_id)
SELECT
  'b0000000-0000-0000-0000-00000000e089'::uuid, '1ba4b92f1f97280a0849c468a5cfcf3030dead14ca3d280b513fe537513a1aa6',
  v.x, v.y, v.printed_name, v.name_ar, t.id
FROM (VALUES
  (872, 1824, 'قريش', 'قريش', NULL::uuid),
  (1200, 1848, 'هوازن', 'هوازن', 'b0000000-0000-0000-0000-c00000000008'::uuid),
  (1660, 916, 'تميم', 'تميم', 'b0000000-0000-0000-0000-c00000000004'::uuid),
  (1844, 712, 'بكر بن وائل', 'بكر بن وائل', NULL::uuid),
  (1192, 240, 'تغلب', 'تغلب', NULL::uuid),
  (1156, 696, 'طيء', 'طيء', NULL::uuid),
  (956, 1236, 'غطفان', 'غطفان', NULL::uuid),
  (1140, 1344, 'أسد', 'أسد', NULL::uuid),
  (1386, 1060, 'عبس', 'عبس', NULL::uuid),
  (1152, 1528, 'سليم', 'سليم', NULL::uuid),
  (724, 1368, 'جهينة', 'جهينة', NULL::uuid),
  (604, 1118, 'بلي', 'بلي', NULL::uuid),
  (724, 948, 'عذرة', 'عذرة', NULL::uuid),
  (494, 728, 'جذام', 'جذام', NULL::uuid),
  (460, 496, 'بهراء', 'بهراء', NULL::uuid),
  (968, 464, 'كلب', 'كلب', NULL::uuid),
  (1056, 112, 'عنيزة', 'عنزة', 'b0000000-0000-0000-0000-c00000000003'::uuid),
  (628, 124, 'الغساسنة', 'الغساسنة', NULL::uuid),
  (1768, 160, 'لخم (المناذرة)', 'لخم (المناذرة)', NULL::uuid),
  (2114, 1196, 'عبد القيس', 'عبد القيس', NULL::uuid),
  (1660, 1368, 'حنيفة', 'حنيفة', NULL::uuid),
  (1844, 1554, 'ضبة', 'ضبة', NULL::uuid),
  (1414, 1246, 'نمير', 'نمير', NULL::uuid),
  (1288, 1664, 'كلاب', 'كلاب', NULL::uuid),
  (1616, 1664, 'باهلة', 'باهلة', NULL::uuid),
  (2028, 1736, 'كعب', 'كعب', NULL::uuid),
  (1480, 1940, 'هلال', 'هلال', NULL::uuid),
  (860, 1620, 'خزاعة', 'خزاعة', NULL::uuid),
  (976, 1696, 'هذيل', 'هذيل', NULL::uuid),
  (994, 1928, 'ثقيف', 'ثقيف', NULL::uuid),
  (1062, 2056, 'كنانة', 'كنانة', NULL::uuid),
  (1140, 2132, 'دوس', 'دوس', NULL::uuid),
  (1246, 2248, 'أزد السراة', 'أزد السراة', NULL::uuid),
  (1382, 2380, 'خثعم', 'خثعم', NULL::uuid),
  (1504, 2268, 'مذحج', 'مذحج', NULL::uuid),
  (1884, 2248, 'أذر السراة', 'أزد السراة (الجنوب)', NULL::uuid),
  (1704, 2468, 'الحارث', 'بنو الحارث', NULL::uuid),
  (1444, 2588, 'همدان', 'همدان', NULL::uuid),
  (1640, 2704, 'مراد', 'مراد', NULL::uuid),
  (1464, 2816, 'عنس', 'عنس', NULL::uuid),
  (1576, 2912, 'يافع', 'يافع', NULL::uuid),
  (1474, 3008, 'المعافر', 'المعافر', NULL::uuid),
  (2308, 2556, 'كندة', 'كندة', NULL::uuid),
  (2716, 2348, 'مهرة', 'مهرة', NULL::uuid),
  (3064, 1700, 'أزد عُمان', 'أزد عُمان', NULL::uuid)
) AS v(x, y, printed_name, name_ar, tribe_id)
LEFT JOIN tribes t ON t.id = v.tribe_id AND t.visibility = 'public'
ON CONFLICT (source_id, scan_sha256, x, y) DO UPDATE SET
  printed_name = EXCLUDED.printed_name, name_ar = EXCLUDED.name_ar,
  tribe_id = COALESCE(plate_captions.tribe_id, EXCLUDED.tribe_id);

-- berghaus-1835-arabia (24 captions)
INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from, citation_ar, location_ar, visibility, metadata)
VALUES (
  'b0000000-0000-0000-0000-00000000e006',
  'خريطة جزيرة العرب وبلاد النيل — هاينريش برغهاوس ١٨٣٥',
  'Heinrich Berghaus (Justus Perthes, Gotha)', 'archive_record', '1835-01-01',
  'Berghaus, H., ''Arabia und das Nil-Land, den Manen Karsten Niebuhr''s...'', Gotha: Justus Perthes, 1835.',
  'Gotha', 'public',
  $json${"license": "public_domain", "license_note": "Published 1835 (Gotha, Justus Perthes); worldwide Public Domain.", "archive_title": "File:خريطة لجزيرة العرب وبلاد النيل.jpg", "fetched_at": "2026-11-23", "file_sha256": "0a1e629807a352062d737deb188b8f5c2c22d63fa356cade2ea8282bd01a66b1", "plate_key": "berghaus-1835-arabia", "plate_language": "de", "depicted_from_year": 1835, "depicted_to_year": 1835, "file_bytes": 3466212, "generator": "infra/local/fit-atlas-affine.py", "georef": {"method": "control_points", "reason": "", "model": {"kind": "affine", "affine": [0.010609833589346274, 0.00033258753856537076, 23.56178887120098, -0.00010149500042969404, -0.009211506473016988, 33.99884686344449]}, "frame": {"outer_left": 272.0, "inner_left": 272.0, "outer_right": 3514.0, "inner_right": 3514.0, "outer_top": 240.0, "inner_top": 240.0, "outer_bottom": 2512.0, "inner_bottom": 2512.0}, "fit": {"lonRmsDeg": 0.35290838200749364, "latRmsDeg": 0.24361694537059855, "lonMaxDeg": 0.5225484060760692, "latMaxDeg": 0.4723051662162234, "heldOutCount": 5, "maxErrorKm": 60.24090138880101, "maxAllowedKm": 80.0, "rmsKm": 45.691580327061175, "coverage": {"fit": {"xSpanFraction": 0.8124614435533621, "ySpanFraction": 0.8970070422535211}, "check": {"xSpanFraction": 0.7217766810610734, "ySpanFraction": 0.7878521126760564}}, "intendedScale": "peninsula_overview"}, "certainty": "approximate", "needsReview": true, "reviewReasons": ["landmark_based_registration"]}}$json$::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  title_ar = EXCLUDED.title_ar, author_ar = EXCLUDED.author_ar,
  citation_ar = EXCLUDED.citation_ar, location_ar = EXCLUDED.location_ar,
  metadata = EXCLUDED.metadata, updated_at = now();

INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
SELECT 'b0000000-0000-0000-0000-00000000e006', 'sources/plates/berghaus-1835-arabia.jpg', 'berghaus-1835-arabia.jpg',
       'image/jpeg', 3466212, '0a1e629807a352062d737deb188b8f5c2c22d63fa356cade2ea8282bd01a66b1'
WHERE NOT EXISTS (SELECT 1 FROM source_files WHERE storage_key = 'sources/plates/berghaus-1835-arabia.jpg');

INSERT INTO plate_captions (source_id, scan_sha256, x, y, printed_name, name_ar, tribe_id)
SELECT
  'b0000000-0000-0000-0000-00000000e006'::uuid, '0a1e629807a352062d737deb188b8f5c2c22d63fa356cade2ea8282bd01a66b1',
  v.x, v.y, v.printed_name, v.name_ar, t.id
FROM (VALUES
  (1846, 1126, 'NEDJED', 'نجد', NULL::uuid),
  (1760, 694, 'EL DJEBEL SCHAMMAR', 'جبل شمّر', 'b0000000-0000-0000-0000-c00000000002'::uuid),
  (1750, 876, 'EL KASSYM', 'القصيم', NULL::uuid),
  (1970, 774, 'EL SEDEIR', 'سدير', NULL::uuid),
  (1998, 976, 'EL AARED', 'العارض', NULL::uuid),
  (2176, 1170, 'EL HARYK', 'الحريق', NULL::uuid),
  (2008, 1618, 'EL AFLADJ', 'الأفلاج', NULL::uuid),
  (1962, 1712, 'WADI DOWASER', 'وادي الدواسر', NULL::uuid),
  (2280, 872, 'EL HASSA', 'الأحساء', NULL::uuid),
  (1918, 1806, 'KACHTAN', 'قحطان', 'b0000000-0000-0000-0000-c0000000000a'::uuid),
  (1902, 1916, 'WADI NEDJRAN', 'وادي نجران', NULL::uuid),
  (1718, 1744, 'ASIER', 'عسير', NULL::uuid),
  (1990, 2004, 'BELAD EL DJOF', 'بلاد الجوف (اليمن)', NULL::uuid),
  (2492, 1924, 'HADRAMAUT', 'حضرموت', NULL::uuid),
  (2998, 1676, 'MAHRA', 'مهرة', NULL::uuid),
  (3108, 1212, 'OMAN', 'عُمان', NULL::uuid),
  (2056, 1344, 'ROBA EL KHALY', 'الربع الخالي', NULL::uuid),
  (2256, 1788, 'AL AHKAF', 'الأحقاف', NULL::uuid),
  (1438, 1368, 'EL HARAM (MEKKA)', 'مكة المكرمة', NULL::uuid),
  (1432, 1004, 'MEDINA', 'المدينة المنورة', NULL::uuid),
  (1584, 424, 'ANEZEH', 'عنزة', 'b0000000-0000-0000-0000-c00000000003'::uuid),
  (1534, 520, 'DJOF', 'دومة الجندل (الجوف)', NULL::uuid),
  (2040, 1008, 'EL DERREYEH', 'الدرعية', NULL::uuid),
  (1306, 956, 'HEDJAZ', 'الحجاز', NULL::uuid)
) AS v(x, y, printed_name, name_ar, tribe_id)
LEFT JOIN tribes t ON t.id = v.tribe_id AND t.visibility = 'public'
ON CONFLICT (source_id, scan_sha256, x, y) DO UPDATE SET
  printed_name = EXCLUDED.printed_name, name_ar = EXCLUDED.name_ar,
  tribe_id = COALESCE(plate_captions.tribe_id, EXCLUDED.tribe_id);

-- admiralty-1916-districts (34 captions)
INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from, citation_ar, location_ar, visibility, metadata)
VALUES (
  'b0000000-0000-0000-0000-00000000e007',
  'خريطة أقاليم ومدن الجزيرة العربية — هيئة الأركان البحرية البريطانية ١٩١٦',
  'Admiralty War Staff, Intelligence Division', 'archive_record', '1916-01-01',
  'Admiralty War Staff, Intelligence Division, ''ARABIA: Districts and Towns'', scale 1:7,500,000, 1916.',
  'London', 'public',
  $json${"license": "public_domain", "license_note": "UK Crown publication pre-1957; US publication pre-1929. Wikimedia Commons Public Domain.", "archive_title": "File:1916 map by the British Admiralty War Staff - ARABIA - Districts and Towns.jpg", "fetched_at": "2026-11-23", "file_sha256": "45a2389ed00ddd50e67fead0db2a605374987a310ffc992f52136d1d72c3cac9", "plate_key": "admiralty-1916-districts", "plate_language": "en", "depicted_from_year": 1916, "depicted_to_year": 1916, "file_bytes": 2473940, "generator": "infra/local/fit-atlas-affine.py", "georef": {"method": "control_points", "reason": "", "model": {"kind": "affine", "affine": [0.009225139250700823, 3.592891471286674e-05, 25.986702149775017, -3.6539705058783076e-05, -0.0082694376808907, 35.771597833378195]}, "frame": {"outer_left": 296.0, "inner_left": 296.0, "outer_right": 3604.0, "inner_right": 3604.0, "outer_top": 188.0, "inner_top": 188.0, "outer_bottom": 2868.0, "inner_bottom": 2868.0}, "fit": {"lonRmsDeg": 0.22281926294150223, "latRmsDeg": 0.24150309178309434, "lonMaxDeg": 0.37258659452462695, "latMaxDeg": 0.37573343923688185, "heldOutCount": 6, "maxErrorKm": 45.42394071614529, "maxAllowedKm": 60.0, "rmsKm": 35.38566367866033, "coverage": {"fit": {"xSpanFraction": 0.937726723095526, "ySpanFraction": 0.9208955223880597}, "check": {"xSpanFraction": 0.5399032648125756, "ySpanFraction": 0.6694029850746268}}, "intendedScale": "peninsula_overview"}, "certainty": "approximate", "needsReview": true, "reviewReasons": ["landmark_based_registration"]}}$json$::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  title_ar = EXCLUDED.title_ar, author_ar = EXCLUDED.author_ar,
  citation_ar = EXCLUDED.citation_ar, location_ar = EXCLUDED.location_ar,
  metadata = EXCLUDED.metadata, updated_at = now();

INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
SELECT 'b0000000-0000-0000-0000-00000000e007', 'sources/plates/admiralty-1916-districts.jpg', 'admiralty-1916-districts.jpg',
       'image/jpeg', 2473940, '45a2389ed00ddd50e67fead0db2a605374987a310ffc992f52136d1d72c3cac9'
WHERE NOT EXISTS (SELECT 1 FROM source_files WHERE storage_key = 'sources/plates/admiralty-1916-districts.jpg');

INSERT INTO plate_captions (source_id, scan_sha256, x, y, printed_name, name_ar, tribe_id)
SELECT
  'b0000000-0000-0000-0000-00000000e007'::uuid, '45a2389ed00ddd50e67fead0db2a605374987a310ffc992f52136d1d72c3cac9',
  v.x, v.y, v.printed_name, v.name_ar, t.id
FROM (VALUES
  (1930, 1294, 'NEJD', 'نجد', NULL::uuid),
  (1696, 1084, 'JEBEL SHAMMAR', 'جبل شمّر', 'b0000000-0000-0000-0000-c00000000002'::uuid),
  (1894, 1146, 'QASIM', 'القصيم', NULL::uuid),
  (2110, 1148, 'SEDEIR', 'سدير', NULL::uuid),
  (2028, 1272, 'WASHM', 'الوشم', NULL::uuid),
  (2152, 1344, 'ARID', 'العارض', NULL::uuid),
  (2240, 1468, 'KHARJ', 'الخرج', NULL::uuid),
  (2128, 1506, 'AFLAJ', 'الأفلاج', NULL::uuid),
  (2010, 1682, 'WIDYAN DAWASIR', 'وادي الدواسر', NULL::uuid),
  (2492, 1266, 'HASA', 'الأحساء', NULL::uuid),
  (2704, 1296, 'EL QATAR', 'قطر', NULL::uuid),
  (2316, 860, 'KOWEIT', 'الكويت', NULL::uuid),
  (1526, 898, 'NEFUD', 'النفود', NULL::uuid),
  (2256, 1076, 'DAHANA', 'الدهناء', NULL::uuid),
  (2364, 1898, 'RUBA'' EL-KHALI', 'الربع الخالي', NULL::uuid),
  (1338, 1292, 'HEJAZ', 'الحجاز', NULL::uuid),
  (1736, 2116, 'ASIR', 'عسير', NULL::uuid),
  (2028, 2292, 'NEJRAN', 'نجران', NULL::uuid),
  (1940, 2594, 'YEMEN', 'اليمن', NULL::uuid),
  (2050, 2440, 'JAUF', 'الجوف (اليمن)', NULL::uuid),
  (2664, 2436, 'HADHRAMAUT', 'حضرموت', NULL::uuid),
  (3090, 2266, 'DHUFAR', 'ظفار', NULL::uuid),
  (3314, 1492, 'OMAN', 'عُمان', NULL::uuid),
  (2940, 1460, 'TRUCIAL OMAN', 'ساحل عُمان', NULL::uuid),
  (3026, 1494, 'DHAFRAH', 'الظفرة', NULL::uuid),
  (2424, 2248, 'BAHR ES-SAFI (AHQAF)', 'الأحقاف', NULL::uuid),
  (1288, 646, 'WADI SIRHAN', 'وادي السرحان', NULL::uuid),
  (1504, 718, 'JAUF EL-''AMR', 'دومة الجندل (الجوف)', NULL::uuid),
  (1720, 1226, 'WADI RUMMAH', 'وادي الرمة', NULL::uuid),
  (2070, 1474, 'JEBEL TUWAIQ', 'جبل طويق', NULL::uuid),
  (1376, 982, 'TEIMA', 'تيماء', NULL::uuid),
  (1342, 1110, 'EL-''ALA', 'العلا', NULL::uuid),
  (2238, 1368, 'RIYADH', 'الرياض', NULL::uuid),
  (1918, 1164, 'BOREIDAH', 'بريدة', NULL::uuid)
) AS v(x, y, printed_name, name_ar, tribe_id)
LEFT JOIN tribes t ON t.id = v.tribe_id AND t.visibility = 'public'
ON CONFLICT (source_id, scan_sha256, x, y) DO UPDATE SET
  printed_name = EXCLUDED.printed_name, name_ar = EXCLUDED.name_ar,
  tribe_id = COALESCE(plate_captions.tribe_id, EXCLUDED.tribe_id);

-- ottoman-asir-pre1919 (22 captions)
INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from, citation_ar, location_ar, visibility, metadata)
VALUES (
  'b0000000-0000-0000-0000-00000000e008',
  'خريطة عثمانية لقبائل عسير وتهامة (قبل ١٩١٩م)',
  'هيئة المساحة العثمانية', 'archive_record', '1918-01-01',
  'خريطة عثمانية عسكرية لقبائل جنوب غرب الجزيرة العربية (عسير وتهامة بين القنفذة وجازان)، قبل ١٩١٩م.',
  'عسير وتهامة', 'public',
  $json${"license": "public_domain", "license_note": "Pre-1919 Ottoman imperial cartographic work; Wikimedia Commons Public Domain.", "archive_title": "File:خريطة عثمانية لقبائل جنوب غرب الجزيرة العربية - آل سعد بللسمر.jpg", "fetched_at": "2026-11-23", "file_sha256": "e116f47cc1d73432f88d2d372150a1ad8217b44b3b63e63b297dd9d8ed17b9d6", "plate_key": "ottoman-asir-pre1919", "plate_language": "ar", "depicted_from_year": 1900, "depicted_to_year": 1918, "file_bytes": 256531, "generator": "infra/local/fit-atlas-affine.py", "georef": {"method": "control_points", "reason": "", "model": {"kind": "affine", "affine": [0.0020407376632993537, 0.00010336284818931565, 40.6035481919713, -0.0002935693487037273, -0.0017287213219728528, 19.90961304000086]}, "frame": {"outer_left": 8.0, "inner_left": 8.0, "outer_right": 850.0, "inner_right": 850.0, "outer_top": 8.0, "inner_top": 8.0, "outer_bottom": 1060.0, "inner_bottom": 1060.0}, "fit": {"lonRmsDeg": 0.11631171078872257, "latRmsDeg": 0.1468354234867709, "lonMaxDeg": 0.19410352132712205, "latMaxDeg": 0.25431860538145656, "heldOutCount": 4, "maxErrorKm": 30.375015666949384, "maxAllowedKm": 35.0, "rmsKm": 20.395711897484635, "coverage": {"fit": {"xSpanFraction": 0.7719714964370546, "ySpanFraction": 0.8517110266159695}, "check": {"xSpanFraction": 0.41092636579572445, "ySpanFraction": 0.903041825095057}}, "intendedScale": "regional_sheet"}, "certainty": "approximate", "needsReview": true, "reviewReasons": ["landmark_based_registration"]}}$json$::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  title_ar = EXCLUDED.title_ar, author_ar = EXCLUDED.author_ar,
  citation_ar = EXCLUDED.citation_ar, location_ar = EXCLUDED.location_ar,
  metadata = EXCLUDED.metadata, updated_at = now();

INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
SELECT 'b0000000-0000-0000-0000-00000000e008', 'sources/plates/ottoman-asir-pre1919.jpg', 'ottoman-asir-pre1919.jpg',
       'image/jpeg', 256531, 'e116f47cc1d73432f88d2d372150a1ad8217b44b3b63e63b297dd9d8ed17b9d6'
WHERE NOT EXISTS (SELECT 1 FROM source_files WHERE storage_key = 'sources/plates/ottoman-asir-pre1919.jpg');

INSERT INTO plate_captions (source_id, scan_sha256, x, y, printed_name, name_ar, tribe_id)
SELECT
  'b0000000-0000-0000-0000-00000000e008'::uuid, 'e116f47cc1d73432f88d2d372150a1ad8217b44b3b63e63b297dd9d8ed17b9d6',
  v.x, v.y, v.printed_name, v.name_ar, t.id
FROM (VALUES
  (664, 274, 'غامد قضاءسي', 'غامد', NULL::uuid),
  (472, 34, 'زهران', 'زهران', NULL::uuid),
  (688, 428, 'آل سعد', 'آل سعد (بللسمر)', NULL::uuid),
  (450, 316, 'آل حارث بدويلري', 'بلحارث', NULL::uuid),
  (482, 444, 'بارق بدويلري', 'بارق', NULL::uuid),
  (620, 326, 'آل موسى بدويلري', 'آل موسى', NULL::uuid),
  (628, 452, 'ربيعة المقاطرة', 'ربيعة المقاطرة', NULL::uuid),
  (488, 474, 'ربيعة التحليف', 'ربيعة التحليف', NULL::uuid),
  (676, 708, 'قحطان قضاءسي', 'قحطان', 'b0000000-0000-0000-0000-c0000000000a'::uuid),
  (466, 566, 'منجحة بدويلري', 'منجحة', NULL::uuid),
  (654, 594, 'آل موسى (محايل)', 'آل موسى (محايل)', NULL::uuid),
  (678, 560, 'محايل', 'محايل عسير', NULL::uuid),
  (186, 404, 'قنفذه', 'القنفذة', NULL::uuid),
  (312, 356, 'بني زيد قبيله سي', 'بني زيد', NULL::uuid),
  (380, 440, 'قواسمة قبيله سي', 'القواسمة', NULL::uuid),
  (502, 882, 'بني عبيدة', 'بني عبيدة', NULL::uuid),
  (554, 834, 'بني هاجر', 'بني هاجر', NULL::uuid),
  (506, 694, 'آل حطاب', 'آل حطاب', NULL::uuid),
  (572, 134, 'عوام قبيله سي', 'العوامر', NULL::uuid),
  (72, 70, 'بني مالك', 'بني مالك', NULL::uuid),
  (104, 26, 'بني سليم', 'بني سليم', NULL::uuid),
  (338, 676, 'حلي', 'وادي حلي', NULL::uuid)
) AS v(x, y, printed_name, name_ar, tribe_id)
LEFT JOIN tribes t ON t.id = v.tribe_id AND t.visibility = 'public'
ON CONFLICT (source_id, scan_sha256, x, y) DO UPDATE SET
  printed_name = EXCLUDED.printed_name, name_ar = EXCLUDED.name_ar,
  tribe_id = COALESCE(plate_captions.tribe_id, EXCLUDED.tribe_id);

-- iraq-1930-hashimi (25 captions)
INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from, citation_ar, location_ar, visibility, metadata)
VALUES (
  'b0000000-0000-0000-0000-00000000e009',
  'خارطة توزيع قبائل وسط وجنوب العراق عام ١٩٣٠ — طه الهاشمي',
  'بحسب وصف طه الهاشمي (جغرافية العراق ١٩٣٠)', 'other', '1930-01-01',
  'خارطة توزيع القبائل في العراق عام ١٩٣٠ بحسب وصف طه الهاشمي في كتاب جغرافية العراق · CC0.',
  'العراق وشمال الجزيرة', 'public',
  $json${"license": "cc0", "license_note": "Wikimedia Commons File:خارطة توزيع القبائل في العراق عام 1930.png dedicated under CC0.", "archive_title": "File:خارطة توزيع القبائل في العراق عام 1930.png", "fetched_at": "2026-11-23", "file_sha256": "f6963be7a630a99d009206d5ad0122d3e02467fc01c573bd73cc794d09584268", "plate_key": "iraq-1930-hashimi", "plate_language": "ar", "depicted_from_year": 1930, "depicted_to_year": 1930, "file_bytes": 409836, "generator": "infra/local/fit-atlas-affine.py", "georef": {"method": "control_points", "reason": "", "model": {"kind": "affine", "affine": [0.004898973188706751, -8.193692617569298e-05, 44.158789980738014, -0.0001304974434138535, -0.003993792529731885, 33.82089139829197]}, "frame": {"outer_left": 6.0, "inner_left": 6.0, "outer_right": 1074.0, "inner_right": 1074.0, "outer_top": 6.0, "inner_top": 6.0, "outer_bottom": 990.0, "inner_bottom": 990.0}, "fit": {"lonRmsDeg": 0.2711424465521505, "latRmsDeg": 0.06711826089863594, "lonMaxDeg": 0.44598966184056366, "latMaxDeg": 0.1296299872648632, "heldOutCount": 5, "maxErrorKm": 43.71020392331645, "maxAllowedKm": 50.0, "rmsKm": 26.4412840283341, "coverage": {"fit": {"xSpanFraction": 0.6647940074906367, "ySpanFraction": 0.6890243902439024}, "check": {"xSpanFraction": 0.4250936329588015, "ySpanFraction": 0.6382113821138211}}, "intendedScale": "regional_sheet"}, "certainty": "approximate", "needsReview": true, "reviewReasons": ["landmark_based_registration"]}}$json$::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  title_ar = EXCLUDED.title_ar, author_ar = EXCLUDED.author_ar,
  citation_ar = EXCLUDED.citation_ar, location_ar = EXCLUDED.location_ar,
  metadata = EXCLUDED.metadata, updated_at = now();

INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
SELECT 'b0000000-0000-0000-0000-00000000e009', 'sources/plates/iraq-1930-hashimi.png', 'iraq-1930-hashimi.png',
       'image/png', 409836, 'f6963be7a630a99d009206d5ad0122d3e02467fc01c573bd73cc794d09584268'
WHERE NOT EXISTS (SELECT 1 FROM source_files WHERE storage_key = 'sources/plates/iraq-1930-hashimi.png');

INSERT INTO plate_captions (source_id, scan_sha256, x, y, printed_name, name_ar, tribe_id)
SELECT
  'b0000000-0000-0000-0000-00000000e009'::uuid, 'f6963be7a630a99d009206d5ad0122d3e02467fc01c573bd73cc794d09584268',
  v.x, v.y, v.printed_name, v.name_ar, t.id
FROM (VALUES
  (276, 800, 'الظفير', 'الظفير', NULL::uuid),
  (224, 240, 'شمر', 'شمّر', 'b0000000-0000-0000-0000-c00000000002'::uuid),
  (34, 520, 'شمر', 'شمّر (البادية الغربية)', 'b0000000-0000-0000-0000-c00000000002'::uuid),
  (540, 320, 'بني لام', 'بنو لام', NULL::uuid),
  (332, 256, 'ربيعة', 'ربيعة', NULL::uuid),
  (60, 224, 'زبيد', 'زبيد', NULL::uuid),
  (116, 108, 'دليم', 'الدليم', NULL::uuid),
  (236, 20, 'تميم', 'بنو تميم', 'b0000000-0000-0000-0000-c00000000004'::uuid),
  (488, 720, 'بني مالك', 'بنو مالك (الناصرية)', NULL::uuid),
  (806, 840, 'بني مالك', 'بنو مالك (البصرة)', NULL::uuid),
  (346, 608, 'السعدون', 'السعدون (المنتفق)', NULL::uuid),
  (368, 558, 'العبودة', 'العبودة', NULL::uuid),
  (382, 588, 'خفاجة', 'خفاجة', NULL::uuid),
  (352, 476, 'بني ركاب', 'بني ركاب', NULL::uuid),
  (346, 636, 'البدور', 'البدور', NULL::uuid),
  (440, 444, 'الحميد', 'الحميد', NULL::uuid),
  (516, 540, 'بني سعيد', 'بني سعيد', NULL::uuid),
  (664, 536, 'البو محمد', 'آل بو محمد', NULL::uuid),
  (532, 444, 'البو دراج', 'آل بو دراج', NULL::uuid),
  (338, 376, 'السراج', 'السراج', NULL::uuid),
  (748, 696, 'المياح', 'المياح', NULL::uuid),
  (124, 514, 'الخزاعل', 'الخزاعل', NULL::uuid),
  (88, 646, 'ال زياد', 'آل زياد', NULL::uuid),
  (576, 680, 'بني اسد', 'بنو أسد', NULL::uuid),
  (668, 748, 'الهارثة', 'الهارثة', NULL::uuid)
) AS v(x, y, printed_name, name_ar, tribe_id)
LEFT JOIN tribes t ON t.id = v.tribe_id AND t.visibility = 'public'
ON CONFLICT (source_id, scan_sha256, x, y) DO UPDATE SET
  printed_name = EXCLUDED.printed_name, name_ar = EXCLUDED.name_ar,
  tribe_id = COALESCE(plate_captions.tribe_id, EXCLUDED.tribe_id);

-- thomas-1931-rub-al-khali (28 captions)
INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from, citation_ar, location_ar, visibility, metadata)
VALUES (
  'b0000000-0000-0000-0000-00000000e010',
  'خريطة الربع الخالي وعُمان وظفار وقطر — رحلة برترام توماس ١٩٣٠–١٩٣١',
  'Bertram Thomas · Royal Geographical Society', 'archive_record', '1931-09-01',
  'Bertram Thomas, ''Route Traverse across the Rub'' al Khali from Dhufar to Doha, 1930-31'', The Geographical Journal, Royal Geographical Society, Sept. 1931.',
  'الربع الخالي وعُمان وقطر', 'public',
  $json${"license": "public_domain", "license_note": "Published in The Geographical Journal, Sept. 1931; Wikimedia Commons File:Oman 1930.jpg tagged Public Domain.", "archive_title": "File:Oman 1930.jpg", "fetched_at": "2026-11-23", "file_sha256": "3c19d3ea7ab0dfbd0b0ba30a92854bebb3b180383adfa6f3a776b9844b5f6783", "plate_key": "thomas-1931-rub-al-khali", "plate_language": "en", "depicted_from_year": 1930, "depicted_to_year": 1931, "file_bytes": 2282197, "generator": "infra/local/fit-atlas-affine.py", "georef": {"method": "control_points", "reason": "", "model": {"kind": "affine", "affine": [0.004270221251732317, -3.0063169196672054e-05, 44.609062215124936, -2.280501500362817e-05, -0.004289201551412409, 27.67436366934591]}, "frame": {"outer_left": 198.0, "inner_left": 198.0, "outer_right": 3702.0, "inner_right": 3702.0, "outer_top": 212.0, "inner_top": 212.0, "outer_bottom": 2788.0, "inner_bottom": 2788.0}, "fit": {"lonRmsDeg": 0.11506288098494777, "latRmsDeg": 0.05897107970426879, "lonMaxDeg": 0.2400999371786483, "latMaxDeg": 0.10114503969041522, "heldOutCount": 6, "maxErrorKm": 24.431449942018784, "maxAllowedKm": 35.0, "rmsKm": 13.414711956443014, "coverage": {"fit": {"xSpanFraction": 0.8344748858447488, "ySpanFraction": 0.7298136645962733}, "check": {"xSpanFraction": 0.7734018264840182, "ySpanFraction": 0.7523291925465838}}, "intendedScale": "regional_sheet"}, "certainty": "approximate", "needsReview": true, "reviewReasons": ["landmark_based_registration"]}}$json$::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  title_ar = EXCLUDED.title_ar, author_ar = EXCLUDED.author_ar,
  citation_ar = EXCLUDED.citation_ar, location_ar = EXCLUDED.location_ar,
  metadata = EXCLUDED.metadata, updated_at = now();

INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
SELECT 'b0000000-0000-0000-0000-00000000e010', 'sources/plates/thomas-1931-rub-al-khali.jpg', 'thomas-1931-rub-al-khali.jpg',
       'image/jpeg', 2282197, '3c19d3ea7ab0dfbd0b0ba30a92854bebb3b180383adfa6f3a776b9844b5f6783'
WHERE NOT EXISTS (SELECT 1 FROM source_files WHERE storage_key = 'sources/plates/thomas-1931-rub-al-khali.jpg');

INSERT INTO plate_captions (source_id, scan_sha256, x, y, printed_name, name_ar, tribe_id)
SELECT
  'b0000000-0000-0000-0000-00000000e010'::uuid, '3c19d3ea7ab0dfbd0b0ba30a92854bebb3b180383adfa6f3a776b9844b5f6783',
  v.x, v.y, v.printed_name, v.name_ar, t.id
FROM (VALUES
  (1450, 1480, 'MURRA', 'آل مرة', NULL::uuid),
  (1974, 964, 'MANASIR', 'المناصير', NULL::uuid),
  (2406, 976, 'AWAMIR', 'العوامر', NULL::uuid),
  (2646, 1054, 'AL BU SHAMIS', 'آل بو شامس', NULL::uuid),
  (2670, 1140, 'DARU', 'الدرع', NULL::uuid),
  (2648, 1254, 'HARASIS', 'الحراسيس', NULL::uuid),
  (2612, 1936, 'HARASIS', 'الحراسيس (جدة الحراسيس)', NULL::uuid),
  (2624, 1316, 'AFAR', 'عفار', NULL::uuid),
  (3324, 1446, 'YAL WAHIBA', 'آل وهيبة', NULL::uuid),
  (1252, 1656, 'Bani Zainan', 'بني زينان', NULL::uuid),
  (250, 1696, 'Sulaiyil (Dawasir)', 'الدواسر (السليل)', NULL::uuid),
  (1680, 1796, 'Ajman (well)', 'العجمان (آبار الربع الخالي)', NULL::uuid),
  (1812, 2004, 'RASHID', 'الرواشد', NULL::uuid),
  (1684, 2248, 'BAIT KATHIR', 'بيت كثير', NULL::uuid),
  (1884, 2194, 'BAIT IMANI', 'بيت إيماني', NULL::uuid),
  (1510, 2418, 'MANAHIL', 'المناهيل', NULL::uuid),
  (1406, 2368, 'AWAMIR', 'العوامر (الجنوب)', NULL::uuid),
  (1116, 2496, 'SAAR', 'الصيعر', NULL::uuid),
  (2110, 2600, 'MAHRA', 'المهرة', NULL::uuid),
  (2050, 2556, 'Jabal Qamr', 'القرا (جبل القمر)', NULL::uuid),
  (1236, 562, 'HASA', 'الأحساء', NULL::uuid),
  (1558, 520, 'QATAR', 'قطر', NULL::uuid),
  (2316, 804, 'TRUCIAL ''OMAN', 'ساحل عُمان', NULL::uuid),
  (2164, 926, 'DHAFRA', 'الظفرة', NULL::uuid),
  (1998, 1044, 'LIWA', 'ليوا', NULL::uuid),
  (1072, 1044, 'Jabrin', 'يبرين', NULL::uuid),
  (680, 360, 'Dahana', 'الدهناء', NULL::uuid),
  (2960, 1030, 'HAJAR (OMAN)', 'جبال الحجر (عُمان)', NULL::uuid)
) AS v(x, y, printed_name, name_ar, tribe_id)
LEFT JOIN tribes t ON t.id = v.tribe_id AND t.visibility = 'public'
ON CONFLICT (source_id, scan_sha256, x, y) DO UPDATE SET
  printed_name = EXCLUDED.printed_name, name_ar = EXCLUDED.name_ar,
  tribe_id = COALESCE(plate_captions.tribe_id, EXCLUDED.tribe_id);

-- yemen-south-arabia-tribes (48 captions)
INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from, citation_ar, location_ar, visibility, metadata)
VALUES (
  'b0000000-0000-0000-0000-00000000e011',
  'خريطة قبائل اليمن وعسير ونجران وجنوب الجزيرة العربية',
  'مساهم ويكيميديا كومنز (CC0)', 'other', NULL,
  'خريطة مناطق قبائل اليمن وجنوب الجزيرة العربية وعسير ونجران وحضرموت والمهرة · CC0.',
  'اليمن وجنوب الجزيرة', 'public',
  $json${"license": "cc0", "license_note": "Wikimedia Commons File:مناطق قبائل اليمن وجنوب الجزيرة العربية.png dedicated under CC0.", "archive_title": "File:مناطق قبائل اليمن وجنوب الجزيرة العربية.png", "fetched_at": "2026-11-23", "file_sha256": "157f02f2616bead36bea12de9946b3777a957806902ffb203a164fa058051e66", "plate_key": "yemen-south-arabia-tribes", "plate_language": "ar", "depicted_from_year": null, "depicted_to_year": null, "file_bytes": 1573663, "generator": "infra/local/fit-atlas-affine.py", "georef": {"method": "control_points", "reason": "", "model": {"kind": "affine", "affine": [0.009079338650494758, 2.125075272365272e-05, 40.33505011761302, 1.313030429663469e-05, -0.008407963093800562, 19.38986101963506]}, "frame": {"outer_left": 8.0, "inner_left": 8.0, "outer_right": 1592.0, "inner_right": 1592.0, "outer_top": 8.0, "inner_top": 8.0, "outer_bottom": 843.0, "inner_bottom": 843.0}, "fit": {"lonRmsDeg": 0.22343144342249788, "latRmsDeg": 0.19265435349327326, "lonMaxDeg": 0.3755440423857266, "latMaxDeg": 0.3502922730994271, "heldOutCount": 4, "maxErrorKm": 41.2289366861068, "maxAllowedKm": 50.0, "rmsKm": 32.122229937889585, "coverage": {"fit": {"xSpanFraction": 0.8156565656565656, "ySpanFraction": 0.7089820359281437}, "check": {"xSpanFraction": 0.6691919191919192, "ySpanFraction": 0.49101796407185627}}, "intendedScale": "regional_sheet"}, "certainty": "approximate", "needsReview": true, "reviewReasons": ["landmark_based_registration"]}}$json$::jsonb
)
ON CONFLICT (id) DO UPDATE SET
  title_ar = EXCLUDED.title_ar, author_ar = EXCLUDED.author_ar,
  citation_ar = EXCLUDED.citation_ar, location_ar = EXCLUDED.location_ar,
  metadata = EXCLUDED.metadata, updated_at = now();

INSERT INTO source_files (source_id, storage_key, original_filename_ar, mime_type, byte_size, checksum_sha256)
SELECT 'b0000000-0000-0000-0000-00000000e011', 'sources/plates/yemen-south-arabia-tribes.png', 'yemen-south-arabia-tribes.png',
       'image/png', 1573663, '157f02f2616bead36bea12de9946b3777a957806902ffb203a164fa058051e66'
WHERE NOT EXISTS (SELECT 1 FROM source_files WHERE storage_key = 'sources/plates/yemen-south-arabia-tribes.png');

INSERT INTO plate_captions (source_id, scan_sha256, x, y, printed_name, name_ar, tribe_id)
SELECT
  'b0000000-0000-0000-0000-00000000e011'::uuid, '157f02f2616bead36bea12de9946b3777a957806902ffb203a164fa058051e66',
  v.x, v.y, v.printed_name, v.name_ar, t.id
FROM (VALUES
  (628, 166, 'يام همدان', 'يام (همدان)', NULL::uuid),
  (304, 34, 'شهران', 'شهران', NULL::uuid),
  (448, 58, 'قحطان', 'قحطان', 'b0000000-0000-0000-0000-c0000000000a'::uuid),
  (192, 42, 'بنو شهر', 'بنو شهر', NULL::uuid),
  (210, 68, 'بلسمر', 'بللسمر', NULL::uuid),
  (214, 94, 'بلحمر', 'بللحمر', NULL::uuid),
  (214, 146, 'عسير', 'عسير', NULL::uuid),
  (968, 42, 'آل مرة', 'آل مرة', NULL::uuid),
  (1338, 22, 'الرواشد', 'الرواشد', NULL::uuid),
  (1458, 92, 'الكثيري', 'الكثيري (بيت كثير)', NULL::uuid),
  (1318, 356, 'المهرة', 'المهرة', NULL::uuid),
  (1524, 218, 'الشحري', 'الشحري', NULL::uuid),
  (1538, 272, 'الحكلي', 'الحكلي', NULL::uuid),
  (842, 232, 'الصيعر', 'الصيعر', NULL::uuid),
  (1016, 206, 'العوامر', 'العوامر', NULL::uuid),
  (1124, 198, 'المناهيل', 'المناهيل', NULL::uuid),
  (1114, 300, 'بني ضنة', 'بني ضنة', NULL::uuid),
  (932, 348, 'آل كثير', 'آل كثير', NULL::uuid),
  (794, 404, 'نهد', 'نهد', NULL::uuid),
  (944, 424, 'الجوابر', 'الجوابر', NULL::uuid),
  (1006, 466, 'الحموم', 'الحموم', NULL::uuid),
  (1104, 488, 'ثعين', 'ثعين', NULL::uuid),
  (976, 536, 'سيبان', 'سيبان', NULL::uuid),
  (898, 582, 'نوح', 'نوح', NULL::uuid),
  (928, 502, 'باوزير', 'باوزير', NULL::uuid),
  (912, 456, 'العوابثة', 'العوابثة', NULL::uuid),
  (834, 622, 'خولان المشرق', 'خولان المشرق', NULL::uuid),
  (546, 340, 'بكيل همدان', 'بكيل (همدان)', NULL::uuid),
  (380, 386, 'حاشد', 'حاشد', NULL::uuid),
  (330, 260, 'خولان', 'خولان بن عامر', NULL::uuid),
  (608, 434, 'عبيدة', 'عبيدة', NULL::uuid),
  (478, 476, 'خولان', 'خولان الطيال', NULL::uuid),
  (604, 496, 'بلحارث', 'بلحارث (شبوة)', NULL::uuid),
  (680, 456, 'الكرب', 'الكرب', NULL::uuid),
  (762, 470, 'آل بريك', 'آل بريك', NULL::uuid),
  (790, 532, 'نعمان', 'نعمان', NULL::uuid),
  (536, 526, 'مراد', 'مراد', NULL::uuid),
  (462, 532, 'الحدا', 'الحدا', NULL::uuid),
  (410, 532, 'آنس', 'آنس', NULL::uuid),
  (456, 566, 'عنس', 'عنس', NULL::uuid),
  (506, 562, 'قيفة', 'قيفة', NULL::uuid),
  (560, 588, 'بنير', 'بني ضبيان / بنير', NULL::uuid),
  (708, 634, 'العوالق', 'العوالق', NULL::uuid),
  (542, 666, 'يافع', 'يافع', NULL::uuid),
  (316, 566, 'الزرانيق', 'الزرانيق', NULL::uuid),
  (412, 756, 'الصبيحة', 'الصبيحة', NULL::uuid),
  (572, 706, 'آل فضل', 'آل فضل', NULL::uuid),
  (1492, 804, 'الفيسي / كلشات', 'قبائل سقطرى', NULL::uuid)
) AS v(x, y, printed_name, name_ar, tribe_id)
LEFT JOIN tribes t ON t.id = v.tribe_id AND t.visibility = 'public'
ON CONFLICT (source_id, scan_sha256, x, y) DO UPDATE SET
  printed_name = EXCLUDED.printed_name, name_ar = EXCLUDED.name_ar,
  tribe_id = COALESCE(plate_captions.tribe_id, EXCLUDED.tribe_id);

COMMIT;
