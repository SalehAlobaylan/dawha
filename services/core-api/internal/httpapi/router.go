package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/collaboration"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/contradiction"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/dashboard"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/dictionary"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/entityresolution"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geography"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/health"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/questions"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/research"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/search"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/suggestions"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/temporalanalysis"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/trees"
	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	DB            *pgxpool.Pool
	Logger        *slog.Logger
	WebOrigin     string
	SecureCookies bool
	Jobs          *jobs.Service
	AI            ai.Provider
	SourceStorage storage.Store
}

type normalizeNameRequest struct {
	Value string `json:"value"`
}

type normalizeNameResponse struct {
	Original          string `json:"original"`
	Normalized        string `json:"normalized"`
	Method            string `json:"method"`
	PreservesOriginal bool   `json:"preserves_original"`
}

func NewRouter(dependencies Dependencies) http.Handler {
	mux := http.NewServeMux()
	healthHandler := health.Handler{Pool: dependencies.DB}
	authService := auth.NewService(dependencies.DB)
	authHandler := auth.Handler{Service: authService, SecureCookies: dependencies.SecureCookies}
	treeService := trees.NewService(dependencies.DB)
	treeHandler := treeHandler{Service: treeService, Auth: authService}
	collaborationService := collaboration.NewService(dependencies.DB)
	collaborationHandler := collaborationHandler{Service: collaborationService, Auth: authService}
	evidenceService := evidence.NewService(dependencies.DB)
	evidenceHandler := evidenceHandler{Service: evidenceService, Auth: authService}
	questionService := questions.NewService(dependencies.DB)
	questionHandler := questionHandler{Service: questionService, Auth: authService}
	dictionaryService := dictionary.NewService(dependencies.DB)
	dictionaryHandler := dictionaryHandler{Service: dictionaryService}
	entityResolutionService := entityresolution.NewService(dependencies.DB, dependencies.AI)
	entityResolutionHandler := entityResolutionHandler{Service: entityResolutionService, Auth: authService}
	geographyService := geography.NewService(dependencies.DB)
	geographyHandler := geographyHandler{Service: geographyService}
	searchService := search.NewService(dependencies.DB)
	searchHandler := searchHandler{Service: searchService}
	researchService := research.NewService(dependencies.DB, dependencies.AI)
	researchHandler := researchHandler{Service: researchService, Auth: authService, Logger: dependencies.Logger}
	jobsService := dependencies.Jobs
	if jobsService == nil {
		jobsService = jobs.NewService(dependencies.DB)
	}
	contradictionService := contradiction.NewService(dependencies.DB, jobsService)
	contradictionHandler := contradictionHandler{Service: contradictionService, Auth: authService}
	temporalAnalysisService := temporalanalysis.NewService(dependencies.DB)
	temporalAnalysisHandler := temporalAnalysisHandler{Service: temporalAnalysisService, Auth: authService}
	jobsHandler := jobHandler{Service: jobsService, Auth: authService}
	sourceProcessingService := sourceprocessing.NewService(dependencies.DB, dependencies.SourceStorage, jobsService, dependencies.AI, sourceprocessing.NewTextExtractor())
	sourceProcessingHandler := sourceProcessingHandler{Service: sourceProcessingService, Auth: authService}
	suggestionService := suggestions.NewService(dependencies.DB)
	suggestionHandler := suggestionHandler{Service: suggestionService, Auth: authService}
	mux.HandleFunc("GET /healthz", healthHandler.Live)
	mux.HandleFunc("GET /readyz", healthHandler.Ready)
	mux.HandleFunc("GET /api/v1/dashboard", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, dashboard.Demo())
	})
	mux.HandleFunc("GET /api/v1/trees", treeHandler.list)
	mux.HandleFunc("POST /api/v1/trees", treeHandler.create)
	mux.HandleFunc("GET /api/v1/trees/{treeID}", treeHandler.get)
	mux.HandleFunc("GET /api/v1/trees/{treeID}/versions", treeHandler.versions)
	mux.HandleFunc("GET /api/v1/trees/{treeID}/versions/{versionID}", treeHandler.version)
	mux.HandleFunc("POST /api/v1/trees/{treeID}/fork", treeHandler.fork)
	mux.HandleFunc("GET /api/v1/trees/{treeID}/diff", treeHandler.diff)
	mux.HandleFunc("GET /api/v1/trees/{treeID}/collaborators", collaborationHandler.listCollaborators)
	mux.HandleFunc("POST /api/v1/trees/{treeID}/invitations", collaborationHandler.createInvitation)
	mux.HandleFunc("PATCH /api/v1/trees/{treeID}/collaborators/{userID}", collaborationHandler.updateCollaborator)
	mux.HandleFunc("DELETE /api/v1/trees/{treeID}/collaborators/{userID}", collaborationHandler.removeCollaborator)
	mux.HandleFunc("DELETE /api/v1/trees/{treeID}/invitations/{invitationID}", collaborationHandler.revokeInvitation)
	mux.HandleFunc("GET /api/v1/trees/{treeID}/activity", collaborationHandler.listActivity)
	mux.HandleFunc("GET /api/v1/invitations", collaborationHandler.listInvitations)
	mux.HandleFunc("POST /api/v1/invitations/{token}/accept", collaborationHandler.acceptInvitation)
	mux.HandleFunc("POST /api/v1/trees/{treeID}/people", treeHandler.addPerson)
	mux.HandleFunc("POST /api/v1/trees/{treeID}/relationships", treeHandler.addRelationship)
	mux.HandleFunc("PATCH /api/v1/trees/{treeID}/relationships/{relationshipID}", treeHandler.updateRelationship)
	mux.HandleFunc("POST /api/v1/trees/{treeID}/publish", treeHandler.publish)
	mux.HandleFunc("POST /api/v1/entity-resolution/runs", entityResolutionHandler.run)
	mux.HandleFunc("GET /api/v1/entity-resolution/runs/{runID}", entityResolutionHandler.getRun)
	mux.HandleFunc("GET /api/v1/entity-resolution/candidates", entityResolutionHandler.listCandidates)
	mux.HandleFunc("GET /api/v1/entity-resolution/merges", entityResolutionHandler.listMerges)
	mux.HandleFunc("GET /api/v1/entity-resolution/candidates/{candidateID}", entityResolutionHandler.getCandidate)
	mux.HandleFunc("POST /api/v1/entity-resolution/candidates/{candidateID}/review", entityResolutionHandler.reviewCandidate)
	mux.HandleFunc("POST /api/v1/entity-resolution/candidates/{candidateID}/merge", entityResolutionHandler.mergeCandidate)
	mux.HandleFunc("POST /api/v1/entity-resolution/merges/{mergeID}/reverse", entityResolutionHandler.reverseMerge)
	mux.HandleFunc("POST /api/v1/contradictions/runs", contradictionHandler.start)
	mux.HandleFunc("GET /api/v1/contradictions/runs/{runID}", contradictionHandler.getRun)
	mux.HandleFunc("GET /api/v1/contradictions/findings", contradictionHandler.listFindings)
	mux.HandleFunc("GET /api/v1/contradictions/findings/{findingID}", contradictionHandler.getFinding)
	mux.HandleFunc("POST /api/v1/contradictions/findings/{findingID}/review", contradictionHandler.review)
	mux.HandleFunc("POST /api/v1/temporal-analysis/runs", temporalAnalysisHandler.start)
	mux.HandleFunc("GET /api/v1/temporal-analysis/runs/{runID}", temporalAnalysisHandler.getRun)
	mux.HandleFunc("GET /api/v1/temporal-analysis/findings", temporalAnalysisHandler.listFindings)
	mux.HandleFunc("GET /api/v1/temporal-analysis/findings/{findingID}", temporalAnalysisHandler.getFinding)
	mux.HandleFunc("POST /api/v1/temporal-analysis/findings/{findingID}/review", temporalAnalysisHandler.review)
	mux.HandleFunc("GET /api/v1/research/layers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"mode": "demo",
			"layers": []map[string]string{
				{"id": string(research.SourceStatement), "label_ar": "عبارة المصدر", "role": "source_text"},
				{"id": string(research.ResearchClaim), "label_ar": "ادعاء الباحث", "role": "research_proposition"},
				{"id": string(research.TreeInterpretation), "label_ar": "تفسير الشجرة", "role": "published_interpretation"},
				{"id": string(research.PlatformFinding), "label_ar": "ملاحظة النظام", "role": "analytical_output"},
				{"id": string(research.OpenQuestion), "label_ar": "سؤال مفتوح", "role": "unresolved_research"},
			},
		})
	})
	mux.HandleFunc("GET /api/v1/sources", evidenceHandler.listSources)
	mux.HandleFunc("POST /api/v1/sources", evidenceHandler.createSource)
	mux.HandleFunc("GET /api/v1/sources/{sourceID}", evidenceHandler.getSource)
	mux.HandleFunc("GET /api/v1/sources/{sourceID}/dependencies", evidenceHandler.listDependencies)
	mux.HandleFunc("POST /api/v1/sources/{sourceID}/dependencies", evidenceHandler.createDependency)
	mux.HandleFunc("POST /api/v1/sources/{sourceID}/dependencies/detect", evidenceHandler.detectDependencies)
	mux.HandleFunc("PATCH /api/v1/source-dependencies/{dependencyID}/review", evidenceHandler.reviewDependency)
	mux.HandleFunc("POST /api/v1/sources/{sourceID}/files", sourceProcessingHandler.uploadFile)
	mux.HandleFunc("GET /api/v1/sources/{sourceID}/processing", sourceProcessingHandler.getProcessing)
	mux.HandleFunc("PATCH /api/v1/source-candidates/{candidateID}/review", sourceProcessingHandler.reviewCandidate)
	mux.HandleFunc("POST /api/v1/sources/{sourceID}/passages", evidenceHandler.createPassage)
	mux.HandleFunc("POST /api/v1/sources/{sourceID}/statements", evidenceHandler.createStatement)
	mux.HandleFunc("GET /api/v1/claims", evidenceHandler.listClaims)
	mux.HandleFunc("POST /api/v1/claims", evidenceHandler.createClaim)
	mux.HandleFunc("GET /api/v1/claims/{claimID}", evidenceHandler.getClaim)
	mux.HandleFunc("POST /api/v1/claims/{claimID}/evidence", evidenceHandler.addEvidence)
	mux.HandleFunc("GET /api/v1/questions", questionHandler.listQuestions)
	mux.HandleFunc("POST /api/v1/questions", questionHandler.createQuestion)
	mux.HandleFunc("GET /api/v1/questions/{questionID}", questionHandler.getQuestion)
	mux.HandleFunc("PATCH /api/v1/questions/{questionID}", questionHandler.updateQuestion)
	mux.HandleFunc("POST /api/v1/questions/{questionID}/notes", questionHandler.addNote)
	mux.HandleFunc("POST /api/v1/questions/{questionID}/claims", questionHandler.linkClaim)
	mux.HandleFunc("POST /api/v1/questions/{questionID}/sources", questionHandler.linkSource)
	mux.HandleFunc("POST /api/v1/questions/{questionID}/disputes", questionHandler.linkDispute)
	mux.HandleFunc("POST /api/v1/questions/{questionID}/entities", questionHandler.linkEntity)
	mux.HandleFunc("POST /api/v1/questions/{questionID}/findings", questionHandler.linkFinding)
	mux.HandleFunc("GET /api/v1/disputes", questionHandler.listDisputes)
	mux.HandleFunc("POST /api/v1/disputes", questionHandler.createDispute)
	mux.HandleFunc("GET /api/v1/disputes/{disputeID}", questionHandler.getDispute)
	mux.HandleFunc("PATCH /api/v1/disputes/{disputeID}", questionHandler.updateDispute)
	mux.HandleFunc("POST /api/v1/disputes/{disputeID}/claims", questionHandler.linkDisputeClaim)
	mux.HandleFunc("GET /api/v1/dictionary", dictionaryHandler.index)
	mux.HandleFunc("GET /api/v1/dictionary/{kind}/{id}", dictionaryHandler.detail)
	mux.HandleFunc("GET /api/v1/map", geographyHandler.mapFeatures)
	mux.HandleFunc("GET /api/v1/map/places/{placeID}", geographyHandler.placeMap)
	mux.HandleFunc("GET /api/v1/search", searchHandler.search)
	mux.HandleFunc("GET /api/v1/research/questions/{questionID}/workspace", researchHandler.workspace)
	mux.HandleFunc("GET /api/v1/research/questions/{questionID}/runs", researchHandler.listRuns)
	mux.HandleFunc("GET /api/v1/research/runs/{runID}", researchHandler.getRun)
	mux.HandleFunc("POST /api/v1/research/query", researchHandler.query)
	mux.HandleFunc("GET /api/v1/jobs", jobsHandler.list)
	mux.HandleFunc("POST /api/v1/jobs", jobsHandler.enqueue)
	mux.HandleFunc("POST /api/v1/jobs/claim", jobsHandler.claim)
	mux.HandleFunc("POST /api/v1/jobs/recover-stale", jobsHandler.recover)
	mux.HandleFunc("POST /api/v1/jobs/{jobID}/complete", jobsHandler.complete)
	mux.HandleFunc("POST /api/v1/jobs/{jobID}/fail", jobsHandler.fail)
	mux.HandleFunc("POST /api/v1/suggestions", suggestionHandler.submit)
	mux.HandleFunc("GET /api/v1/trees/{treeID}/suggestions", suggestionHandler.list)
	mux.HandleFunc("PATCH /api/v1/suggestions/{suggestionID}", suggestionHandler.review)
	mux.HandleFunc("POST /api/v1/normalize-name", normalizeName)
	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("GET /api/v1/auth/me", authHandler.Me)
	mux.HandleFunc("POST /api/v1/auth/logout", authHandler.Logout)

	return withRequestID(withCORS(mux, dependencies.WebOrigin))
}

func normalizeName(w http.ResponseWriter, r *http.Request) {
	var request normalizeNameRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.Value) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "value is required"})
		return
	}

	writeJSON(w, http.StatusOK, normalizeNameResponse{
		Original:          request.Value,
		Normalized:        identity.NormalizeArabicName(request.Value),
		Method:            "deterministic_arabic_normalization",
		PreservesOriginal: true,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func withCORS(next http.Handler, allowedOrigin string) http.Handler {
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:5173"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin == allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = time.Now().UTC().Format("20060102150405.000000000")
		}
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r)
	})
}
