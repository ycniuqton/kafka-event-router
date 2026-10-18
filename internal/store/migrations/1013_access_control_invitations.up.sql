-- 1013_access_control_invitations: the first login of an invited email takes over the pending user row
-- (and its invited role bindings) instead of inserting a new user; acceptance is stamped for the audit trail.

ALTER TABLE users ADD COLUMN invitation_accepted_at timestamptz;

CREATE FUNCTION users_claim_invitation() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.sso_subject IS NULL THEN
        RETURN NEW;
    END IF;
    UPDATE users SET sso_subject = NEW.sso_subject
    WHERE lower(email) = lower(NEW.email) AND sso_subject IS NULL AND invited_at IS NOT NULL;
    IF FOUND THEN
        RETURN NULL;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER users_claim_invitation BEFORE INSERT ON users
    FOR EACH ROW EXECUTE FUNCTION users_claim_invitation();
