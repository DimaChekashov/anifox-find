CREATE TABLE anime (
    id SERIAL PRIMARY KEY,
    url TEXT,
    title TEXT NOT NULL,
    image TEXT,
    episodes INTEGER,
    aired TEXT,
    synopsis TEXT,
    updated TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
