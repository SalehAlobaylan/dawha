-- Move the reserved local development account out of the test-fixture domain.
--
-- The quick-sign-in shortcut signs in as one reserved account,
-- local-reviewer@dawha.test, and that account is meant to persist in a
-- developer's own database: it is created by pressing a button, and it is
-- meant to still be there tomorrow.
--
-- `@dawha.test` is also one of the two domains `testsupport`.
-- SyntheticEmailSuffixes reserves for the actors a FIXTURE creates, and the
-- database gate fails a run when a row carrying one of those markers
-- survives the suite. The reserved account is not a fixture actor, so the gate
-- read it as a leaked test actor and failed the acceptance run for a
-- developer who had signed in locally and nothing else.
--
-- The address moves instead. The account keeps its id, its sessions, its
-- sources and its audit trail, because none of those reference the address;
-- only `users.email` changes, and only for this one row.
--
-- This is idempotent, and it is a no-op on a database that never had the old
-- address.

UPDATE users
   SET email = 'local-reviewer@dawha.local',
       updated_at = now()
 WHERE email = 'local-reviewer@dawha.test'
   AND email NOT LIKE '%@example.test';

-- The account must not hold a platform role. It is a plain registered user,
-- and the quick sign-in refuses to proceed if that ever stops being true, so
-- a row that has been given a role by hand is removed rather than trusted.
DELETE FROM user_roles
 WHERE role <> 'registered'
   AND user_id = (SELECT id FROM users WHERE email = 'local-reviewer@dawha.local');
