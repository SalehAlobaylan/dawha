ALTER TABLE jobs
  ADD COLUMN max_attempts integer NOT NULL DEFAULT 3;
