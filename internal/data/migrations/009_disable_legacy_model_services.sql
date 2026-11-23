-- +goose Up
-- Older experimental Bifrost rows had no virtual key and can never
-- authenticate against the gateway. Keep them for audit but disable them so
-- they are not selectable by new agents.
UPDATE model_services
SET enabled = false, updated_at = now()
WHERE gateway_type = 'bifrost' AND coalesce(gateway_key_id, '') = '';

UPDATE model_models AS m
SET enabled = false, updated_at = now()
FROM model_services AS s
WHERE m.service_id = s.id
  AND s.gateway_type = 'bifrost'
  AND coalesce(s.gateway_key_id, '') = '';

-- +goose Down
-- Data cleanup only.
