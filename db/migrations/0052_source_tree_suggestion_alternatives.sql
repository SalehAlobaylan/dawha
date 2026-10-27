-- The reader's OWN alternatives, kept beside its chosen reading.
--
-- A suggestion is a hypothesis, and the reader frequently says "this could be
-- محمد or أحمد" without being able to choose. Those alternatives are the most
-- useful AI assistance the review screen can offer: a person can pick the right
-- one with one click instead of typing the whole name in Arabic. They are not
-- facts, they are not merged into the reading, and the reviewer's decision is
-- still the only thing that creates a name.
--
-- An empty array is the normal case for a reading that was not ambiguous.
ALTER TABLE source_tree_suggestions
  ADD COLUMN alternatives jsonb NOT NULL DEFAULT '[]'::jsonb
  CHECK (jsonb_typeof(alternatives) = 'array');
