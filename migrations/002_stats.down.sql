DROP TRIGGER IF EXISTS trg_pr_reviewers_inc ON pr_reviewers;
DROP TRIGGER IF EXISTS trg_pr_reviewers_dec ON pr_reviewers;
DROP FUNCTION IF EXISTS pr_reviewers_inc();
DROP FUNCTION IF EXISTS pr_reviewers_dec();
DROP TABLE IF EXISTS stats_assignments;