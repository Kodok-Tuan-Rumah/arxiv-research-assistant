-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Papers table
CREATE TABLE IF NOT EXISTS papers (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  arxiv_id    VARCHAR(20) UNIQUE NOT NULL,
  title       TEXT NOT NULL,
  authors     TEXT[],
  abstract    TEXT,
  categories  TEXT[],
  published   DATE,
  ingested_at TIMESTAMPTZ DEFAULT NOW()
);

-- Chunks table
CREATE TABLE IF NOT EXISTS chunks (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  paper_id    UUID REFERENCES papers(id) ON DELETE CASCADE,
  chunk_index INT NOT NULL,
  text        TEXT NOT NULL,
  qdrant_id   UUID,
  char_count  INT,
  created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Query logs table
CREATE TABLE IF NOT EXISTS query_logs (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  query              TEXT NOT NULL,
  answer             TEXT,
  retrieval_ms       INT,
  generation_ms      INT,
  faithfulness       FLOAT,
  failure_mode       VARCHAR(50),
  model              VARCHAR(50),
  retrieval_strategy VARCHAR(50),
  chunk_size_used    INT,
  session_id         VARCHAR(100),
  request_id         TEXT,
  created_at         TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_papers_arxiv_id ON papers(arxiv_id);
CREATE INDEX IF NOT EXISTS idx_chunks_paper_id ON chunks(paper_id);
CREATE INDEX IF NOT EXISTS idx_query_logs_created_at ON query_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_query_logs_request_id ON query_logs(request_id);
CREATE INDEX IF NOT EXISTS idx_query_logs_failure_mode ON query_logs(failure_mode);

-- Prompt versions table
CREATE TABLE IF NOT EXISTS prompt_versions (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  version     VARCHAR(20) NOT NULL UNIQUE,
  name        VARCHAR(100) NOT NULL,
  template    TEXT NOT NULL,
  description TEXT,
  is_active   BOOLEAN DEFAULT FALSE,
  created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Embedding versions table
CREATE TABLE IF NOT EXISTS embedding_versions (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  model_name  VARCHAR(100) NOT NULL,
  model_dim   INT NOT NULL,
  batch_id    UUID DEFAULT gen_random_uuid(),
  paper_count INT DEFAULT 0,
  chunk_count INT DEFAULT 0,
  created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Version tracking columns on query_logs
ALTER TABLE query_logs ADD COLUMN IF NOT EXISTS prompt_version VARCHAR(20) DEFAULT 'v1.0';
ALTER TABLE query_logs ADD COLUMN IF NOT EXISTS embedding_version VARCHAR(100) DEFAULT 'all-MiniLM-L6-v2';
