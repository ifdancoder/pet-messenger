-- End-to-end encryption identities belong to devices, not accounts.  The
-- server stores only public material and opaque ciphertext; private keys never
-- leave the browser/device that generated them.
CREATE TYPE e2ee_device_status AS ENUM ('pending', 'trusted', 'revoked');

CREATE TABLE e2ee_devices (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    public_key      BYTEA NOT NULL,
    fingerprint     TEXT NOT NULL,
    label           TEXT NOT NULL DEFAULT '',
    status          e2ee_device_status NOT NULL DEFAULT 'pending',
    approved_by     UUID REFERENCES e2ee_devices(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at     TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ,
    UNIQUE (user_id, fingerprint)
);
CREATE INDEX idx_e2ee_devices_user_status ON e2ee_devices (user_id, status);

-- A recovery backup is encrypted locally with a key derived from the user's
-- recovery phrase. The server cannot decrypt it or enroll a device with it.
CREATE TABLE e2ee_backups (
    user_id         UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    ciphertext      BYTEA NOT NULL,
    salt            BYTEA NOT NULL,
    nonce           BYTEA NOT NULL,
    algorithm       TEXT NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Per-recipient opaque key packages make an encrypted message or call key
-- available to every trusted device without giving the server the plaintext.
CREATE TABLE e2ee_key_envelopes (
    id              UUID PRIMARY KEY,
    owner_device_id UUID NOT NULL REFERENCES e2ee_devices(id) ON DELETE CASCADE,
    recipient_device_id UUID NOT NULL REFERENCES e2ee_devices(id) ON DELETE CASCADE,
    scope           TEXT NOT NULL,
    ciphertext      BYTEA NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_device_id, recipient_device_id, scope)
);
CREATE INDEX idx_e2ee_key_envelopes_recipient ON e2ee_key_envelopes (recipient_device_id, scope);
