-- Stats table for quick per-user assignment counts
CREATE TABLE IF NOT EXISTS stats_assignments (
    user_id TEXT PRIMARY KEY,
    cnt     INTEGER NOT NULL DEFAULT 0
);

-- Backfill from existing data
INSERT INTO stats_assignments(user_id, cnt)
SELECT user_id, COUNT(*)
FROM pr_reviewers
GROUP BY user_id
ON CONFLICT (user_id) DO UPDATE SET cnt = EXCLUDED.cnt;

-- Trigger functions to keep counts in sync
CREATE OR REPLACE FUNCTION pr_reviewers_inc() RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO stats_assignments(user_id, cnt) VALUES (NEW.user_id, 1)
  ON CONFLICT (user_id) DO UPDATE SET cnt = stats_assignments.cnt + 1;
  RETURN NEW;
END; $$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION pr_reviewers_dec() RETURNS TRIGGER AS $$
BEGIN
  UPDATE stats_assignments SET cnt = GREATEST(cnt - 1, 0) WHERE user_id = OLD.user_id;
  RETURN OLD;
END; $$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_pr_reviewers_inc ON pr_reviewers;
CREATE TRIGGER trg_pr_reviewers_inc
AFTER INSERT ON pr_reviewers
FOR EACH ROW EXECUTE FUNCTION pr_reviewers_inc();

DROP TRIGGER IF EXISTS trg_pr_reviewers_dec ON pr_reviewers;
CREATE TRIGGER trg_pr_reviewers_dec
AFTER DELETE ON pr_reviewers
FOR EACH ROW EXECUTE FUNCTION pr_reviewers_dec();