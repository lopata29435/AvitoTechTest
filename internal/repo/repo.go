package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Repo struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repo { return &Repo{db: db} }

func (r *Repo) Db() *pgxpool.Pool { return r.db }

func (r *Repo) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if err = fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repo) TeamExists(ctx context.Context, name string) (bool, error) {
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM teams WHERE team_name=$1)`, name).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repo) CreateTeam(ctx context.Context, name string) error {
	_, err := r.db.Exec(ctx, `INSERT INTO teams(team_name) VALUES ($1)`, name)
	return err
}

func (r *Repo) UpsertUser(ctx context.Context, userID, username, team string, active bool) error {
	_, err := r.db.Exec(ctx, `INSERT INTO users(user_id, username, team_name, is_active)
        VALUES ($1,$2,$3,$4)
        ON CONFLICT (user_id) DO UPDATE SET username=EXCLUDED.username, team_name=EXCLUDED.team_name, is_active=EXCLUDED.is_active`, userID, username, team, active)
	return err
}

type TeamMemberRow struct {
	UserID   string
	Username string
	IsActive bool
}

func (r *Repo) GetTeam(ctx context.Context, name string) ([]TeamMemberRow, error) {
	rows, err := r.db.Query(ctx, `SELECT user_id, username, is_active FROM users WHERE team_name=$1 ORDER BY user_id`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []TeamMemberRow{}
	for rows.Next() {
		var m TeamMemberRow
		if err := rows.Scan(&m.UserID, &m.Username, &m.IsActive); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	if len(members) == 0 {
		var exists bool
		if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM teams WHERE team_name=$1)`, name).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
	}
	return members, nil
}

func (r *Repo) SetUserActive(ctx context.Context, userID string, active bool) (string, string, bool, error) {
	var username, team string
	var isActive bool
	err := r.db.QueryRow(ctx, `UPDATE users SET is_active=$2 WHERE user_id=$1 RETURNING username, team_name, is_active`, userID, active).Scan(&username, &team, &isActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, ErrNotFound
	}
	return username, team, isActive, err
}

func (r *Repo) UserTeam(ctx context.Context, userID string) (string, error) {
	var team string
	err := r.db.QueryRow(ctx, `SELECT team_name FROM users WHERE user_id=$1`, userID).Scan(&team)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return team, err
}

func (r *Repo) UserExists(ctx context.Context, userID string) (bool, error) {
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE user_id=$1)`, userID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repo) CreatePR(ctx context.Context, id, name, author string) error {
	_, err := r.db.Exec(ctx, `INSERT INTO pull_requests(pull_request_id, pull_request_name, author_id) VALUES ($1,$2,$3)`, id, name, author)
	return err
}

func (r *Repo) PRExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pull_requests WHERE pull_request_id=$1)`, id).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repo) PRAuthor(ctx context.Context, id string) (string, error) {
	var author string
	err := r.db.QueryRow(ctx, `SELECT author_id FROM pull_requests WHERE pull_request_id=$1`, id).Scan(&author)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return author, err
}

func (r *Repo) PRStatus(ctx context.Context, id string) (string, error) {
	var status string
	err := r.db.QueryRow(ctx, `SELECT status FROM pull_requests WHERE pull_request_id=$1`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return status, err
}

func (r *Repo) AssignReviewersRandom(ctx context.Context, prID, team, exclude1, exclude2 string, limit int) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT user_id FROM users WHERE team_name=$1 AND is_active=true AND user_id<>$2 AND (CASE WHEN $3='' THEN true ELSE user_id<>$3 END) ORDER BY random() LIMIT $4`, team, exclude1, exclude2, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	batch := pgx.Batch{}
	for _, uid := range ids {
		batch.Queue(`INSERT INTO pr_reviewers(pull_request_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, prID, uid)
	}
	res := r.db.SendBatch(ctx, &batch)
	if err := res.Close(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *Repo) GetPR(ctx context.Context, id string) (name, author, status string, createdAt pgtype.Timestamptz, mergedAt pgtype.Timestamptz, reviewers []string, err error) {
	err = r.db.QueryRow(ctx, `SELECT pull_request_name, author_id, status, created_at, merged_at FROM pull_requests WHERE pull_request_id=$1`, id).Scan(&name, &author, &status, &createdAt, &mergedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", pgtype.Timestamptz{}, pgtype.Timestamptz{}, nil, ErrNotFound
	}
	if err != nil {
		return
	}
	rows, err2 := r.db.Query(ctx, `SELECT user_id FROM pr_reviewers WHERE pull_request_id=$1 ORDER BY user_id`, id)
	if err2 != nil {
		err = err2
		return
	}
	defer rows.Close()
	var rids []string
	for rows.Next() {
		var u string
		if err2 := rows.Scan(&u); err2 != nil {
			err = err2
			return
		}
		rids = append(rids, u)
	}
	reviewers = rids
	return
}

func (r *Repo) MergePR(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE pull_requests SET status='MERGED', merged_at=COALESCE(merged_at, now()) WHERE pull_request_id=$1`, id)
	return err
}

func (r *Repo) ReplaceReviewer(ctx context.Context, prID, oldUser, newUser string) error {
	_, err := r.db.Exec(ctx, `WITH del AS (
            DELETE FROM pr_reviewers WHERE pull_request_id=$1 AND user_id=$2 RETURNING 1
        ) INSERT INTO pr_reviewers(pull_request_id, user_id) VALUES ($1,$3) ON CONFLICT DO NOTHING`, prID, oldUser, newUser)
	return err
}

func (r *Repo) DeleteReviewer(ctx context.Context, prID, userID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM pr_reviewers WHERE pull_request_id=$1 AND user_id=$2`, prID, userID)
	return err
}

func (r *Repo) IsReviewerAssigned(ctx context.Context, prID, userID string) (bool, error) {
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pr_reviewers WHERE pull_request_id=$1 AND user_id=$2)`, prID, userID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repo) RandomReplacementCandidate(ctx context.Context, team, author string, excludeAssigned []string) (string, error) {
	sql := `SELECT user_id FROM users WHERE team_name=$1 AND is_active=true AND user_id<>$2`
	args := []any{team, author}
	if len(excludeAssigned) > 0 {
		sql += ` AND user_id <> ALL($3)`
		args = append(args, excludeAssigned)
	}
	sql += ` ORDER BY random() LIMIT 1`
	var uid string
	err := r.db.QueryRow(ctx, sql, args...).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return uid, err
}

func (r *Repo) PRsForReviewer(ctx context.Context, userID string) ([]struct{ ID, Name, Author, Status string }, error) {
	rows, err := r.db.Query(ctx, `SELECT p.pull_request_id, p.pull_request_name, p.author_id, p.status FROM pull_requests p JOIN pr_reviewers r ON p.pull_request_id=r.pull_request_id WHERE r.user_id=$1 ORDER BY p.pull_request_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct{ ID, Name, Author, Status string }
	for rows.Next() {
		var o struct{ ID, Name, Author, Status string }
		if err := rows.Scan(&o.ID, &o.Name, &o.Author, &o.Status); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

func (r *Repo) AssignmentStats(ctx context.Context) ([]struct {
	UserID string
	Cnt    int
}, error) {
	rows, err := r.db.Query(ctx, `SELECT user_id, cnt FROM stats_assignments ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		UserID string
		Cnt    int
	}
	for rows.Next() {
		var u string
		var c int
		if err := rows.Scan(&u, &c); err != nil {
			return nil, err
		}
		out = append(out, struct {
			UserID string
			Cnt    int
		}{u, c})
	}
	return out, nil
}

func (r *Repo) DeactivateTeamUsers(ctx context.Context, team string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET is_active=false WHERE team_name=$1`, team)
	return err
}

func (r *Repo) OpenPRsAffectedByUsers(ctx context.Context, userIDs []string) ([]struct {
	PRID, Reviewer string
	Author         string
}, error) {
	rows, err := r.db.Query(ctx, `SELECT r.pull_request_id, r.user_id, p.author_id FROM pr_reviewers r JOIN pull_requests p ON p.pull_request_id=r.pull_request_id WHERE r.user_id = ANY($1) AND p.status='OPEN'`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		PRID, Reviewer string
		Author         string
	}
	for rows.Next() {
		var o struct {
			PRID, Reviewer string
			Author         string
		}
		if err := rows.Scan(&o.PRID, &o.Reviewer, &o.Author); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

func (r *Repo) TeamMembers(ctx context.Context, team string, onlyActive bool) ([]string, error) {
	sql := `SELECT user_id FROM users WHERE team_name=$1`
	if onlyActive {
		sql += ` AND is_active=true`
	}
	rows, err := r.db.Query(ctx, sql, team)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
