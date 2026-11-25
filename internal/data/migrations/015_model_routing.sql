-- +goose Up
-- Fix nested gateway model keys produced by the first version of migration
-- 012 (substring after the first slash, not split_part(... , 2)).
UPDATE model_catalog
SET provider = CASE
        WHEN provider = '' AND position('/' in model_key) > 0 THEN split_part(model_key, '/', 1)
        ELSE provider
    END,
    gateway_model_key = CASE
        WHEN position('/' in model_key) > 0 THEN substring(model_key from position('/' in model_key) + 1)
        ELSE gateway_model_key
    END
WHERE position('/' in model_key) > 0;

-- +goose Down
-- Data-only migration; no destructive down.
