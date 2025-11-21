package service

import (
	"context"
	"errors"

	"github.com/example/avito-pr-service/internal/domain"
	"github.com/example/avito-pr-service/internal/repo"
)

type Service struct{ r *repo.Repo }

func New(r *repo.Repo) *Service { return &Service{r: r} }

func (s *Service) CreateTeam(ctx context.Context, team domain.Team) (domain.Team, error) {
	exists, err := s.r.TeamExists(ctx, team.TeamName)
	if err != nil {
		return domain.Team{}, err
	}
	if exists {
		return domain.Team{}, errors.New(string(domain.ErrTeamExists))
	}
	if err := s.r.CreateTeam(ctx, team.TeamName); err != nil {
		return domain.Team{}, err
	}
	for _, m := range team.Members {
		if err := s.r.UpsertUser(ctx, m.UserID, m.Username, team.TeamName, m.IsActive); err != nil {
			return domain.Team{}, err
		}
	}
	rows, err := s.r.GetTeam(ctx, team.TeamName)
	if err != nil {
		return domain.Team{}, err
	}
	members := make([]domain.TeamMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, domain.TeamMember{UserID: row.UserID, Username: row.Username, IsActive: row.IsActive})
	}
	return domain.Team{TeamName: team.TeamName, Members: members}, nil
}

func (s *Service) GetTeam(ctx context.Context, name string) (domain.Team, error) {
	rows, err := s.r.GetTeam(ctx, name)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.Team{}, errors.New(string(domain.ErrNotFound))
		}
		return domain.Team{}, err
	}
	members := make([]domain.TeamMember, 0, len(rows))
	for _, row := range rows {
		members = append(members, domain.TeamMember{UserID: row.UserID, Username: row.Username, IsActive: row.IsActive})
	}
	return domain.Team{TeamName: name, Members: members}, nil
}

func (s *Service) SetUserActive(ctx context.Context, userID string, active bool) (domain.User, error) {
	username, team, isActive, err := s.r.SetUserActive(ctx, userID, active)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.User{}, errors.New(string(domain.ErrNotFound))
		}
		return domain.User{}, err
	}
	return domain.User{UserID: userID, Username: username, TeamName: team, IsActive: isActive}, nil
}

func (s *Service) CreatePR(ctx context.Context, id, name, author string) (domain.PullRequest, error) {
	exists, err := s.r.PRExists(ctx, id)
	if err != nil {
		return domain.PullRequest{}, err
	}
	if exists {
		return domain.PullRequest{}, errors.New(string(domain.ErrPRExists))
	}
	team, err := s.r.UserTeam(ctx, author)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.PullRequest{}, errors.New(string(domain.ErrNotFound))
		}
		return domain.PullRequest{}, err
	}
	if err := s.r.CreatePR(ctx, id, name, author); err != nil {
		return domain.PullRequest{}, err
	}
	_, err = s.r.AssignReviewersRandom(ctx, id, team, author, "", 2)
	if err != nil {
		return domain.PullRequest{}, err
	}
	return s.GetPR(ctx, id)
}

func (s *Service) GetPR(ctx context.Context, id string) (domain.PullRequest, error) {
	name, author, status, createdAt, mergedAt, reviewers, err := s.r.GetPR(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.PullRequest{}, errors.New(string(domain.ErrNotFound))
		}
		return domain.PullRequest{}, err
	}
	pr := domain.PullRequest{ID: id, Name: name, AuthorID: author, Status: domain.PRStatus(status), Reviewers: reviewers}
	if createdAt.Valid {
		pr.CreatedAt = createdAt.Time
	}
	if mergedAt.Valid {
		t := mergedAt.Time
		pr.MergedAt = &t
	}
	return pr, nil
}

func (s *Service) MergePR(ctx context.Context, id string) (domain.PullRequest, error) {
	if _, err := s.r.PRStatus(ctx, id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.PullRequest{}, errors.New(string(domain.ErrNotFound))
		}
		return domain.PullRequest{}, err
	}
	if err := s.r.MergePR(ctx, id); err != nil {
		return domain.PullRequest{}, err
	}
	return s.GetPR(ctx, id)
}

func (s *Service) ReassignReviewer(ctx context.Context, prID, oldUser string) (domain.PullRequest, string, error) {
	status, err := s.r.PRStatus(ctx, prID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.PullRequest{}, "", errors.New(string(domain.ErrNotFound))
		}
		return domain.PullRequest{}, "", err
	}
	if status == string(domain.PRMerged) {
		return domain.PullRequest{}, "", errors.New(string(domain.ErrPRMerged))
	}
	assigned, err := s.r.IsReviewerAssigned(ctx, prID, oldUser)
	if err != nil {
		return domain.PullRequest{}, "", err
	}
	if !assigned {
		return domain.PullRequest{}, "", errors.New(string(domain.ErrNotAssigned))
	}
	pr, err := s.GetPR(ctx, prID)
	if err != nil {
		return domain.PullRequest{}, "", err
	}
	team, err := s.r.UserTeam(ctx, oldUser)
	if err != nil {
		return domain.PullRequest{}, "", err
	}
	exclude := append([]string{}, pr.Reviewers...)
	exclude = append(exclude, oldUser)
	uid, err := s.r.RandomReplacementCandidate(ctx, team, pr.AuthorID, exclude)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return domain.PullRequest{}, "", errors.New(string(domain.ErrNoCandidate))
		}
		return domain.PullRequest{}, "", err
	}
	if err := s.r.ReplaceReviewer(ctx, prID, oldUser, uid); err != nil {
		return domain.PullRequest{}, "", err
	}
	updated, err := s.GetPR(ctx, prID)
	return updated, uid, err
}

func (s *Service) PRsForReviewer(ctx context.Context, userID string) ([]domain.PullRequestShort, error) {
	rows, err := s.r.PRsForReviewer(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PullRequestShort, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.PullRequestShort{ID: r.ID, Name: r.Name, AuthorID: r.Author, Status: domain.PRStatus(r.Status)})
	}
	return out, nil
}

func (s *Service) AssignmentStats(ctx context.Context) ([]struct {
	UserID string
	Count  int
}, error) {
	rows, err := s.r.AssignmentStats(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]struct {
		UserID string
		Count  int
	}, 0, len(rows))
	for _, r := range rows {
		out = append(out, struct {
			UserID string
			Count  int
		}{UserID: r.UserID, Count: r.Cnt})
	}
	return out, nil
}

func (s *Service) MassDeactivate(ctx context.Context, team string) (int, int, error) {
	activeBefore, err := s.r.TeamMembers(ctx, team, true)
	if err != nil {
		return 0, 0, err
	}
	if len(activeBefore) == 0 {
		allMembers, err := s.r.TeamMembers(ctx, team, false)
		if err != nil {
			return 0, 0, err
		}
		if len(allMembers) == 0 {
			return 0, 0, errors.New(string(domain.ErrNotFound))
		}
		return 0, 0, nil
	}

	if err := s.r.DeactivateTeamUsers(ctx, team); err != nil {
		return 0, 0, err
	}

	affected, err := s.r.OpenPRsAffectedByUsers(ctx, activeBefore)
	if err != nil {
		return 0, 0, err
	}

	reassigned := 0
	removed := 0
	for _, a := range affected {
		pr, err := s.GetPR(ctx, a.PRID)
		if err != nil {
			return 0, 0, err
		}
		candidate, err := s.r.RandomReplacementCandidate(ctx, team, pr.AuthorID, pr.Reviewers)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				if derr := s.r.DeleteReviewer(ctx, a.PRID, a.Reviewer); derr != nil {
					return 0, 0, derr
				}
				removed++
				continue
			}
			return 0, 0, err
		}
		if err := s.r.ReplaceReviewer(ctx, a.PRID, a.Reviewer, candidate); err != nil {
			return 0, 0, err
		}
		reassigned++
	}
	return reassigned, removed, nil
}
