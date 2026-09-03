package hist

import (
	"path/filepath"
	"testing"
)

// openTestStore returns a separate history file for one test.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatalf("the history file did not open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestSaveAndReadAConversation(t *testing.T) {
	store := openTestStore(t)
	turns := []ChatTurn{
		{Role: ChatRoleUser, Content: "how many orders are unpaid",
			Context: "select * from orders"},
		{Role: ChatRoleAssistant, Content: "Two of them."},
	}

	id, err := store.SaveConversation("shop", 0, turns)
	if err != nil {
		t.Fatalf("the conversation was not written: %v", err)
	}
	if id == 0 {
		t.Fatal("the conversation was given no id")
	}

	read, err := store.ListChatTurns(id)
	if err != nil {
		t.Fatalf("the turns were not read: %v", err)
	}
	if len(read) != 2 || read[0].Content != turns[0].Content ||
		read[0].Context != turns[0].Context || read[1].Role != ChatRoleAssistant {
		t.Errorf("the turns read %+v", read)
	}

	conversations, err := store.ListConversations("shop")
	if err != nil {
		t.Fatalf("the conversations were not read: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("the profile holds %d conversations", len(conversations))
	}
	held := conversations[0]
	if held.Title != "how many orders are unpaid" || held.TurnCount != 2 {
		t.Errorf("the conversation reads %+v", held)
	}
	if conversations, _ := store.ListConversations("another"); len(conversations) != 0 {
		t.Error("the conversation of one profile was read for another")
	}
}

func TestSaveAConversationAgainReplacesItsTurns(t *testing.T) {
	store := openTestStore(t)
	id, _ := store.SaveConversation("shop", 0, []ChatTurn{
		{Role: ChatRoleUser, Content: "first"},
	})
	again, err := store.SaveConversation("shop", id, []ChatTurn{
		{Role: ChatRoleUser, Content: "first"},
		{Role: ChatRoleAssistant, Content: "an answer"},
		{Role: ChatRoleUser, Content: "second"},
	})
	if err != nil || again != id {
		t.Fatalf("the conversation was written as %d, wanted %d: %v", again, id, err)
	}

	read, _ := store.ListChatTurns(id)
	if len(read) != 3 {
		t.Errorf("the conversation holds %d turns, wanted three", len(read))
	}
	conversations, _ := store.ListConversations("shop")
	if len(conversations) != 1 || conversations[0].TurnCount != 3 {
		t.Errorf("the list reads %+v", conversations)
	}
}

func TestAConversationKeepsTheNewestTurns(t *testing.T) {
	store := openTestStore(t)
	turns := []ChatTurn{}
	for at := range maxKeptChatTurns + 4 {
		role := ChatRoleUser
		if at%2 == 1 {
			role = ChatRoleAssistant
		}
		turns = append(turns, ChatTurn{Role: role, Content: string(rune('a' + at%26))})
	}
	id, err := store.SaveConversation("shop", 0, turns)
	if err != nil {
		t.Fatalf("the conversation was not written: %v", err)
	}

	read, _ := store.ListChatTurns(id)
	// The oldest turns are removed, and an answer without its question is removed too.
	if len(read) > maxKeptChatTurns {
		t.Errorf("the conversation holds %d turns, wanted at most %d",
			len(read), maxKeptChatTurns)
	}
	if len(read) == 0 || read[0].Role != ChatRoleUser {
		t.Error("the conversation does not open with a question")
	}
}

func TestTheOldestConversationsAreDropped(t *testing.T) {
	store := openTestStore(t)
	for at := range maxKeptConversations + 3 {
		if _, err := store.SaveConversation("shop", 0, []ChatTurn{
			{Role: ChatRoleUser, Content: "question " + string(rune('a'+at%26))},
		}); err != nil {
			t.Fatalf("conversation %d was not written: %v", at, err)
		}
	}
	conversations, _ := store.ListConversations("shop")
	if len(conversations) != maxKeptConversations {
		t.Errorf("the profile holds %d conversations, wanted %d",
			len(conversations), maxKeptConversations)
	}
}

func TestDeleteConversation(t *testing.T) {
	store := openTestStore(t)
	id, _ := store.SaveConversation("shop", 0, []ChatTurn{
		{Role: ChatRoleUser, Content: "one"},
	})
	if err := store.DeleteConversation(id); err != nil {
		t.Fatalf("the conversation was not removed: %v", err)
	}
	if conversations, _ := store.ListConversations("shop"); len(conversations) != 0 {
		t.Errorf("the profile still holds %d conversations", len(conversations))
	}
	if turns, _ := store.ListChatTurns(id); len(turns) != 0 {
		t.Errorf("the turns of a removed conversation still read %+v", turns)
	}
}

func TestBuildConversationTitle(t *testing.T) {
	if title := buildConversationTitle(nil); title != "a conversation" {
		t.Errorf("an empty conversation is titled %q", title)
	}
	if title := buildConversationTitle([]ChatTurn{
		{Role: ChatRoleAssistant, Content: "an answer"},
		{Role: ChatRoleUser, Content: "  how   many\norders  "},
	}); title != "how many orders" {
		t.Errorf("the title reads %q", title)
	}
	long := make([]rune, maxTitleChars+10)
	for at := range long {
		long[at] = 'q'
	}
	title := buildConversationTitle([]ChatTurn{{Role: ChatRoleUser, Content: string(long)}})
	if len([]rune(title)) != maxTitleChars+1 || title[len(title)-3:] != "…" {
		t.Errorf("a long title reads %d characters", len([]rune(title)))
	}
}
