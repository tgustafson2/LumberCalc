-- material_usable is the only visibility rule for materials.
-- A system row has a null owner. GET /v1/materials, price insert, and design usage insert call this function.
CREATE FUNCTION material_usable(material_owner uuid, caller uuid) RETURNS boolean
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$ SELECT material_owner IS NULL OR material_owner = caller $$;
