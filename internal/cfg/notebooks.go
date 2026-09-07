package cfg

// NotebookSettings is the app configuration under `[notebooks]`.
type NotebookSettings struct {
	// The directories the notebook list reads, beside the project and the state directory.
	Paths []string
}

// ParseNotebookSettings reads `[notebooks]`.
func ParseNotebookSettings(document Table) NotebookSettings {
	settings := NotebookSettings{}
	section, held := FindSection(document, "notebooks")
	if !held {
		return settings
	}
	if paths, found := FindStringList(section, "paths"); found {
		settings.Paths = paths
	}
	return settings
}
