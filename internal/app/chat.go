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

// The chat of one connection: the turns said so far, what the reply is doing now, and the
// conversations of the profile behind it. Nothing here reaches a model; the screen starts a run
// and feeds what arrives back in.

// ChatStatus says how far the reply got.
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
	// Context is what the editor held at the time. It is sent with the question, and is not
	// drawn.
	Context string
}

// ChatUsage is what the chat spent on this connection. It is counted for the run of the client
// and not for the conversation, because a conversation read from the file was paid for already.
type ChatUsage struct {
	InputTokens  int
	OutputTokens int
	// CachedInputTokens is the part of the input the provider read from its cache, at a tenth
	// of the price. It is a part of the input count, not a sum beside it.
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
	// Summary is what it would do, in the words the screens use, such as "removes data on
	// prod".
	Summary string
	SQL     string
	// What the write does, one line each, and nothing where none was measured.
	Plan []string
}

// ChatEventKind names what happened while a reply was written.
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
	// ChatEnded says the run is over, with what it spent and what went wrong.
	ChatEnded ChatEventKind = "ended"
)

// ChatEvent is one thing a run reported.
type ChatEvent struct {
	// Run numbers the run it belongs to, so a run that was stopped writes nothing more.
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
	// Pending is the statement that waits for a yes, and nothing between runs.
	Pending *PendingRun
	// allowed is where the answer to the waiting statement goes.
	allowed chan bool

	// Conversations holds the conversations of this profile, the most recent first, and OpenID
	// the one on screen. An empty chat is no conversation, so the id is zero until the first
	// turn is written.
	Conversations []hist.ChatConversation
	OpenID        int64

	// Run numbers the run that writes now, so a run that was stopped is told apart from
	// the one after it.
	Run int
	// stop ends the run that writes now.
	stop func()

	// TurnAt is the turn a jump landed on, and HasTurn is false before the first jump.
	TurnAt  int
	HasTurn bool
	// Offset is how far the conversation is scrolled, and Follow is true while it keeps to the
	// newest row.
	Offset int
	Follow bool
	// StartedAt is when the reply being written began, so the wheel can count the seconds.
	StartedAt time.Time
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

// StartTurn adds the question and an empty reply, and returns the turns to send. The editor
// rides with a question only where it was not sent already, because the conversation carries it
// from the first question on.
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

	// The turns are copied once. The request carries them as they stand, and the panel
	// draws the same turns with the reply that is being written under them, so the history
	// is capped and the empty reply lands past its end.
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

// StartTextBlock parts a block of text from the one before it. A tool step has text on both
// sides of the call, with no mark between them.
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

// DropEmptyReply removes the reply that was never written, so a failure leaves no empty turn.
func (chat *Chat) DropEmptyReply() {
	if len(chat.Messages) == 0 {
		return
	}
	last := chat.Messages[len(chat.Messages)-1]
	if last.Role == hist.ChatRoleAssistant && last.Content == "" {
		chat.Messages = chat.Messages[:len(chat.Messages)-1]
	}
}

// writeReply rewrites the reply being written, and does nothing where the last turn is not one.
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

// StartStep names the call that runs now.
func (chat *Chat) StartStep(label string) {
	chat.Activity = label
}

// FinishStep keeps the call that ran as a step of the reply, so the line does not fall back to
// what it said before it.
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

// AnswerPending returns the waiting statement. A no does not run it.
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
	// A waiting statement does not run, so the tool returns instead of waiting for ever.
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

// Begin opens a run: it numbers it, keeps the way to stop it, and returns the channel the run
// reports through.
func (chat *Chat) Begin(stop func()) (int, chan ChatEvent) {
	chat.Run++
	chat.stop = stop
	chat.StartedAt = time.Now()
	chat.Follow = true
	chat.Notice = ""
	chat.Problem = ""
	chat.ClearSteps()
	chat.Status = ChatStreaming
	// The channel belongs to the run, not to the chat: a run that was stopped goes on
	// reporting until the provider notices, and it must not reach the run after it.
	return chat.Run, make(chan ChatEvent, ChatEventRoom)
}

// ChatEventRoom is how many events a run may report before it waits for the screen to read
// them. A reply arrives in small pieces, and the screen takes every piece that is waiting
// before it draws.
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

// DescribeUsage writes what the chat spent, and nothing while it has spent nothing. The log
// holds the exact figure of every message.
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
