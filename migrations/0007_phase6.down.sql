BEGIN;

ALTER TABLE usage_logs DROP COLUMN IF EXISTS rate_window_id;
ALTER TABLE sessions DROP COLUMN IF EXISTS rate_window_id;
ALTER TABLE hotspots DROP COLUMN IF EXISTS applied_multiplier;

DROP TABLE IF EXISTS config_jobs;
DROP TABLE IF EXISTS anomaly_summaries;
DROP TABLE IF EXISTS anomaly_alerts;
DROP TABLE IF EXISTS rate_windows;
DROP TABLE IF EXISTS payments;

COMMIT;