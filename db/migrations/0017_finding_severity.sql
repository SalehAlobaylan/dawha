ALTER TABLE platform_findings
  ADD COLUMN IF NOT EXISTS severity text NOT NULL DEFAULT 'medium' CHECK (severity IN ('low', 'medium', 'high'));
