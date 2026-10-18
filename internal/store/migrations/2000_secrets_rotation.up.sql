-- 2000_secrets_rotation: master-key rotation (FR-29) pages through envelopes whose key_id differs
-- from the current master key, per provider, in ref order.

CREATE INDEX secrets_rotation_idx ON secrets (provider, key_id, ref);
