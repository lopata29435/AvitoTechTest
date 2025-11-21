package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/example/avito-pr-service/internal/domain"
	"github.com/example/avito-pr-service/internal/repo"
	"github.com/example/avito-pr-service/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	svc *service.Service
}

func NewRouter(pool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()
	s := &Server{svc: service.New(repo.New(pool))}

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("healthz write error: %v", err)
		}
	})

	r.Post("/team/add", s.handleTeamAdd)
	r.Get("/team/get", s.handleTeamGet)
	r.Post("/users/setIsActive", s.handleSetIsActive)

	r.Post("/pullRequest/create", s.handlePRCreate)
	r.Post("/pullRequest/merge", s.handlePRMerge)
	r.Post("/pullRequest/reassign", s.handlePRReassign)
	r.Get("/users/getReview", s.handleUserGetReview)

	r.Get("/stats/assignments", s.handleStatsAssignments)
	r.Post("/team/deactivateUsers", s.handleTeamDeactivate)
	return r
}

func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func respondError(w http.ResponseWriter, httpCode int, code domain.APIErrorCode, message string) {
	respondJSON(w, httpCode, domain.NewAPIError(code, message))
}

func (s *Server) handleTeamAdd(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		TeamName string              `json:"team_name"`
		Members  []domain.TeamMember `json:"members"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	team, err := s.svc.CreateTeam(r.Context(), domain.Team{TeamName: payload.TeamName, Members: payload.Members})
	if err != nil {
		if strings.Contains(err.Error(), string(domain.ErrTeamExists)) {
			respondError(w, http.StatusBadRequest, domain.ErrTeamExists, "team_name already exists")
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusCreated, map[string]any{"team": team})
}

func (s *Server) handleTeamGet(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("team_name")
	if name == "" {
		http.Error(w, "team_name required", http.StatusBadRequest)
		return
	}
	team, err := s.svc.GetTeam(r.Context(), name)
	if err != nil {
		if strings.Contains(err.Error(), string(domain.ErrNotFound)) {
			respondError(w, http.StatusNotFound, domain.ErrNotFound, "team not found")
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, team)
}

func (s *Server) handleSetIsActive(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UserID   string `json:"user_id"`
		IsActive bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, err := s.svc.SetUserActive(r.Context(), payload.UserID, payload.IsActive)
	if err != nil {
		if strings.Contains(err.Error(), string(domain.ErrNotFound)) {
			respondError(w, http.StatusNotFound, domain.ErrNotFound, "user not found")
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handlePRCreate(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ID     string `json:"pull_request_id"`
		Name   string `json:"pull_request_name"`
		Author string `json:"author_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	pr, err := s.svc.CreatePR(r.Context(), payload.ID, payload.Name, payload.Author)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), string(domain.ErrPRExists)):
			respondError(w, http.StatusConflict, domain.ErrPRExists, "PR id already exists")
			return
		case strings.Contains(err.Error(), string(domain.ErrNotFound)):
			respondError(w, http.StatusNotFound, domain.ErrNotFound, "author or team not found")
			return
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	respondJSON(w, http.StatusCreated, map[string]any{"pr": pr})
}

func (s *Server) handlePRMerge(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ID string `json:"pull_request_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	pr, err := s.svc.MergePR(r.Context(), payload.ID)
	if err != nil {
		if strings.Contains(err.Error(), string(domain.ErrNotFound)) {
			respondError(w, http.StatusNotFound, domain.ErrNotFound, "PR not found")
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"pr": pr})
}

func (s *Server) handlePRReassign(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ID  string `json:"pull_request_id"`
		Old string `json:"old_user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	pr, replacedBy, err := s.svc.ReassignReviewer(r.Context(), payload.ID, payload.Old)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), string(domain.ErrNotFound)):
			respondError(w, http.StatusNotFound, domain.ErrNotFound, "PR or user not found")
			return
		case strings.Contains(err.Error(), string(domain.ErrPRMerged)):
			respondError(w, http.StatusConflict, domain.ErrPRMerged, "cannot reassign on merged PR")
			return
		case strings.Contains(err.Error(), string(domain.ErrNotAssigned)):
			respondError(w, http.StatusConflict, domain.ErrNotAssigned, "reviewer is not assigned to this PR")
			return
		case strings.Contains(err.Error(), string(domain.ErrNoCandidate)):
			respondError(w, http.StatusConflict, domain.ErrNoCandidate, "no active replacement candidate in team")
			return
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	respondJSON(w, http.StatusOK, map[string]any{"pr": pr, "replaced_by": replacedBy})
}

func (s *Server) handleUserGetReview(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("user_id")
	if uid == "" {
		http.Error(w, "user_id required", http.StatusBadRequest)
		return
	}
	prs, err := s.svc.PRsForReviewer(r.Context(), uid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"user_id": uid, "pull_requests": prs})
}

func (s *Server) handleStatsAssignments(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.AssignmentStats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, it := range rows {
		out = append(out, map[string]any{"user_id": it.UserID, "count": it.Count})
	}
	respondJSON(w, http.StatusOK, map[string]any{"assignments": out})
}

func (s *Server) handleTeamDeactivate(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Team string `json:"team_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	reassigned, removed, err := s.svc.MassDeactivate(r.Context(), payload.Team)
	if err != nil {
		if strings.Contains(err.Error(), string(domain.ErrNotFound)) {
			respondError(w, http.StatusNotFound, domain.ErrNotFound, "team not found")
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"team_name": payload.Team, "reassigned": reassigned, "removed": removed})
}
