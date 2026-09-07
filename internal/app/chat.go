package app

import (
	"regexp"
	"slices"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

// Connection chat state and stored conversations. The UI handles model requests and passes response events to this state.

// ChatStatus is the response state.
type ChatStatus string

// The three states the chat can be in.
const (
	ChatIdle      ChatStatus = "idle"
	ChatStreaming ChatStatus = "streaming"
	ChatFailed    ChatStatus = "failed"
)

// ChatMessage is one turn of the conversation.
type ChatMessage struct {
	Role    string
	Content string
	// Context is the editor snapshot sent with the question and omitted from display.
	Context string
}

// ChatUsage is the connection token usage for the current client session.
type ChatUsage struct {
	InputTokens  int
	OutputTokens int
	// CachedInputTokens is the cached portion of InputTokens.
	CachedInputTokens int
}

// Add sums what a run spent into the total of the connection.
func (usage ChatUsage) Add(spent ChatUsage) ChatUsage {
	return ChatUsage{
		InputTokens:       usage.InputTokens + spent.InputTokens,
		OutputTokens:      usage.OutputTokens + spent.OutputTokens,
		CachedInputTokens: usage.CachedInputTokens + spent.CachedInputTokens,
	}
}

// PendingRun is a statement the chat wants to run, waiting for the user to allow it.
type PendingRun struct {
	// Summary is the statement risk and environment.
	Summary string
	SQL     string
	// The measured write plan as text, or empty when unavailable.
	Plan []string
}

// ChatEventKind is the response event category.
type ChatEventKind string

// The things a run reports.
const (
	// ChatTextStarted opens a block of text that follows an earlier one.
	ChatTextStarted  ChatEventKind = "text-started"
	ChatTextArrived  ChatEventKind = "text"
	ChatStepStarted  ChatEventKind = "step-started"
	ChatStepFinished ChatEventKind = "step-finished"
	// ChatRunAsked asks the user whether a statement may run.
	ChatRunAsked ChatEventKind = "run-asked"
	// ChatTableRead marks a relation the chat described as read in the tree as well.
	ChatTableRead ChatEventKind = "table-read"
	// ChatUndoKept hands over the undo of a write that ran.
	ChatUndoKept ChatEventKind = "undo-kept"
	// ChatEnded is the final event with usage and error details.
	ChatEnded ChatEventKind = "ended"
)

// ChatEvent is one thing a run reported.
type ChatEvent struct {
	// Run is the response sequence for rejecting stale events.
	Run  int
	Kind ChatEventKind
	Text string
	Ask  PendingRun
	// Allowed carries the answer of the user back to the statement that waits for it.
	Allowed chan bool
	Usage   ChatUsage
	Problem string
	// The relation the chat described, and what the server said about it.
	Table  db.TableRef
	Detail db.TableDetail
	Undo   writeplan.Undo
}

// Chat is the chat of one connection.
type Chat struct {
	Messages []ChatMessage
	Status   ChatStatus
	Problem  string
	// Activity is the call that runs now, and Steps the ones that finished.
	Activity string
	Steps    []string
	Usage    ChatUsage
	// Pending is the statement awaiting confirmation, or nil.
	Pending *PendingRun
	// allowed is where the answer to the waiting statement goes.
	allowed chan bool

	// Conversations is the profile history, newest first. OpenID is zero until the conversation has a stored turn.
	Conversations []hist.ChatConversation
	OpenID        int64

	// Run is the current response sequence.
	Run int
	// stop ends the run that writes now.
	stop func()

	// TurnAt is the selected turn. HasTurn is false before the first turn selection.
	TurnAt  int
	HasTurn bool
	// Offset is the scroll position. Follow is true while the newest row remains visible.
	Offset int
	Follow bool
	// StartedAt is the response start time.
	StartedAt time.Time
	// True while the reply of this run becomes a notebook, and what it is to cover.
	BuildsNotebook  bool
	NotebookSubject string
	// Notice is the line under the field, in place of what the chat spent.
	Notice string
}

// NewChat builds the chat of one connection.
func NewChat() *Chat {
	return &Chat{Status: ChatIdle, Follow: true}
}

// IsStreaming is true while a reply is being written.
func (chat *Chat) IsStreaming() bool {
	return chat.Status == ChatStreaming
}

// StartTurn appends a question and an empty reply. The request includes editor context only when the context changes.
func (chat *Chat) StartTurn(prompt, context string) []ChatMessage {
	sent := ""
	for _, message := range chat.Messages {
		if message.Context != "" {
			sent = message.Context
		}
	}
	asked := ChatMessage{Role: hist.ChatRoleUser, Content: prompt}
	if context != "" && context != sent {
		asked.Context = context
	}

	// Cap request history before the empty reply.
	held := make([]ChatMessage, 0, len(chat.Messages)+2)
	held = append(append(held, chat.Messages...), asked)
	history := held[:len(held):len(held)]
	chat.Messages = append(held, ChatMessage{Role: hist.ChatRoleAssistant})
	return history
}

// AppendDelta writes what arrived into the reply.
func (chat *Chat) AppendDelta(delta string) {
	chat.writeReply(func(reply ChatMessage) (ChatMessage, bool) {
		reply.Content += delta
		return reply, true
	})
}

// StartTextBlock separates response text blocks when the previous text has no trailing whitespace.
func (chat *Chat) StartTextBlock() {
	chat.writeReply(func(reply ChatMessage) (ChatMessage, bool) {
		if reply.Content == "" || endsInBlank.MatchString(reply.Content) {
			return reply, false
		}
		reply.Content += "\n\n"
		return reply, true
	})
}

// endsInBlank matches a reply that already ends in a blank.
var endsInBlank = regexp.MustCompile(`\s$`)

// DropEmptyReply removes a trailing empty assistant message.
func (chat *Chat) DropEmptyReply() {
	if len(chat.Messages) == 0 {
		return
	}
	last := chat.Messages[len(chat.Messages)-1]
	if last.Role == hist.ChatRoleAssistant && last.Content == "" {
		chat.Messages = chat.Messages[:len(chat.Messages)-1]
	}
}

// writeReply updates the final assistant message when present.
func (chat *Chat) writeReply(rewrite func(reply ChatMessage) (ChatMessage, bool)) {
	if len(chat.Messages) == 0 {
		return
	}
	at := len(chat.Messages) - 1
	if chat.Messages[at].Role != hist.ChatRoleAssistant {
		return
	}
	written, changed := rewrite(chat.Messages[at])
	if changed {
		chat.Messages[at] = written
	}
}

// StartStep updates the current activity label.
func (chat *Chat) StartStep(label string) {
	chat.Activity = label
}

// FinishStep records the current activity as a completed step.
func (chat *Chat) FinishStep() {
	if chat.Activity != "" {
		chat.Steps = append(chat.Steps, chat.Activity)
	}
	chat.Activity = ""
}

// ClearSteps drops the steps, which belong to one reply and not to the conversation.
func (chat *Chat) ClearSteps() {
	chat.Steps = nil
	chat.Activity = ""
}

// Ask keeps the statement that waits for a yes.
func (chat *Chat) Ask(pending PendingRun, allowed chan bool) {
	held := pending
	chat.Pending, chat.allowed = &held, allowed
}

// AnswerPending sends the confirmation response and clears the pending request.
func (chat *Chat) AnswerPending(confirmed bool) {
	if chat.allowed == nil {
		return
	}
	chat.allowed <- confirmed
	chat.Pending, chat.allowed = nil, nil
}

// Fail reports why the reply stopped.
func (chat *Chat) Fail(problem string) {
	chat.DropEmptyReply()
	chat.Problem = problem
	chat.Status = ChatFailed
}

// Stopped ends the run that writes now, and keeps what it had written.
func (chat *Chat) Stopped() {
	// Refuse pending execution before cancelling the response.
	chat.AnswerPending(false)
	if chat.stop != nil {
		chat.stop()
		chat.stop = nil
	}
	chat.Run++
	chat.DropEmptyReply()
	chat.ClearSteps()
	chat.Status = ChatIdle
}

// Begin starts a response and returns its sequence and event channel.
func (chat *Chat) Begin(stop func()) (int, chan ChatEvent) {
	chat.Run++
	chat.stop = stop
	chat.StartedAt = time.Now()
	chat.Follow = true
	chat.Notice = ""
	chat.Problem = ""
	chat.ClearSteps()
	chat.Status = ChatStreaming
	// Each response has a separate event channel.
	return chat.Run, make(chan ChatEvent, ChatEventRoom)
}

// ChatEventRoom is the response event buffer capacity.
const ChatEventRoom = 256

// End closes the run, and does nothing for a run that is not the one writing.
func (chat *Chat) End(run int) {
	if run != chat.Run {
		return
	}
	chat.stop = nil
	chat.FinishStep()
}

// OpenConversation puts a conversation read from the file on screen.
func (chat *Chat) OpenConversation(id int64, turns []hist.ChatTurn) {
	chat.Messages = make([]ChatMessage, 0, len(turns))
	for _, turn := range turns {
		chat.Messages = append(chat.Messages, ChatMessage{
			Role: turn.Role, Content: turn.Content, Context: turn.Context,
		})
	}
	chat.OpenID = id
	chat.TurnAt, chat.HasTurn, chat.Offset = 0, false, 0
}

// WriteTurns returns the turns of the conversation as the file keeps them.
func (chat *Chat) WriteTurns() []hist.ChatTurn {
	turns := make([]hist.ChatTurn, 0, len(chat.Messages))
	for _, message := range chat.Messages {
		turns = append(turns, hist.ChatTurn{
			Role: message.Role, Content: message.Content, Context: message.Context,
		})
	}
	return turns
}

// FindLastReply returns what the model last wrote, and whether it wrote anything.
func (chat *Chat) FindLastReply() (string, bool) {
	for _, v := range slices.Backward(chat.Messages) {
		if v.Role == hist.ChatRoleAssistant {
			return v.Content, true
		}
	}
	return "", false
}

// DescribeUsage summarizes session token usage, or returns an empty string before usage is recorded.
func (chat *Chat) DescribeUsage() string {
	if chat.Usage.InputTokens == 0 && chat.Usage.OutputTokens == 0 {
		return ""
	}
	written := present.FormatCount(int64(chat.Usage.InputTokens)) + " in"
	if chat.Usage.CachedInputTokens > 0 {
		written += " (" + present.FormatCount(int64(chat.Usage.CachedInputTokens)) + " cached)"
	}
	return written + " / " +
		present.FormatCount(int64(chat.Usage.OutputTokens)) + " out this session"
}
