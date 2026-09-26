package dashboard

type Dashboard struct {
	Mode string `json:"mode"`
	// Demo states that what follows is synthetic, in the payload as well as in
	// the response. A client that renders this without reading `mode` still has
	// the word "demo" in the data it is holding, which is the difference between
	// a labelled fixture and an unlabelled one.
	Demo          bool        `json:"demo"`
	WorkspaceName string      `json:"workspaceName"`
	Metrics       []Metric    `json:"metrics"`
	Activity      []Activity  `json:"activity"`
	OpenQuestions []Question  `json:"openQuestions"`
	TreePreview   TreePreview `json:"treePreview"`
	LayerLegend   []Layer     `json:"layerLegend"`
}

type Metric struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Detail string `json:"detail"`
	Tone   string `json:"tone"`
}

type Activity struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	SourceCount int    `json:"sourceCount"`
	UpdatedAt   string `json:"updatedAt"`
}

type Question struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Priority   string `json:"priority"`
	ClaimCount int    `json:"claimCount"`
	UpdatedAt  string `json:"updatedAt"`
}

type TreePreview struct {
	Title         string `json:"title"`
	Version       string `json:"version"`
	State         string `json:"state"`
	People        int    `json:"people"`
	Relationships int    `json:"relationships"`
	Unresolved    int    `json:"unresolved"`
}

type Layer struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Tone        string `json:"tone"`
}

func Demo() Dashboard {
	return Dashboard{
		Mode:          "demo",
		Demo:          true,
		WorkspaceName: "مساحة نجم",
		Metrics: []Metric{
			{Label: "المصادر المفهرسة", Value: "1,248", Detail: "+18 هذا الشهر", Tone: "source"},
			{Label: "الأشجار المنشورة", Value: "376", Detail: "من 42 منطقة", Tone: "interpretation"},
			{Label: "أسئلة مفتوحة", Value: "89", Detail: "17 تحتاج مراجعة", Tone: "question"},
			{Label: "ادعاءات متنازع عليها", Value: "17", Detail: "لا تُحسم تلقائياً", Tone: "disputed"},
		},
		Activity: []Activity{
			{ID: "q-1", Title: "من كان والد عبدالله؟", Description: "تظهر روايتان متعارضتان، وتحتاجان إلى فحص الاعتماد بين المصادر.", Kind: "question", Status: "قيد التحقيق", SourceCount: 4, UpdatedAt: "منذ ساعتين"},
			{ID: "c-1", Title: "أصل فرع الشعبة", Description: "ادعاء جديد يربط الفرع بعائلة أخرى، ويحتاج إلى مصدر مستقل.", Kind: "claim", Status: "مدعوم مبدئياً", SourceCount: 2, UpdatedAt: "أمس"},
			{ID: "f-1", Title: "ترتيب زمني يستحق النظر", Description: "الفارق بين تاريخَي شخصيتين يتعارض مع ترتيب جيل تقريبي.", Kind: "finding", Status: "يحتاج مراجعة", SourceCount: 3, UpdatedAt: "منذ 3 أيام"},
		},
		OpenQuestions: []Question{
			{ID: "q-1", Title: "من كان والد عبدالله في هذه الروايات؟", Status: "قيد التحقيق", Priority: "عالية", ClaimCount: 2, UpdatedAt: "منذ ساعتين"},
			{ID: "q-2", Title: "هل السجلان يمثّلان الشخص نفسه؟", Status: "مفتوحة", Priority: "متوسطة", ClaimCount: 3, UpdatedAt: "أمس"},
			{ID: "q-3", Title: "أين كانت نقطة الانتقال من الأحساء؟", Status: "مفتوحة", Priority: "عالية", ClaimCount: 1, UpdatedAt: "منذ 4 أيام"},
		},
		TreePreview: TreePreview{Title: "شجرة بيت العنبر", Version: "v3", State: "منشورة", People: 48, Relationships: 57, Unresolved: 4},
		LayerLegend: []Layer{
			{Label: "المصدر يقول", Description: "ما نصّ عليه مصدر", Tone: "source"},
			{Label: "ادعاء قيد المراجعة", Description: "استنتاج باحث أو مساهم", Tone: "claim"},
			{Label: "تفسير الشجرة", Description: "ما تعرضه نسخة محفوظة", Tone: "interpretation"},
			{Label: "سؤال لم يُحسم", Description: "ثغرة أو خلاف مفتوح", Tone: "question"},
		},
	}
}
