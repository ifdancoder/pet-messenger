// rebuildconversationinbox fully reconstructs the conversation_inbox read
// model from the write-side tables (conversations, conversation_members).
// Run it after restoring a backup, after a schema change to the
// projection, or any time drift between the projection and the write side
// is suspected — this is the required rebuild path the design spec's
// "Rebuildability" section calls for: a projection that cannot be rebuilt
// from the system of record is not a trustworthy read model.
package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"

	"pet-messenger/internal/platform/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	db, err := sql.Open("pgx", cfg.DB.DSN())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	n, err := rebuild(db)
	if err != nil {
		log.Fatalf("rebuild: %v", err)
	}
	log.Printf("conversation_inbox rebuilt: %d rows", n)
}

// rebuild truncates and repopulates conversation_inbox from
// conversations/conversation_members in one transaction. Every rebuilt
// row's last_event_seq is reset to 0, so any subsequent event the
// projector applies (seq >= 1) is always treated as newer than this
// baseline — deliberately simple: a rebuild is an infrequent, operator-
// triggered repair, not a hot path, so it does not try to preserve
// in-flight event ordering across the rebuild boundary.
func rebuild(db *sql.DB) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`TRUNCATE conversation_inbox`); err != nil {
		return 0, fmt.Errorf("truncate conversation_inbox: %w", err)
	}

	const rebuildQ = `
		INSERT INTO conversation_inbox (user_id, conversation_id, kind, title, avatar_url, public, created_at, archived_at, direct_peer_user_id, last_message_at, last_event_seq)
		SELECT
			cm.user_id,
			c.id,
			c.kind,
			c.title,
			c.avatar_url,
			c.public,
			c.created_at,
			c.archived_at,
			CASE WHEN c.kind = 'direct' THEN (
				SELECT other.user_id FROM conversation_members other
				WHERE other.conversation_id = c.id AND other.user_id <> cm.user_id
				LIMIT 1
			) END,
			c.last_message_at,
			0
		FROM conversation_members cm
		JOIN conversations c ON c.id = cm.conversation_id`
	res, err := tx.Exec(rebuildQ)
	if err != nil {
		return 0, fmt.Errorf("rebuild conversation_inbox: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return n, nil
}
