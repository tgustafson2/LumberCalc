ALTER TABLE patterns
    ADD COLUMN publisher_name text
    CHECK (publisher_name IS NULL OR btrim(publisher_name) <> '');

CREATE FUNCTION pattern_visible(is_public boolean, publisher uuid, viewer uuid)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$ SELECT is_public OR publisher = viewer $$;

CREATE FUNCTION designs_hide_deleted_pattern() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    UPDATE patterns
    SET is_public = false
    WHERE source_design_id = NEW.id AND is_public;
    RETURN NULL;
END $$;

CREATE TRIGGER designs_hide_deleted_pattern
    AFTER UPDATE OF deleted_at ON designs
    FOR EACH ROW
    WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION designs_hide_deleted_pattern();

UPDATE patterns AS p
SET is_public = false
FROM designs AS d
WHERE d.id = p.source_design_id
  AND d.deleted_at IS NOT NULL
  AND p.is_public;
