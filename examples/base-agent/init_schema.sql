CREATE TABLE metadata (
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    remark TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE memory (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    importance REAL DEFAULT 0.5
);
CREATE TABLE dna (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    question TEXT NOT NULL,
    answer TEXT NOT NULL,
    weight REAL DEFAULT 0.5,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE loop (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    describe TEXT NOT NULL,
    content TEXT NOT NULL,
    weight REAL NOT NULL DEFAULT 0.5 CHECK (weight >= 0 AND weight <= 1),
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE loop_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    loop_id INTEGER NOT NULL,
    page_id TEXT NOT NULL,
    parent_run_id INTEGER,
    seed_name TEXT NOT NULL,
    seed_describe TEXT NOT NULL,
    seed_content TEXT NOT NULL,
    seed_weight REAL NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'completed', 'aborted')),
    plan TEXT NOT NULL DEFAULT 'null',
    progress TEXT NOT NULL DEFAULT 'null',
    result TEXT NOT NULL DEFAULT 'null',
    reflection TEXT NOT NULL DEFAULT 'null',
    abort_reason TEXT,
    started_at TEXT NOT NULL,
    ended_at TEXT,
    reflected_at TEXT,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (loop_id) REFERENCES loop(id),
    FOREIGN KEY (parent_run_id) REFERENCES loop_runs(id)
);

CREATE INDEX idx_loop_runs_page_status
ON loop_runs(page_id, status);

CREATE INDEX idx_loop_runs_parent_status
ON loop_runs(parent_run_id, status);

CREATE INDEX idx_loop_runs_loop_started
ON loop_runs(loop_id, id DESC);

CREATE INDEX idx_loop_runs_loop_ended
ON loop_runs(loop_id, ended_at DESC, id DESC)
WHERE status IN ('completed', 'aborted') AND ended_at IS NOT NULL;

CREATE UNIQUE INDEX idx_loop_runs_one_active_main
ON loop_runs(page_id)
WHERE status = 'active' AND parent_run_id IS NULL;

CREATE TABLE pages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    page_id TEXT NOT NULL,
    cwd TEXT,
    context TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
