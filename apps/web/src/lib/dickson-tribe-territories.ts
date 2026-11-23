// Digitized tribal territory boundaries traced from the fine dotted lines on
// Harold R. P. Dickson's 1914 "Rough Tribal Map of Arabia" (1572 × 1761 scan).
//
// Register rule (Plans 030/032): these polygons are visual localisations of the
// historical plate's own approximate dotted lines ("Tribal Boundaries are only
// approximate"). They belong to the plate's visual register and are projected
// through the plate's stored affine transform — never promoted as verified
// historical_regions without independent human review.

export interface PlateTribeTerritory {
  /** Matches the caption anchor coordinates (x, y) in dickson-tribe-labels.ts. */
  captionX: number;
  captionY: number;
  printedName: string;
  nameAr: string;
  /** Optional subtitle explaining the geographic span shown on the plate. */
  spanAr?: string;
  /** Closed polygon ring(s) in Dickson 1914 image pixels [x, y]. */
  rings: ReadonlyArray<ReadonlyArray<readonly [number, number]>>;
}

export const dicksonTribeTerritories: ReadonlyArray<PlateTribeTerritory> = [
  // ── Major confederations ──────────────────────────────────────────────
  {
    captionX: 432,
    captionY: 1066,
    printedName: "Harb",
    nameAr: "حرب",
    spanAr: "يمتد نطاقها في اللوحة من ساحل الحجاز بين المدينة ومكة حتى القصيم وشمال نجد",
    rings: [[
      [338, 1022], [385, 1012], [418, 982], [456, 976], [496, 944],
      [548, 910], [608, 868], [666, 822], [712, 778], [750, 744],
      [786, 766], [752, 814], [710, 854], [662, 884], [606, 902],
      [554, 928], [514, 960], [494, 1004], [480, 1068], [472, 1126],
      [460, 1162], [400, 1158], [382, 1110], [358, 1062], [338, 1022],
    ]],
  },
  {
    captionX: 692,
    captionY: 816,
    printedName: "Harb.",
    nameAr: "حرب",
    spanAr: "يمتد نطاقها في اللوحة من ساحل الحجاز بين المدينة ومكة حتى القصيم وشمال نجد",
    rings: [[
      [338, 1022], [385, 1012], [418, 982], [456, 976], [496, 944],
      [548, 910], [608, 868], [666, 822], [712, 778], [750, 744],
      [786, 766], [752, 814], [710, 854], [662, 884], [606, 902],
      [554, 928], [514, 960], [494, 1004], [480, 1068], [472, 1126],
      [460, 1162], [400, 1158], [382, 1110], [358, 1062], [338, 1022],
    ]],
  },
  {
    captionX: 596,
    captionY: 1068,
    printedName: "'Utaibah (Many Section)",
    nameAr: "عتيبة",
    spanAr: "نطاق واسع وسط نجد والحجاز بين مكة والقصيم والرياض",
    rings: [[
      [472, 1142], [482, 1068], [504, 1008], [570, 992], [648, 964],
      [688, 944], [736, 952], [742, 1012], [724, 1078], [696, 1138],
      [666, 1182], [616, 1186], [568, 1190], [516, 1180], [472, 1142],
    ]],
  },
  {
    captionX: 825,
    captionY: 845,
    printedName: "Mutair ('Ilwa & Wasil / Buraih)",
    nameAr: "مطير",
    spanAr: "الصمّان وشرق القصيم حتى حدود الكويت والعوازم",
    rings: [[
      [786, 766], [850, 762], [870, 822], [876, 876], [870, 932],
      [818, 924], [768, 898], [732, 862], [752, 814], [786, 766],
    ]],
  },
  {
    captionX: 592,
    captionY: 956,
    printedName: "MUTAIR (Bani Abdillah / Many Section)",
    nameAr: "مطير (بنو عبدالله)",
    spanAr: "غرب القصيم بين حرب وعتيبة",
    rings: [[
      [504, 1008], [516, 962], [568, 926], [642, 902], [688, 908],
      [688, 944], [648, 964], [570, 992], [504, 1008],
    ]],
  },
  {
    captionX: 615,
    captionY: 830,
    printedName: "Shammar",
    nameAr: "شمّر",
    spanAr: "جبل شمّر (حائل) والنفود الكبير",
    rings: [[
      [496, 834], [568, 804], [642, 774], [688, 774], [662, 844],
      [602, 880], [548, 910], [526, 874], [496, 834],
    ]],
  },
  {
    captionX: 554,
    captionY: 776,
    printedName: "Abda",
    nameAr: "عبدة (شمّر)",
    rings: [[
      [462, 764], [532, 736], [616, 738], [656, 766], [642, 774],
      [568, 804], [496, 834], [462, 764],
    ]],
  },
  {
    captionX: 694,
    captionY: 738,
    printedName: "Saiyah",
    nameAr: "الصايح (شمّر)",
    rings: [[
      [616, 738], [664, 706], [734, 718], [762, 746], [712, 774],
      [656, 766], [616, 738],
    ]],
  },
  {
    captionX: 564,
    captionY: 342,
    printedName: "Jerba Shammar",
    nameAr: "شمر الجرباء",
    rings: [[
      [506, 316], [564, 296], [620, 314], [638, 362], [606, 404],
      [546, 414], [490, 392], [488, 346], [506, 316],
    ]],
  },
  {
    captionX: 784,
    captionY: 1265,
    printedName: "QAHTAN (Many Section)",
    nameAr: "قحطان",
    spanAr: "جنوب نجد بين الدواسر وعسير ويام والربع الخالي",
    rings: [[
      [682, 1216], [746, 1186], [806, 1122], [852, 1086], [892, 1132],
      [904, 1216], [878, 1292], [824, 1340], [744, 1352], [674, 1346],
      [656, 1286], [682, 1216],
    ]],
  },
  {
    captionX: 765,
    captionY: 1142,
    printedName: "Duwasir",
    nameAr: "الدواسر",
    spanAr: "وادي الدواسر بين عتيبة والسهول وقحطان",
    rings: [[
      [688, 1178], [724, 1112], [756, 1058], [818, 1058], [852, 1086],
      [796, 1142], [734, 1198], [684, 1216], [688, 1178],
    ]],
  },
  {
    captionX: 1032,
    captionY: 1168,
    printedName: "Al-Murra (Many Section)",
    nameAr: "آل مرة",
    spanAr: "جنوب الأحساء وشرق الربع الخالي",
    rings: [[
      [948, 1046], [1082, 1044], [1116, 1114], [1148, 1196], [1162, 1244],
      [1072, 1258], [978, 1236], [922, 1172], [916, 1094], [948, 1046],
    ]],
  },
  {
    captionX: 922,
    captionY: 910,
    printedName: "AJMAN (Many Sec.)",
    nameAr: "العجمان",
    spanAr: "الأحساء والهفوف",
    rings: [[
      [872, 874], [942, 888], [978, 914], [974, 956], [918, 962],
      [868, 940], [872, 874],
    ]],
  },
  {
    captionX: 922,
    captionY: 868,
    printedName: "Bani Khalid",
    nameAr: "بنو خالد",
    spanAr: "ساحل الخليج شمال القطيف",
    rings: [[
      [866, 822], [920, 836], [958, 866], [978, 898], [942, 888],
      [872, 874], [866, 822],
    ]],
  },
  {
    captionX: 904,
    captionY: 804,
    printedName: "Alwazim",
    nameAr: "العوازم",
    spanAr: "جنوب الكويت على ساحل الخليج",
    rings: [[
      [852, 766], [894, 744], [918, 786], [948, 832], [958, 866],
      [920, 836], [866, 822], [852, 766],
    ]],
  },
  {
    captionX: 964,
    captionY: 986,
    printedName: "Bani Hajir",
    nameAr: "بنو هاجر",
    spanAr: "جنوب الهفوف وغرب قطر",
    rings: [[
      [918, 962], [974, 956], [992, 978], [984, 1038], [918, 1046],
      [904, 1002], [918, 962],
    ]],
  },
  {
    captionX: 820,
    captionY: 954,
    printedName: "Sbei",
    nameAr: "سبيع",
    rings: [[
      [770, 932], [848, 932], [866, 968], [840, 996], [770, 996], [770, 932],
    ]],
  },
  {
    captionX: 828,
    captionY: 1026,
    printedName: "Suhul",
    nameAr: "السهول",
    rings: [[
      [770, 996], [852, 996], [868, 1038], [834, 1062], [764, 1058], [770, 996],
    ]],
  },
  {
    captionX: 660,
    captionY: 1204,
    printedName: "Sbei 'Illawiyin",
    nameAr: "سبيع (العلويين)",
    rings: [[
      [622, 1190], [682, 1184], [682, 1226], [634, 1236], [606, 1218], [622, 1190],
    ]],
  },

  // ── Northern Arabia, Syrian Desert & Iraq ─────────────────────────────
  {
    captionX: 390,
    captionY: 505,
    printedName: "'Anizah",
    nameAr: "عنزة",
    spanAr: "بادية الشام الكبرى (الفدعان، السبعة، الرولة، ولد علي، العمارات)",
    rings: [[
      [296, 434], [358, 376], [438, 378], [502, 434], [588, 488],
      [618, 566], [568, 608], [462, 622], [358, 602], [296, 514], [296, 434],
    ]],
  },
  {
    captionX: 398,
    captionY: 400,
    printedName: "Fadan",
    nameAr: "الفدعان",
    rings: [[
      [344, 382], [432, 374], [462, 404], [428, 424], [356, 422], [344, 382],
    ]],
  },
  {
    captionX: 442,
    captionY: 438,
    printedName: "Sbaa",
    nameAr: "السبعة",
    rings: [[
      [402, 424], [472, 412], [492, 452], [442, 466], [398, 450], [402, 424],
    ]],
  },
  {
    captionX: 398,
    captionY: 502,
    printedName: "Wuld Ali",
    nameAr: "ولد علي",
    rings: [[
      [346, 474], [418, 474], [446, 512], [414, 530], [354, 516], [346, 474],
    ]],
  },
  {
    captionX: 395,
    captionY: 552,
    printedName: "Ruwala",
    nameAr: "الرولة",
    rings: [[
      [338, 526], [434, 526], [462, 568], [434, 604], [346, 594], [338, 526],
    ]],
  },
  {
    captionX: 534,
    captionY: 510,
    printedName: "Amarat",
    nameAr: "العمارات",
    rings: [[
      [476, 474], [574, 476], [610, 524], [588, 568], [486, 566], [476, 474],
    ]],
  },
  {
    captionX: 604,
    captionY: 622,
    printedName: "Dahamsha (Amarat)",
    nameAr: "الدهامشة",
    rings: [[
      [536, 588], [648, 582], [684, 622], [656, 668], [556, 672], [536, 588],
    ]],
  },
  {
    captionX: 556,
    captionY: 452,
    printedName: "Dulaim",
    nameAr: "الدليم",
    rings: [[
      [512, 416], [588, 418], [608, 454], [586, 488], [526, 482], [504, 444], [512, 416],
    ]],
  },
  {
    captionX: 658,
    captionY: 466,
    printedName: "Tauqah",
    nameAr: "شمر طوقة",
    rings: [[
      [620, 444], [692, 444], [708, 486], [670, 502], [620, 486], [620, 444],
    ]],
  },
  {
    captionX: 740,
    captionY: 522,
    printedName: "Zoba",
    nameAr: "زوبع",
    rings: [[
      [698, 500], [776, 502], [794, 538], [752, 554], [700, 534], [698, 500],
    ]],
  },
  {
    captionX: 848,
    captionY: 524,
    printedName: "Bani Lam",
    nameAr: "بنو لام",
    rings: [[
      [798, 496], [888, 496], [908, 542], [858, 562], [802, 540], [798, 496],
    ]],
  },
  {
    captionX: 720,
    captionY: 566,
    printedName: "Khazail",
    nameAr: "الخزاعل",
    rings: [[
      [682, 546], [760, 548], [768, 586], [716, 594], [682, 578], [682, 546],
    ]],
  },
  {
    captionX: 794,
    captionY: 608,
    printedName: "Rubiah",
    nameAr: "ربيعة",
    rings: [[
      [766, 570], [820, 566], [854, 622], [822, 650], [764, 624], [766, 570],
    ]],
  },
  {
    captionX: 912,
    captionY: 600,
    printedName: "Bani Turuf",
    nameAr: "بنو طرف",
    rings: [[
      [856, 574], [958, 574], [968, 616], [912, 628], [858, 614], [856, 574],
    ]],
  },
  {
    captionX: 948,
    captionY: 650,
    printedName: "Bani Kaab",
    nameAr: "بنو كعب",
    rings: [[
      [898, 632], [992, 626], [1004, 668], [946, 682], [898, 664], [898, 632],
    ]],
  },
  {
    captionX: 744,
    captionY: 642,
    printedName: "Muntafiq",
    nameAr: "المنتفق",
    rings: [[
      [686, 610], [776, 614], [822, 656], [790, 676], [692, 670], [686, 610],
    ]],
  },
  {
    captionX: 712,
    captionY: 692,
    printedName: "Dhafir",
    nameAr: "الظفير",
    rings: [[
      [646, 670], [786, 668], [826, 704], [782, 728], [666, 712], [646, 670],
    ]],
  },
  {
    captionX: 268,
    captionY: 558,
    printedName: "Bani Sakhr",
    nameAr: "بنو صخر",
    rings: [[
      [234, 526], [286, 528], [296, 578], [254, 594], [224, 570], [234, 526],
    ]],
  },
  {
    captionX: 300,
    captionY: 542,
    printedName: "Jebeliyah (Hauran Druze)",
    nameAr: "الجبلية (دروز حوران)",
    rings: [[
      [278, 492], [318, 496], [336, 576], [302, 592], [278, 540], [278, 492],
    ]],
  },
  {
    captionX: 237,
    captionY: 646,
    printedName: "Huwaitat (HUTAIM)",
    nameAr: "الحويطات",
    rings: [[
      [190, 622], [282, 608], [302, 650], [254, 686], [192, 678], [190, 622],
    ]],
  },
  {
    captionX: 356,
    captionY: 668,
    printedName: "Shararat (Hutaim)",
    nameAr: "الشرارات",
    rings: [[
      [304, 628], [382, 626], [408, 694], [362, 722], [306, 684], [304, 628],
    ]],
  },
  {
    captionX: 236,
    captionY: 730,
    printedName: "Bani Atiyah",
    nameAr: "بنو عطية",
    rings: [[
      [196, 690], [278, 684], [296, 756], [244, 782], [190, 754], [196, 690],
    ]],
  },
  {
    captionX: 310,
    captionY: 778,
    printedName: "Mawahib",
    nameAr: "المواهيب",
    rings: [[
      [262, 758], [344, 756], [358, 796], [306, 814], [262, 794], [262, 758],
    ]],
  },
  {
    captionX: 432,
    captionY: 690,
    printedName: "W. Sulaiman (Anizah)",
    nameAr: "ولد سليمان",
    rings: [[
      [388, 656], [472, 652], [486, 704], [434, 738], [392, 712], [388, 656],
    ]],
  },
  {
    captionX: 476,
    captionY: 842,
    printedName: "Fugara (Anizah)",
    nameAr: "الفقراء",
    rings: [[
      [414, 792], [478, 798], [526, 874], [486, 896], [432, 842], [414, 792],
    ]],
  },
  {
    captionX: 412,
    captionY: 858,
    printedName: "Bani Wahab (Anizah)",
    nameAr: "بنو وهب",
    rings: [[
      [368, 826], [432, 834], [462, 874], [412, 886], [372, 862], [368, 826],
    ]],
  },
  {
    captionX: 446,
    captionY: 906,
    printedName: "W. Sulaiman (Anizah)",
    nameAr: "ولد سليمان",
    rings: [[
      [372, 874], [478, 884], [524, 916], [456, 954], [388, 938], [372, 874],
    ]],
  },
  {
    captionX: 315,
    captionY: 882,
    printedName: "Billi",
    nameAr: "بلي",
    rings: [[
      [256, 838], [356, 838], [372, 912], [312, 936], [268, 906], [256, 838],
    ]],
  },
  {
    captionX: 356,
    captionY: 984,
    printedName: "Juhainah",
    nameAr: "جهينة",
    rings: [[
      [298, 944], [396, 940], [418, 982], [385, 1012], [338, 1022], [306, 992], [298, 944],
    ]],
  },

  // ── Gulf & Oman ───────────────────────────────────────────────────────
  {
    captionX: 1042,
    captionY: 976,
    printedName: "Al Naim",
    nameAr: "آل نعيم",
    rings: [[
      [1020, 936], [1054, 934], [1074, 998], [1048, 1022], [1024, 992], [1020, 936],
    ]],
  },
  {
    captionX: 1150,
    captionY: 1118,
    printedName: "Manasir",
    nameAr: "المناصير",
    rings: [[
      [1082, 1042], [1186, 1042], [1206, 1126], [1214, 1216], [1166, 1176],
      [1132, 1094], [1082, 1042],
    ]],
  },
  {
    captionX: 1226,
    captionY: 1012,
    printedName: "Bani Yas",
    nameAr: "بنو ياس",
    rings: [[
      [1184, 1022], [1232, 976], [1274, 980], [1252, 1034], [1192, 1046], [1184, 1022],
    ]],
  },
  {
    captionX: 1292,
    captionY: 932,
    printedName: "Qawasim",
    nameAr: "القواسم",
    rings: [[
      [1258, 930], [1298, 902], [1328, 922], [1316, 956], [1272, 962], [1258, 930],
    ]],
  },
  {
    captionX: 1312,
    captionY: 904,
    printedName: "Shihuh",
    nameAr: "الشحوح",
    rings: [[
      [1296, 882], [1326, 876], [1338, 912], [1314, 924], [1294, 904], [1296, 882],
    ]],
  },
  {
    captionX: 1232,
    captionY: 1060,
    printedName: "'Awamir",
    nameAr: "العوامر",
    rings: [[
      [1192, 1046], [1262, 1034], [1276, 1068], [1204, 1084], [1192, 1046],
    ]],
  },
  {
    captionX: 1234,
    captionY: 1094,
    printedName: "Albu Shamis",
    nameAr: "آل بو شامس",
    rings: [[
      [1202, 1084], [1282, 1068], [1294, 1106], [1206, 1126], [1202, 1084],
    ]],
  },
  {
    captionX: 1264,
    captionY: 1134,
    printedName: "Daru",
    nameAr: "الدرع",
    rings: [[
      [1206, 1126], [1318, 1104], [1334, 1142], [1212, 1168], [1206, 1126],
    ]],
  },
  {
    captionX: 1276,
    captionY: 1172,
    printedName: "Harasis",
    nameAr: "الحراسيس",
    rings: [[
      [1212, 1168], [1352, 1142], [1368, 1188], [1214, 1216], [1212, 1168],
    ]],
  },
  {
    captionX: 1320,
    captionY: 1216,
    printedName: "'Afar",
    nameAr: "العفار",
    rings: [[
      [1264, 1204], [1394, 1192], [1398, 1244], [1274, 1252], [1264, 1204],
    ]],
  },
  {
    captionX: 1354,
    captionY: 1082,
    printedName: "Ghafiri and Hinawi (Numerous tribes)",
    nameAr: "الغافرية والهناوية",
    spanAr: "جبال عُمان والداخلية حتى مسقط",
    rings: [[
      [1274, 980], [1342, 984], [1448, 1044], [1464, 1094], [1396, 1152],
      [1318, 1104], [1262, 1034], [1274, 980],
    ]],
  },
  {
    captionX: 1472,
    captionY: 1136,
    printedName: "Ja'alan",
    nameAr: "جعلان",
    rings: [[
      [1448, 1094], [1512, 1094], [1514, 1138], [1468, 1172], [1436, 1144], [1448, 1094],
    ]],
  },
  {
    captionX: 1428,
    captionY: 1190,
    printedName: "Yalwahiba",
    nameAr: "آل وهيبة",
    rings: [[
      [1386, 1178], [1466, 1148], [1474, 1184], [1416, 1226], [1386, 1178],
    ]],
  },
  {
    captionX: 1318,
    captionY: 1292,
    printedName: "Janaba",
    nameAr: "الجنبة",
    rings: [[
      [1238, 1254], [1398, 1244], [1388, 1314], [1328, 1362], [1252, 1344], [1238, 1254],
    ]],
  },

  // ── Southern Hejaz, Asir, Yemen, Hadhramaut & Dhofar ──────────────────
  {
    captionX: 476,
    captionY: 1218,
    printedName: "Hudhail",
    nameAr: "هذيل",
    rings: [[
      [446, 1182], [492, 1182], [504, 1244], [472, 1244], [446, 1182],
    ]],
  },
  {
    captionX: 456,
    captionY: 1252,
    printedName: "Junadlah",
    nameAr: "الجحادلة",
    rings: [[
      [406, 1196], [448, 1206], [496, 1272], [468, 1288], [406, 1230], [406, 1196],
    ]],
  },
  {
    captionX: 524,
    captionY: 1208,
    printedName: "Thaqif",
    nameAr: "ثقيف",
    rings: [[
      [492, 1188], [564, 1188], [564, 1226], [504, 1228], [492, 1188],
    ]],
  },
  {
    captionX: 542,
    captionY: 1254,
    printedName: "Ghamid",
    nameAr: "غامد",
    rings: [[
      [502, 1228], [576, 1228], [588, 1282], [516, 1284], [502, 1228],
    ]],
  },
  {
    captionX: 616,
    captionY: 1244,
    printedName: "Buqum",
    nameAr: "البقوم",
    rings: [[
      [576, 1208], [648, 1224], [654, 1286], [588, 1286], [576, 1208],
    ]],
  },
  {
    captionX: 616,
    captionY: 1318,
    printedName: "Shahran",
    nameAr: "شهران",
    rings: [[
      [568, 1286], [656, 1286], [666, 1344], [574, 1346], [568, 1286],
    ]],
  },
  {
    captionX: 634,
    captionY: 1368,
    printedName: "Rabiah wa Rubaibak",
    nameAr: "ربيعة ورفيدة",
    rings: [[
      [582, 1346], [674, 1346], [684, 1384], [588, 1384], [582, 1346],
    ]],
  },
  {
    captionX: 564,
    captionY: 1406,
    printedName: "Rijalalma",
    nameAr: "رجال ألمع",
    rings: [[
      [524, 1362], [574, 1368], [598, 1436], [556, 1446], [524, 1362],
    ]],
  },
  {
    captionX: 604,
    captionY: 1396,
    printedName: "Mughad",
    nameAr: "بنو مغيد",
    rings: [[
      [574, 1384], [648, 1384], [648, 1412], [582, 1414], [574, 1384],
    ]],
  },
  {
    captionX: 764,
    captionY: 1384,
    printedName: "Bani Yam",
    nameAr: "يام",
    spanAr: "نجران وشمال اليمن",
    rings: [[
      [684, 1352], [858, 1346], [898, 1386], [866, 1422], [704, 1426], [684, 1352],
    ]],
  },
  {
    captionX: 666,
    captionY: 1492,
    printedName: "Hashid",
    nameAr: "حاشد",
    rings: [[
      [632, 1456], [704, 1458], [720, 1518], [672, 1538], [628, 1514], [632, 1456],
    ]],
  },
  {
    captionX: 614,
    captionY: 1554,
    printedName: "Quhrah",
    nameAr: "القحرى",
    rings: [[
      [584, 1532], [652, 1536], [652, 1584], [588, 1584], [584, 1532],
    ]],
  },
  {
    captionX: 674,
    captionY: 1580,
    printedName: "Anis",
    nameAr: "آنس",
    rings: [[
      [648, 1560], [706, 1562], [708, 1598], [646, 1598], [648, 1560],
    ]],
  },
  {
    captionX: 636,
    captionY: 1614,
    printedName: "Zaranik",
    nameAr: "الزرانيق",
    rings: [[
      [598, 1596], [672, 1598], [672, 1632], [608, 1634], [598, 1596],
    ]],
  },
  {
    captionX: 680,
    captionY: 1638,
    printedName: "Amiri",
    nameAr: "الأميري",
    rings: [[
      [656, 1624], [708, 1624], [708, 1656], [656, 1656], [656, 1624],
    ]],
  },
  {
    captionX: 730,
    captionY: 1608,
    printedName: "Yafa'i",
    nameAr: "يافع",
    rings: [[
      [702, 1588], [766, 1588], [768, 1628], [704, 1630], [702, 1588],
    ]],
  },
  {
    captionX: 740,
    captionY: 1648,
    printedName: "Fadhli",
    nameAr: "الفضلي",
    rings: [[
      [710, 1630], [776, 1630], [772, 1666], [712, 1668], [710, 1630],
    ]],
  },
  {
    captionX: 672,
    captionY: 1666,
    printedName: "Haushabi",
    nameAr: "الحوشبي",
    rings: [[
      [640, 1652], [708, 1656], [704, 1684], [648, 1680], [640, 1652],
    ]],
  },
  {
    captionX: 644,
    captionY: 1684,
    printedName: "Subaihi",
    nameAr: "الصبيحي",
    rings: [[
      [614, 1664], [662, 1676], [692, 1702], [634, 1708], [614, 1664],
    ]],
  },
  {
    captionX: 784,
    captionY: 1608,
    printedName: "'Aulaqi",
    nameAr: "العوالق",
    rings: [[
      [764, 1564], [816, 1564], [804, 1654], [764, 1652], [764, 1564],
    ]],
  },
  {
    captionX: 852,
    captionY: 1602,
    printedName: "Wahidi",
    nameAr: "الواحدي",
    rings: [[
      [812, 1572], [898, 1570], [894, 1630], [804, 1634], [812, 1572],
    ]],
  },
  {
    captionX: 916,
    captionY: 1558,
    printedName: "Ka'aiti",
    nameAr: "القعيطي",
    rings: [[
      [874, 1528], [964, 1528], [962, 1588], [898, 1594], [874, 1528],
    ]],
  },
  {
    captionX: 948,
    captionY: 1446,
    printedName: "Sai'ar",
    nameAr: "الصيعر",
    rings: [[
      [884, 1456], [964, 1412], [1024, 1416], [1002, 1468], [892, 1492], [884, 1456],
    ]],
  },
  {
    captionX: 1018,
    captionY: 1456,
    printedName: "Kathiri",
    nameAr: "الكثيري",
    rings: [[
      [964, 1474], [1038, 1422], [1082, 1442], [1014, 1494], [964, 1474],
    ]],
  },
  {
    captionX: 1042,
    captionY: 1492,
    printedName: "Awamir",
    nameAr: "العوامر",
    rings: [[
      [996, 1496], [1084, 1458], [1096, 1496], [1006, 1522], [996, 1496],
    ]],
  },
  {
    captionX: 1048,
    captionY: 1528,
    printedName: "Mahrah",
    nameAr: "المهرة",
    spanAr: "الساحل الجنوبي بين المكلا وظفار",
    rings: [[
      [978, 1532], [1094, 1498], [1124, 1526], [1054, 1564], [978, 1568], [978, 1532],
    ]],
  },
  {
    captionX: 1122,
    captionY: 1384,
    printedName: "Ar-Rashid",
    nameAr: "آل راشد",
    rings: [[
      [1044, 1396], [1142, 1352], [1188, 1372], [1146, 1422], [1048, 1422], [1044, 1396],
    ]],
  },
  {
    captionX: 1184,
    captionY: 1434,
    printedName: "Al Kathir",
    nameAr: "آل كثير",
    rings: [[
      [1106, 1442], [1224, 1404], [1258, 1436], [1132, 1478], [1106, 1442],
    ]],
  },
  {
    captionX: 1232,
    captionY: 1372,
    printedName: "Qara",
    nameAr: "القرا",
    rings: [[
      [1184, 1356], [1268, 1344], [1282, 1384], [1218, 1408], [1184, 1356],
    ]],
  },
];
