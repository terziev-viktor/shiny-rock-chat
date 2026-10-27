CREATE TABLE users (
    account_id UUID PRIMARY KEY,
    public_key BYTEA NOT NULL UNIQUE CHECK (octet_length(public_key) = 32),
    key_fingerprint CHAR(64) NOT NULL UNIQUE CHECK (key_fingerprint ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    qr_payload TEXT NOT NULL UNIQUE,
    shiny_rock BIGINT NOT NULL DEFAULT 0 CHECK (shiny_rock >= 0)
);

CREATE FUNCTION prevent_user_identity_change() RETURNS trigger AS $$
BEGIN
    IF NEW.account_id IS DISTINCT FROM OLD.account_id
        OR NEW.public_key IS DISTINCT FROM OLD.public_key
        OR NEW.key_fingerprint IS DISTINCT FROM OLD.key_fingerprint
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
        OR NEW.qr_payload IS DISTINCT FROM OLD.qr_payload THEN
        RAISE EXCEPTION 'user identity and QR payload are immutable'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER users_identity_immutable
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION prevent_user_identity_change();

CREATE TABLE auth_challenges (
    challenge_id UUID PRIMARY KEY,
    public_key BYTEA NOT NULL CHECK (octet_length(public_key) = 32),
    purpose TEXT NOT NULL CHECK (purpose IN ('register', 'login')),
    nonce BYTEA NOT NULL CHECK (octet_length(nonce) = 32),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX auth_challenges_expiry_idx ON auth_challenges (expires_at);
