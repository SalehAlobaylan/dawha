-- Import the localized captions only for the scan their anchors were measured on.
INSERT INTO plate_captions (source_id, scan_sha256, x, y, printed_name, name_ar)
SELECT s.id, s.metadata->>'file_sha256', c.x, c.y, c.printed_name, c.name_ar
FROM sources s CROSS JOIN (VALUES
  (1850, 1721, '''ANEIZAH', 'عنزة'),
  (2823, 2778, 'SHAMMAR', 'شمّر'),
  (3140, 3561, '''ATEIBAH', 'عتيبة'),
  (1827, 3902, 'HARB', 'حرب'),
  (3587, 4374, 'QAHTAN', 'قحطان'),
  (3510, 4236, 'DAWASIR', 'الدواسر'),
  (3510, 2725, 'MUTEIR', 'مطير'),
  (1456, 2253, 'HUWEITAT', 'الحويطات'),
  (3504, 2306, 'MUNTEFIQ', 'المنتفق'),
  (3373, 2420, 'DHAFIR', 'الظفير'),
  (4268, 3651, 'AHL MURRAH', 'آل مرة'),
  (4883, 3836, 'MANASIR', 'المناصير'),
  (5909, 4021, 'GHAFIRI AND HINAWI FACTIONS', 'الغافرية والهناوية'),
  (4722, 5742, 'MAHRAH', 'المهرة'),
  (3050, 5288, 'YAM', 'يام'),
  (2716, 1464, 'DULEYM', 'الدليم'),
  (2418, 1034, 'TAYY', 'طيء'),
  (2853, 1087, 'SHAMMAR', 'شمّر'),
  (2543, 1135, 'JERBA', 'الجرباء'),
  (3080, 1601, 'TAUQAH', 'شمر طوقة'),
  (2006, 1392, 'FED''AN', 'الفدعان'),
  (2149, 1512, 'SIBA''', 'السبعة'),
  (2603, 1739, '''AMARAT', 'العمارات'),
  (2143, 1828, 'WULD ''ALI', 'ولد علي'),
  (1844, 1846, 'RUWEILAH', 'الرولة'),
  (1731, 1703, 'JEBBELIYAH', 'الجبلية'),
  (1540, 1948, 'SAKHR', 'بنو صخر'),
  (1612, 2032, 'HOMEIDAH', 'بنو حميدة'),
  (1767, 2450, 'HUTEIM', 'هتيم'),
  (1976, 2450, 'SHERARAT', 'الشرارات'),
  (1462, 2599, '''ATIYAH', 'بنو عطية'),
  (1737, 2790, 'MAWAHIB', 'المواهيب'),
  (2226, 2904, 'FUQARA', 'الفقراء'),
  (2232, 3053, 'WAHAB', 'بنو وهب'),
  (1659, 3173, 'BILLI', 'بلي'),
  (2322, 3334, 'W. SULEIMAN', 'ولد سليمان'),
  (1922, 3615, 'JUHEINAH', 'جهينة'),
  (3695, 2665, 'HAWAZIN', 'هوازن'),
  (3820, 2874, 'KHALID', 'بنو خالد'),
  (3420, 3053, 'BARRIYAH', 'برية'),
  (4077, 3221, '''AJMAN', 'العجمان'),
  (4286, 3221, 'HAJAR', 'بنو هاجر'),
  (3002, 4439, 'SEBEI''', 'سبيع'),
  (2549, 4487, 'THAQIF', 'ثقيف'),
  (2418, 4505, 'HUDHEIL', 'هذيل'),
  (2328, 4583, 'JUHADLAH', 'الجحادلة'),
  (2656, 4696, 'GHAMID', 'غامد'),
  (3050, 4708, 'BUQUM', 'البقوم'),
  (2937, 4929, 'SHAHRAN', 'شهران'),
  (2985, 5138, 'RABI''AH WA RUFEIDAH', 'ربيعة ورفيدة'),
  (2847, 5246, 'MUGHEID', 'بنو مغيد'),
  (2728, 5246, 'RIJAL EL-MA''', 'رجال ألمع'),
  (5223, 5300, 'QARA', 'القرا'),
  (5235, 5413, 'AL KATHIR', 'آل كثير'),
  (3098, 5664, 'HASHID', 'حاشد'),
  (4280, 5664, 'KATHIRI', 'الكثيري'),
  (4172, 5897, 'KA''AITI', 'القعيطي'),
  (2955, 5885, 'QUHRAH', 'القحرى'),
  (3092, 5939, '''ANIS', 'آنس'),
  (2955, 6071, 'ZARANIK', 'الزرانيق'),
  (3396, 6124, 'YAFA''I', 'يافع'),
  (3910, 6094, 'WAHIDI', 'الواحدي'),
  (3653, 6094, '''AULAQI', 'العوالق'),
  (3271, 6214, 'AMIRI', 'الأميري'),
  (3486, 6256, 'FADHLI', 'الفضلي'),
  (3241, 6322, 'HAUSHABI', 'الحوشبي'),
  (3092, 6399, 'SUBEIHI', 'الصبيحي')
) AS c(x, y, printed_name, name_ar)
WHERE s.metadata->>'plate_key' = 'admiralty-1916-tribal'
  AND s.metadata->>'file_sha256' = '6de65d63ff0c3bef2e81bb5709403e08e4330a97b50af98adc8510729da06814'
ON CONFLICT (source_id, scan_sha256, x, y) DO NOTHING;

-- Link only explicit identifications against the documented tribe seed IDs.
UPDATE plate_captions c
SET tribe_id = t.id
FROM (VALUES
  ('HARB',     'b0000000-0000-0000-0000-c00000000001'),
  ('SHAMMAR',  'b0000000-0000-0000-0000-c00000000002'),
  ('''ANEIZAH','b0000000-0000-0000-0000-c00000000003'),
  ('''ATEIBAH','b0000000-0000-0000-0000-c00000000005'),
  ('MUTEIR',   'b0000000-0000-0000-0000-c00000000006'),
  ('KHALID',   'b0000000-0000-0000-0000-c00000000007'),
  ('HAWAZIN',  'b0000000-0000-0000-0000-c00000000008'),
  ('QAHTAN',   'b0000000-0000-0000-0000-c0000000000a')
) AS known(printed_name, tribe_id)
JOIN tribes t ON t.id = known.tribe_id::uuid AND t.visibility = 'public'
WHERE c.source_id = 'b0000000-0000-0000-0000-00000000e004'
  AND c.scan_sha256 = '6de65d63ff0c3bef2e81bb5709403e08e4330a97b50af98adc8510729da06814'
  AND c.printed_name = known.printed_name AND c.tribe_id IS NULL;
