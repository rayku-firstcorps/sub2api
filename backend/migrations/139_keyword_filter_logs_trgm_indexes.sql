DO $$
BEGIN
    EXECUTE 'CREATE EXTENSION IF NOT EXISTS pg_trgm';
EXCEPTION WHEN insufficient_privilege OR OTHERS THEN
    RAISE NOTICE 'pg_trgm extension not created (insufficient privileges or already exists): %', SQLERRM;
END
$$;
