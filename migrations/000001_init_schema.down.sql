DROP INDEX IF EXISTS idx_ledger_kid;
DROP INDEX IF EXISTS idx_task_logs_assigned_status;
DROP INDEX IF EXISTS idx_profiles_family;

DROP TABLE IF EXISTS ledger;
DROP TABLE IF EXISTS task_logs;
DROP TABLE IF EXISTS task_definitions;
DROP TABLE IF EXISTS profiles;
DROP TABLE IF EXISTS families;

DROP TYPE IF EXISTS task_status;
DROP TYPE IF EXISTS task_type;
DROP TYPE IF EXISTS user_role;
