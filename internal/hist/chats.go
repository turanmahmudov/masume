package hist

import (
	"database/sql"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
)

// The conversations of a profile, one row each, and the turns of one conversation. The
// client works on a whole conversation, so it writes a whole conversation, in the same way
// as the tabs.

// The limits of one profile. Above them, the oldest entries are removed.
const (
	maxKeptChatTurns     = 100
	maxKeptConversations = 50
)

// maxTitleChars is the part of the first question a row of the list keeps.
const maxTitleChars = 90

// The roles a turn can have.
const (
	ChatRoleUser      = "user"
	ChatRoleAssistant = "assistant"
)

// ChatTurn is one turn of a conversation in stored form.
type ChatTurn struct {
	Role    string
	Content string
	// Context is the content of the editor at that time. It was sent with the question
	// and is never displayed.
	Context string
}

// ChatConversation is one conversation of a profile.
type ChatConversation struct {
	ID int64
	// Title is the first question of the conversation.
	Title     string
	StartedAt time.Time
	UpdatedAt time.Time
	TurnCount int
}

// buildConversationTitle returns the title of a conversation, which is its first question.
func buildConversationTitle(turns []ChatTurn) string {
	asked := ""
	for _, turn := range turns {
		if turn.Role == ChatRoleUser {
			asked = turn.Content
			break
		}
	}
	written := strings.TrimSpace(core.CollapseWhitespace(asked))
	if written == "" {
		return "a conversation"
	}
	if len([]rune(written)) > maxTitleChars {
		return string([]rune(written)[:maxTitleChars]) + "…"
	}
	return written
}

// ListConversations returns the conversations of one profile, the most recent first.
func (store *Store) ListConversations(profileName string) ([]ChatConversation, error) {
	if store == nil {
		return nil, nil
	}
	rows, err := store.file.Query(
		`SELECT c.id, c.title, c.started_at, c.updated_at,
		        (SELECT COUNT(*) FROM chat_turn t WHERE t.conversation_id = c.id) AS turn_count
		   FROM chat_conversation c
		  WHERE c.profile_name = ?
		  ORDER BY c.updated_at DESC, c.id DESC`, profileName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	kept := []ChatConversation{}
	for rows.Next() {
		held := ChatConversation{}
		var startedAt, updatedAt int64
		if scanErr := rows.Scan(
			&held.ID, &held.Title, &startedAt, &updatedAt, &held.TurnCount,
		); scanErr != nil {
			return nil, scanErr
		}
		held.StartedAt = time.UnixMilli(startedAt)
		held.UpdatedAt = time.UnixMilli(updatedAt)
		kept = append(kept, held)
	}
	return kept, rows.Err()
}

// ListChatTurns returns the turns of one conversation, the oldest first. A conversation
// starts with a question, so if the limit removed the first question, its answer is removed
// too.
func (store *Store) ListChatTurns(conversationID int64) ([]ChatTurn, error) {
	if store == nil {
		return nil, nil
	}
	rows, err := store.file.Query(
		`SELECT role, content, context FROM chat_turn
		  WHERE conversation_id = ? ORDER BY position`, conversationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	kept := []ChatTurn{}
	for rows.Next() {
		held := ChatTurn{}
		var context sql.NullString
		if scanErr := rows.Scan(&held.Role, &held.Content, &context); scanErr != nil {
			return nil, scanErr
		}
		if held.Role != ChatRoleAssistant {
			held.Role = ChatRoleUser
		}
		held.Context = context.String
		kept = append(kept, held)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return dropTurnsBeforeAQuestion(kept), nil
}

// dropTurnsBeforeAQuestion returns the turns from the first question to the end.
func dropTurnsBeforeAQuestion(turns []ChatTurn) []ChatTurn {
	for at, turn := range turns {
		if turn.Role == ChatRoleUser {
			return turns[at:]
		}
	}
	return []ChatTurn{}
}

// SaveConversation writes the turns of a conversation and returns its id. An id of zero
// creates a new conversation.
func (store *Store) SaveConversation(
	profileName string, conversationID int64, turns []ChatTurn,
) (int64, error) {
	if store == nil {
		return 0, nil
	}
	kept := turns
	if len(kept) > maxKeptChatTurns {
		kept = kept[len(kept)-maxKeptChatTurns:]
	}

	transaction, err := store.file.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = transaction.Rollback() }()

	savedAt := time.Now().UnixMilli()
	id, err := resolveConversationID(transaction, profileName, conversationID, kept, savedAt)
	if err != nil {
		return 0, err
	}
	if _, err = transaction.Exec(
		`DELETE FROM chat_turn WHERE conversation_id = ?`, id); err != nil {
		return 0, err
	}
	for position, turn := range kept {
		var context any
		if turn.Context != "" {
			context = turn.Context
		}
		if _, err = transaction.Exec(
			`INSERT INTO chat_turn (conversation_id, position, role, content, context)
			 VALUES (?, ?, ?, ?, ?)`,
			id, position, turn.Role, turn.Content, context); err != nil {
			return 0, err
		}
	}
	if err = dropOldConversations(transaction, profileName); err != nil {
		return 0, err
	}
	return id, transaction.Commit()
}

// resolveConversationID returns the id of the conversation the turns belong to, and creates
// a conversation if there is none.
func resolveConversationID(
	transaction *sql.Tx, profileName string, conversationID int64,
	turns []ChatTurn, savedAt int64,
) (int64, error) {
	if conversationID != 0 {
		_, err := transaction.Exec(
			`UPDATE chat_conversation SET updated_at = ? WHERE id = ?`, savedAt, conversationID)
		return conversationID, err
	}
	answered, err := transaction.Exec(
		`INSERT INTO chat_conversation (profile_name, title, started_at, updated_at)
		 VALUES (?, ?, ?, ?)`, profileName, buildConversationTitle(turns), savedAt, savedAt)
	if err != nil {
		return 0, err
	}
	return answered.LastInsertId()
}

// dropOldConversations removes the conversations of a profile above the limit, with their
// turns.
func dropOldConversations(transaction *sql.Tx, profileName string) error {
	if _, err := transaction.Exec(
		`DELETE FROM chat_conversation
		  WHERE profile_name = ?
		    AND id NOT IN (
		      SELECT id FROM chat_conversation
		       WHERE profile_name = ?
		       ORDER BY updated_at DESC, id DESC
		       LIMIT ?
		    )`, profileName, profileName, maxKeptConversations); err != nil {
		return err
	}
	_, err := transaction.Exec(
		`DELETE FROM chat_turn
		  WHERE conversation_id NOT IN (SELECT id FROM chat_conversation)`)
	return err
}

// DeleteConversation removes one conversation and its turns.
func (store *Store) DeleteConversation(conversationID int64) error {
	if store == nil {
		return nil
	}
	transaction, err := store.file.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	if _, err = transaction.Exec(
		`DELETE FROM chat_turn WHERE conversation_id = ?`, conversationID); err != nil {
		return err
	}
	if _, err = transaction.Exec(
		`DELETE FROM chat_conversation WHERE id = ?`, conversationID); err != nil {
		return err
	}
	return transaction.Commit()
}
