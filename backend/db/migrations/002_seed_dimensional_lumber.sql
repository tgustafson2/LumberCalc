INSERT INTO materials (
    owner_id, name, species, grade,
    nominal_thickness_in, nominal_width_in,
    actual_thickness_in, actual_width_in,
    rough_thickness_in, rough_width_in
)
SELECT
    CAST(NULL AS uuid), name,
    CAST(NULL AS text), CAST(NULL AS text),
    nominal_thickness_in, nominal_width_in,
    actual_thickness_in, actual_width_in,
    actual_thickness_in + 0.25, actual_width_in
FROM (VALUES
    ('1x2', 1, 2, 0.75, 1.5),
    ('1x3', 1, 3, 0.75, 2.5),
    ('1x4', 1, 4, 0.75, 3.5),
    ('1x6', 1, 6, 0.75, 5.5),
    ('1x8', 1, 8, 0.75, 7.25),
    ('1x10', 1, 10, 0.75, 9.25),
    ('1x12', 1, 12, 0.75, 11.25),
    ('2x2', 2, 2, 1.5, 1.5),
    ('2x3', 2, 3, 1.5, 2.5),
    ('2x4', 2, 4, 1.5, 3.5),
    ('2x6', 2, 6, 1.5, 5.5),
    ('2x8', 2, 8, 1.5, 7.25),
    ('2x10', 2, 10, 1.5, 9.25),
    ('2x12', 2, 12, 1.5, 11.25),
    ('4x4', 4, 4, 3.5, 3.5),
    ('4x6', 4, 6, 3.5, 5.5),
    ('6x6', 6, 6, 5.5, 5.5)
) AS catalog (name, nominal_thickness_in, nominal_width_in, actual_thickness_in, actual_width_in)
ON CONFLICT (owner_id, name) DO NOTHING;
