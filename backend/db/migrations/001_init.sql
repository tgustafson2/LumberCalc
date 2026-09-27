CREATE TABLE users (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    clerk_user_id text        NOT NULL UNIQUE CHECK (clerk_user_id <> ''),
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- The PK is the FK, so one settings row per user is structural.
-- Defaults live here and only here. Go never names a default kerf.
CREATE TABLE user_settings (
    user_id                 uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    default_use_kerf        boolean     NOT NULL DEFAULT false,
    default_kerf_in         numeric     NOT NULL DEFAULT 0.125 CHECK (default_kerf_in > 0),
    default_dimension_basis text        NOT NULL DEFAULT 'nominal'
                            CHECK (default_dimension_basis IN ('nominal', 'actual')),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

-- owner_id NULL is a system material.
CREATE TABLE materials (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id             uuid        REFERENCES users (id) ON DELETE CASCADE,
    name                 text        NOT NULL CHECK (btrim(name) <> ''),
    species              text,
    grade                text,
    nominal_thickness_in numeric     NOT NULL CHECK (nominal_thickness_in > 0),
    nominal_width_in     numeric     NOT NULL CHECK (nominal_width_in > 0),
    actual_thickness_in  numeric     NOT NULL CHECK (actual_thickness_in > 0),
    actual_width_in      numeric     NOT NULL CHECK (actual_width_in > 0),
    rough_thickness_in   numeric     NOT NULL CHECK (rough_thickness_in > 0),
    rough_width_in       numeric     NOT NULL CHECK (rough_width_in > 0),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);
-- Two system materials cannot share a name either.
CREATE UNIQUE INDEX materials_owner_name_key ON materials (owner_id, name) NULLS NOT DISTINCT;

CREATE TABLE prices (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    material_id uuid        NOT NULL REFERENCES materials (id) ON DELETE RESTRICT,
    length_in   numeric     NOT NULL CHECK (length_in > 0),
    price_cents integer     NOT NULL CHECK (price_cents >= 0),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, material_id, length_in)
);
CREATE INDEX prices_material_id_idx ON prices (material_id);

CREATE TABLE designs (
    id                     uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id               uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name                   text        NOT NULL CHECK (btrim(name) <> ''),
    description            text        NOT NULL DEFAULT '',
    schema_version         integer     NOT NULL CHECK (schema_version >= 1),
    document               jsonb       NOT NULL CHECK (jsonb_typeof(document) = 'object'),
    version                integer     NOT NULL DEFAULT 1 CHECK (version >= 1),
    copied_from_design_id  uuid        REFERENCES designs (id) ON DELETE SET NULL,
    copied_from_pattern_id uuid,       -- FK added after patterns exists
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    deleted_at             timestamptz,
    CHECK (num_nonnulls(copied_from_design_id, copied_from_pattern_id) <= 1),
    UNIQUE (id, owner_id)              -- target of the patterns composite FK
);
CREATE INDEX designs_owner_live_idx ON designs (owner_id, updated_at DESC) WHERE deleted_at IS NULL;

-- Rebuilt on every design write. RESTRICT is what turns material delete into 409.
CREATE TABLE design_material_usages (
    design_id   uuid NOT NULL REFERENCES designs (id) ON DELETE CASCADE,
    material_id uuid NOT NULL REFERENCES materials (id) ON DELETE RESTRICT,
    PRIMARY KEY (design_id, material_id)
);
CREATE INDEX design_material_usages_material_id_idx ON design_material_usages (material_id);

-- Usages of this user's designs are deleted by the design cascade, which is
-- still queued when the material RESTRICT check runs. Remove them first.
-- A usage on someone else's design still RESTRICTs.
CREATE FUNCTION users_clear_own_usages() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM design_material_usages AS u
    USING designs AS d
    WHERE u.design_id = d.id AND d.owner_id = OLD.id;
    RETURN OLD;
END $$;

CREATE TRIGGER users_clear_own_usages
    BEFORE DELETE ON users
    FOR EACH ROW EXECUTE FUNCTION users_clear_own_usages();

-- One row per source design, ever. Unpublish flips is_public. No soft delete.
-- The composite FK makes "publisher is the design's owner" impossible to violate.
CREATE TABLE patterns (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_design_id uuid        NOT NULL UNIQUE,
    publisher_id     uuid        NOT NULL,
    name             text        NOT NULL CHECK (btrim(name) <> ''),
    description      text        NOT NULL DEFAULT '',
    schema_version   integer     NOT NULL CHECK (schema_version >= 1),
    document         jsonb       NOT NULL CHECK (jsonb_typeof(document) = 'object'),
    is_public        boolean     NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (source_design_id, publisher_id) REFERENCES designs (id, owner_id) ON DELETE CASCADE
);
CREATE INDEX patterns_public_idx ON patterns (updated_at DESC) WHERE is_public;
CREATE INDEX patterns_publisher_id_idx ON patterns (publisher_id);

ALTER TABLE designs
    ADD CONSTRAINT designs_copied_from_pattern_id_fkey
    FOREIGN KEY (copied_from_pattern_id) REFERENCES patterns (id) ON DELETE SET NULL;

-- The status CHECK makes each lifecycle state carry exactly its own fields.
CREATE TABLE optimization_runs (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    design_id        uuid        NOT NULL REFERENCES designs (id) ON DELETE CASCADE,
    input_hash       char(64)    NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
    status           text        NOT NULL CHECK (status IN ('pending', 'succeeded', 'failed')),
    result           jsonb,
    total_cost_cents integer     CHECK (total_cost_cents >= 0),
    error_code       text        CHECK (error_code <> ''),
    error_detail     jsonb,
    created_at       timestamptz NOT NULL DEFAULT now(),
    finished_at      timestamptz,
    CHECK (CASE status
        WHEN 'pending'   THEN num_nonnulls(result, total_cost_cents, error_code, error_detail, finished_at) = 0
        WHEN 'succeeded' THEN result IS NOT NULL AND total_cost_cents IS NOT NULL
                              AND error_code IS NULL AND error_detail IS NULL AND finished_at IS NOT NULL
        WHEN 'failed'    THEN error_code IS NOT NULL AND result IS NULL AND total_cost_cents IS NULL
                              AND finished_at IS NOT NULL
    END)
);
CREATE INDEX optimization_runs_owner_idx ON optimization_runs (owner_id, created_at DESC);
CREATE INDEX optimization_runs_design_id_idx ON optimization_runs (design_id);
CREATE INDEX optimization_runs_input_hash_idx ON optimization_runs (input_hash) WHERE status = 'succeeded';
