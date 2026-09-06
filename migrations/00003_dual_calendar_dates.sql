-- +goose Up
-- +goose StatementBegin
ALTER TABLE events ADD COLUMN secondary_calendar_type TEXT CHECK (secondary_calendar_type IS NULL OR secondary_calendar_type = 'gregorian');
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE events ADD COLUMN secondary_month INTEGER CHECK (secondary_month IS NULL OR secondary_month BETWEEN 1 AND 12);
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE events ADD COLUMN secondary_day INTEGER CHECK (secondary_day IS NULL OR secondary_day BETWEEN 1 AND 31);
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE events ADD COLUMN secondary_next_occurrence DATE;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS ix_events_secondary_next ON events (secondary_next_occurrence) WHERE deleted_at IS NULL AND is_active = 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS ix_events_secondary_next;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE events DROP COLUMN secondary_next_occurrence;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE events DROP COLUMN secondary_day;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE events DROP COLUMN secondary_month;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE events DROP COLUMN secondary_calendar_type;
-- +goose StatementEnd
