# Calls / media-service — local deploy & manual verification

Brings up `api` + `media` + coturn + redis + nats + postgres for a real
end-to-end call, for manual testing (there is no automated e2e test for the
full stack yet — see the implementation plan's Task 12 Step 5).

## 1. Generate a JWT signing key

```
go run ./cmd/genkey
```

Export the printed value as `JWT_ED25519_SEED_BASE64` in your shell (both
`api` and `media` need the same key so join-tickets verify).

## 2. Apply migrations

The compose stack does not run migrations automatically. With `postgres`
listening on `localhost:5432` (bring it up first: `docker compose -f
deploy/media/docker-compose.yml up postgres -d`), run from the repo root:

```
BLUEPRINT_DB_HOST=localhost BLUEPRINT_DB_PORT=5432 BLUEPRINT_DB_DATABASE=app \
BLUEPRINT_DB_USERNAME=app BLUEPRINT_DB_PASSWORD=app BLUEPRINT_DB_SCHEMA=public \
go run ./cmd/migrate -direction up
```

## 3. Bring up the rest of the stack

```
docker compose -f deploy/media/docker-compose.yml up --build
```

## 4. Create two users and a group chat

Use the existing auth/conversation REST endpoints (`POST /auth/register`,
`POST /auth/login`, `POST /conversations`) to register two users and put them
both in a `group_chat`. Keep both access tokens and the conversation id.

## 5. Run the manual test page

Open `deploy/media/rtc-test.html` in two browser tabs (a `file://` URL is
fine — it only talks to `http://localhost:8080` and `ws://localhost:8081`).
In each tab: paste the API base, that tab's access token, and the shared
conversation id, then click **Start / Join**.

Verify:
- Two-way audio + video appears in both tabs within a few seconds.
- Click **Share screen** in one tab; the other tab gets a second remote
  video.
- From a third admin tab (or curl), `DELETE /calls/:id/participants/:userId`
  for one participant and confirm that tab's `removed` log line fires and
  its media stops.

## 6. Force the TURN relay path

In the page's `RTCPeerConnection` config, temporarily set
`iceTransportPolicy: "relay"` (edit `rtc-test.html` or use the browser
devtools) and confirm the call still connects — check the `coturn`
container's logs for an allocation, which confirms `TURN_URL` / `TURN_SECRET`
on the `api` service match `turnserver.conf`'s `static-auth-secret`.

### Moderator mute and unmute

Use authenticated `POST /calls/{callID}/participants/{userID}/mute` and
`POST /calls/{callID}/participants/{userID}/unmute` requests. Both require the
existing call moderation permissions (call initiator or eligible conversation
admin), and neither permits self-targeting. The target receives a `muted` or
`unmuted` signaling frame with `by: "server"`.

A client offer, replacement connection, or leave/rejoin cannot clear a moderator
mute. The media room retains that decision for its lifetime. Video and screen
sharing continue while audio forwarding is muted. Unmute restores forwarding;
clients retain control of their local microphone capture settings.
