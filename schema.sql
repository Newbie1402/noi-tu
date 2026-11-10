CREATE TABLE IF NOT EXISTS word_observations (
    phrase text PRIMARY KEY,
    key_word text NOT NULL,
    accepted boolean NOT NULL,
    observed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS word_observations_key_idx ON word_observations (key_word);

CREATE TABLE IF NOT EXISTS runtime_settings (
    id smallint PRIMARY KEY CHECK (id = 1),
    suggestions_enabled boolean NOT NULL DEFAULT true,
    collector_enabled boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO runtime_settings(id) VALUES (1) ON CONFLICT (id) DO NOTHING;
