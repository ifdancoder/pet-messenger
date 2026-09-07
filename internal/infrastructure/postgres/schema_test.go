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

	var pkName string
	if err := testDB.QueryRowContext(ctx, `
		SELECT constraint_name
		FROM information_schema.table_constraints
		WHERE table_schema = 'public'
		  AND table_name = 'conversation_members'
		  AND constraint_type = 'PRIMARY KEY'`).Scan(&pkName); err != nil {
		t.Fatalf("query pk name: %v", err)
	}

	rows, err := testDB.QueryContext(ctx, `
		SELECT column_name
		FROM information_schema.key_column_usage
		WHERE table_schema = 'public' AND constraint_name = $1
		ORDER BY ordinal_position`, pkName)
	if err != nil {
		t.Fatalf("query pk columns: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			t.Fatalf("scan pk column: %v", err)
		}
		got = append(got, col)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

	want := []string{"conversation_id", "user_id"}
	if len(got) != len(want) {
		t.Fatalf("PK columns = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("PK column[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSchema_ForeignKeyDeleteActions(t *testing.T) {
	ctx := context.Background()

	rows, err := testDB.QueryContext(ctx, `
		SELECT cl.relname, a.attname, c.confdeltype
		FROM pg_constraint c
		JOIN pg_class cl ON cl.oid = c.conrelid
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
		WHERE c.contype = 'f'
		  AND cl.relnamespace = 'public'::regnamespace`)
	if err != nil {
		t.Fatalf("query fk delete actions: %v", err)
	}
	defer rows.Close()

	got := map[string]string{}
	for rows.Next() {
		var table, col, confdeltype string
		if err := rows.Scan(&table, &col, &confdeltype); err != nil {
			t.Fatalf("scan fk row: %v", err)
		}
		got[table+"."+col] = confdeltype
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

	// confdeltype: 'c' = ON DELETE CASCADE, 'n' = ON DELETE SET NULL.
	want := map[string]string{
		"messages.conversation_id":                  "c",
		"conversation_members.conversation_id":      "c",
		"conversation_members.user_id":              "c",
		"messages.sender_id":                        "n",
		"messages.reply_to_id":                      "n",
		"conversation_members.last_read_message_id": "n",
	}
	for key, wantType := range want {
		if got[key] != wantType {
			t.Errorf("FK %s confdeltype = %q, want %q", key, got[key], wantType)
		}
	}
}
