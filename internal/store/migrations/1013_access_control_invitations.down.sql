DROP TRIGGER IF EXISTS users_claim_invitation ON users;
DROP FUNCTION IF EXISTS users_claim_invitation();
ALTER TABLE users DROP COLUMN IF EXISTS invitation_accepted_at;
