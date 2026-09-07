// internal/infrastructure/postgres/schema_test.go
package postgres

import (
	"context"
	"testing"
)

func TestSchema_TablesExist(t *testing.T) {
	ctx := context.Background()
	for _, table := range []string{"conversations", "messages", "conversation_members"} {
		var reg *string
		if err := testDB.QueryRowContext(ctx, `SELECT to_regclass($1)`, "public."+table).Scan(&reg); err != nil {
			t.Fatalf("to_regclass(%q): %v", table, err)
		}
		if reg == nil {
			t.Errorf("table %q does not exist", table)
		}
	}
}

func TestSchema_EnumTypes(t *testing.T) {
	ctx := context.Background()
	want := map[string][]string{
		"conversation_kind": {"direct", "group_chat", "channel_group", "channel_broadcast"},
		"conversation_role": {"owner", "admin", "member", "subscriber"},
		"message_type":      {"text", "attachment", "system", "call_event"},
	}
	for typ, labels := range want {
		rows, err := testDB.QueryContext(ctx, `
			SELECT e.enumlabel
			FROM pg_enum e JOIN pg_type t ON t.oid = e.enumtypid
			WHERE t.typname = $1
			ORDER BY e.enumsortorder`, typ)
		if err != nil {
			t.Fatalf("query enum %q: %v", typ, err)
		}
		var got []string
		for rows.Next() {
			var l string
			if err := rows.Scan(&l); err != nil {
				rows.Close()
				t.Fatalf("scan enum label: %v", err)
			}
			got = append(got, l)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatalf("rows err: %v", err)
		}
		if len(got) != len(labels) {
			t.Fatalf("enum %q labels = %v, want %v", typ, got, labels)
		}
		for i := range labels {
			if got[i] != labels[i] {
				t.Errorf("enum %q label[%d] = %q, want %q", typ, i, got[i], labels[i])
			}
		}
	}
}

func TestSchema_MessagesHistoryIndex(t *testing.T) {
	var reg *string
	if err := testDB.QueryRow(`SELECT to_regclass($1)`, "public.idx_messages_conversation_created_id").Scan(&reg); err != nil {
		t.Fatalf("to_regclass(index): %v", err)
	}
	if reg == nil {
		t.Error("expected idx_messages_conversation_created_id to exist")
	}
}

func TestSchema_ConversationMembersCompositePK(t *testing.T) {
	ctx := context.Background()
	var cols int
	err := testDB.QueryRowContext(ctx, `
		SELECT count(*)
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		WHERE tc.table_name = 'conversation_members' AND tc.constraint_type = 'PRIMARY KEY'`).Scan(&cols)
	if err != nil {
		t.Fatalf("query pk columns: %v", err)
	}
	if cols != 2 {
		t.Errorf("conversation_members PK spans %d columns, want 2 (conversation_id, user_id)", cols)
	}
}
