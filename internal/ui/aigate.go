package ui

// With `[ai] enabled = false` in the config file, masume carries no AI feature at all. The
// set below is the whole surface: no chord reaches any of these actions, no hint or help
// line names them, and the palette does not offer them. The gate is one set, so a new AI
// action has one place to be listed.

// aiActions is every action the AI features own.
var aiActions = map[ActionID]bool{
	ActionShowAiChat:  true,
	ActionSendToAi:    true,
	ActionAiFixError:  true,
	ActionAiCheckPlan: true,
	ActionInsertAiSQL: true,
	ActionStopAiReply: true,
	ActionNewAiChat:   true,
	ActionShowAiChats: true,
}

// IsAiAction reports whether this action belongs to the AI features.
func IsAiAction(id ActionID) bool { return aiActions[id] }

// aiPaletteRows are the palette rows that open an AI feature. They carry an id of their own
// rather than an action, so they are named here as well.
var aiPaletteRows = map[string]bool{
	"show-ai-chat":      true,
	"ai-explain-query":  true,
	"ai-optimize-query": true,
	"ai-fix-error":      true,
}

// aiHelpSection is the title of the help section the AI features own.
const aiHelpSection = "ai chat"

// offersAi reports whether the AI features are on.
func (model *Model) offersAi() bool { return model.ai.Enabled }

// listHelpSections returns the help groups to draw. The AI group is left out where the
// features are off, so the help never names a feature the client does not carry.
func (model *Model) listHelpSections() []HelpSection {
	if model.offersAi() {
		return HelpSections
	}
	kept := make([]HelpSection, 0, len(HelpSections))
	for _, section := range HelpSections {
		if section.Title == aiHelpSection {
			continue
		}
		kept = append(kept, section)
	}
	return kept
}
