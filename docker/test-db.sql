-- Throw-away database for `go test` and e2e/run.sh. Every test run EMPTIES it,
-- and the user below can touch nothing else, so the real database is safe.
-- Run it with `make db-test` (the password is for local development only: the
-- development compose file publishes MySQL on 127.0.0.1 and nowhere else, and
-- the main file does not publish it at all).
CREATE DATABASE IF NOT EXISTS lectern_test CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'lectern_test'@'%' IDENTIFIED BY 'lectern_test';
GRANT ALL ON lectern_test.* TO 'lectern_test'@'%';
