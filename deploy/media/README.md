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

Open a second terminal in the repository root. The commands below need `curl`
and `python3`; they register two users, create a group as the first user, and
add the second user to it. They leave the two access tokens and the conversation
id in shell variables for the next step.

```bash
API=http://localhost:8080
json_field() {
  python3 -c 'import json, sys; v = json.load(sys.stdin)
for key in sys.argv[1].split("."): v = v[key]
print(v)' "$1"
}

ALICE=$(curl -fsS -X POST "$API/auth/register" \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice_demo","password":"password123","display_name":"Alice"}')
BOB=$(curl -fsS -X POST "$API/auth/register" \
  -H 'Content-Type: application/json' \
  -d '{"username":"bob_demo","password":"password123","display_name":"Bob"}')

ALICE_TOKEN=$(printf %s "$ALICE" | json_field access_token)
BOB_TOKEN=$(printf %s "$BOB" | json_field access_token)
BOB_ID=$(printf %s "$BOB" | json_field user.id)

GROUP=$(curl -fsS -X POST "$API/conversations/groups" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"Call test"}')
CONVERSATION_ID=$(printf %s "$GROUP" | json_field id)

curl -fsS -X POST "$API/conversations/$CONVERSATION_ID/members" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$BOB_ID\"}"

printf 'Conversation: %s\nAlice token: %s\nBob token: %s\n' \
  "$CONVERSATION_ID" "$ALICE_TOKEN" "$BOB_TOKEN"
```

If you rerun this exact block, choose new usernames or log in instead:
`POST /auth/login` takes `{"username":"…","password":"…"}` and returns
the same token shape.

## 5. Run the manual test page

Run a small local web server in a third terminal:

```bash
python3 -m http.server 5173 --directory deploy/media
```

Open `http://localhost:5173/rtc-test.html` in two browser tabs. In both tabs
leave API base as `http://localhost:8080`, paste the shared conversation id,
and paste Alice's token in the first tab and Bob's token in the second. Click
**Start / Join** in the first tab, allow camera and microphone access, then do
the same in the second tab.

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
