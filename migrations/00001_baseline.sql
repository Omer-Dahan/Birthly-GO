-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS app_meta (
    key   TEXT PRIMARY KEY,
    value TEXT
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS users (
    id                          INTEGER PRIMARY KEY,
    username                    TEXT,
    first_name                  TEXT NOT NULL,
    last_name                   TEXT,
    language                    TEXT NOT NULL DEFAULT 'he',
    timezone                    TEXT NOT NULL DEFAULT 'Asia/Jerusalem',
    date_format                 TEXT NOT NULL DEFAULT 'DD/MM/YYYY',
    time_format                 TEXT NOT NULL DEFAULT '24h',
    default_notify_time         TEXT NOT NULL DEFAULT '09:00',
    notifications_enabled       BOOLEAN NOT NULL DEFAULT 1,
    silent_notifications        BOOLEAN NOT NULL DEFAULT 0,
    show_hebrew_date            BOOLEAN NOT NULL DEFAULT 1,
    daily_digest_enabled        BOOLEAN NOT NULL DEFAULT 0,
    digest_time                 TEXT NOT NULL DEFAULT '08:00',
    list_sort                   TEXT NOT NULL DEFAULT 'upcoming',
    list_filter                 TEXT,
    adar_policy                 TEXT NOT NULL DEFAULT 'adar_ii',
    feb29_policy                TEXT NOT NULL DEFAULT 'feb28',
    is_admin                    BOOLEAN NOT NULL DEFAULT 0,
    is_blocked                  BOOLEAN NOT NULL DEFAULT 0,
    bot_blocked_by_user         BOOLEAN NOT NULL DEFAULT 0,
    onboarded                   BOOLEAN NOT NULL DEFAULT 0,
    created_at                  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at                DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT lang_valid CHECK (language IN ('he','en')),
    CONSTRAINT df_valid CHECK (date_format IN ('DD/MM/YYYY','DD.MM.YYYY','YYYY-MM-DD')),
    CONSTRAINT tf_valid CHECK (time_format IN ('24h','12h')),
    CONSTRAINT list_sort_valid CHECK (list_sort IN ('upcoming','name','age','created')),
    CONSTRAINT adar_policy_valid CHECK (adar_policy IN ('adar_i','adar_ii')),
    CONSTRAINT feb29_policy_valid CHECK (feb29_policy IN ('feb28','mar01'))
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action     TEXT NOT NULL,
    entity     TEXT NOT NULL,
    entity_id  INTEGER,
    payload    TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS ix_audit_user_time ON audit_logs (user_id, created_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS backups (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER REFERENCES users(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,
    format       TEXT NOT NULL,
    path         TEXT NOT NULL,
    size_bytes   INTEGER NOT NULL DEFAULT 0,
    events_count INTEGER NOT NULL DEFAULT 0,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT backup_kind_valid CHECK (kind IN ('auto','manual','export')),
    CONSTRAINT backup_format_valid CHECK (format IN ('db','json','csv','xlsx'))
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS events (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id            INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type         TEXT NOT NULL DEFAULT 'birthday',
    custom_type_label  TEXT,
    first_name         TEXT NOT NULL,
    last_name          TEXT,
    nickname           TEXT,
    gender             TEXT,
    relation           TEXT,
    category           TEXT NOT NULL DEFAULT 'other',
    calendar_type      TEXT NOT NULL DEFAULT 'gregorian',
    year               INTEGER,
    month              INTEGER NOT NULL,
    day                INTEGER NOT NULL,
    event_time         TEXT,
    phone              TEXT,
    telegram_username  TEXT,
    photo_file_id      TEXT,
    notes              TEXT,
    next_occurrence    DATE,
    is_active          BOOLEAN NOT NULL DEFAULT 1,
    deleted_at         DATETIME,
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT event_type_valid CHECK (event_type IN ('birthday','anniversary','wedding','memorial','custom')),
    CONSTRAINT calendar_type_valid CHECK (calendar_type IN ('gregorian','hebrew')),
    CONSTRAINT month_range CHECK (month BETWEEN 1 AND 13),
    CONSTRAINT day_range CHECK (day BETWEEN 1 AND 31),
    CONSTRAINT year_range CHECK (year IS NULL OR (year BETWEEN 1900 AND 2100) OR (year BETWEEN 5660 AND 5860)),
    CONSTRAINT gender_valid CHECK (gender IS NULL OR gender IN ('m','f','other')),
    CONSTRAINT category_valid CHECK (category IN ('family','friends','work','clients','school','other'))
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS ix_events_user ON events (user_id) WHERE deleted_at IS NULL;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS ix_events_next ON events (next_occurrence) WHERE deleted_at IS NULL AND is_active = 1;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS ix_events_user_next ON events (user_id, next_occurrence);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS ix_events_user_month ON events (user_id, month);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS ix_events_name ON events (user_id, first_name, last_name);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS reminder_rules (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_id       INTEGER REFERENCES events(id) ON DELETE CASCADE,
    offset_days    INTEGER,
    offset_minutes INTEGER,
    send_time      TEXT,
    enabled        BOOLEAN NOT NULL DEFAULT 1,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT exactly_one_offset CHECK ((offset_days IS NOT NULL) <> (offset_minutes IS NOT NULL))
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS ix_rules_user ON reminder_rules (user_id, enabled);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS ix_rules_event ON reminder_rules (event_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS notifications_log (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_id         INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    rule_id          INTEGER REFERENCES reminder_rules(id) ON DELETE SET NULL,
    occurrence_date  DATE NOT NULL,
    scheduled_at     DATETIME NOT NULL,
    sent_at          DATETIME,
    status           TEXT NOT NULL DEFAULT 'pending',
    error            TEXT,
    attempts         INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT notif_status_valid CHECK (status IN ('pending','sent','failed','skipped')),
    CONSTRAINT uq_notif UNIQUE (event_id, rule_id, occurrence_date)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS ix_notif_sched ON notifications_log (status, scheduled_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS greeting_templates (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER REFERENCES users(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    tone       TEXT NOT NULL,
    gender     TEXT,
    language   TEXT NOT NULL,
    body       TEXT NOT NULL,
    is_active  BOOLEAN NOT NULL DEFAULT 1,
    CONSTRAINT tpl_event_type_valid CHECK (event_type IN ('birthday','anniversary','wedding','memorial','custom')),
    CONSTRAINT tpl_tone_valid CHECK (tone IN ('warm','funny','formal','short')),
    CONSTRAINT tpl_gender_valid CHECK (gender IS NULL OR gender IN ('m','f')),
    CONSTRAINT tpl_lang_valid CHECK (language IN ('he','en'))
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO app_meta (key, value)
SELECT 'schema_version', '1'
WHERE NOT EXISTS (SELECT 1 FROM app_meta WHERE key = 'schema_version');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS greeting_templates;
DROP TABLE IF EXISTS notifications_log;
DROP TABLE IF EXISTS reminder_rules;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS backups;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS app_meta;
-- +goose StatementEnd
