package domain

import "time"

type TeamMember struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

type Team struct {
	TeamName string       `json:"team_name"`
	Members  []TeamMember `json:"members"`
}

type User struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	TeamName string `json:"team_name"`
	IsActive bool   `json:"is_active"`
}

type PRStatus string

const (
	PROpen   PRStatus = "OPEN"
	PRMerged PRStatus = "MERGED"
)

type PullRequest struct {
	ID        string     `json:"pull_request_id"`
	Name      string     `json:"pull_request_name"`
	AuthorID  string     `json:"author_id"`
	Status    PRStatus   `json:"status"`
	Reviewers []string   `json:"assigned_reviewers"`
	CreatedAt time.Time  `json:"createdAt,omitempty"`
	MergedAt  *time.Time `json:"mergedAt,omitempty"`
}

type PullRequestShort struct {
	ID       string   `json:"pull_request_id"`
	Name     string   `json:"pull_request_name"`
	AuthorID string   `json:"author_id"`
	Status   PRStatus `json:"status"`
}

type APIErrorCode string

const (
	ErrTeamExists  APIErrorCode = "TEAM_EXISTS"
	ErrPRExists    APIErrorCode = "PR_EXISTS"
	ErrPRMerged    APIErrorCode = "PR_MERGED"
	ErrNotAssigned APIErrorCode = "NOT_ASSIGNED"
	ErrNoCandidate APIErrorCode = "NO_CANDIDATE"
	ErrNotFound    APIErrorCode = "NOT_FOUND"
)

type APIError struct {
	Error struct {
		Code    APIErrorCode `json:"code"`
		Message string       `json:"message"`
	} `json:"error"`
}

func NewAPIError(code APIErrorCode, msg string) APIError {
	var e APIError
	e.Error.Code = code
	e.Error.Message = msg
	return e
}
