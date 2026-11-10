package domain

type Observation struct {
	Phrase   string `json:"phrase"`
	Accepted *bool  `json:"accepted"`
}

type Suggestion struct {
	Word     string `json:"word"`
	Verified bool   `json:"verified"`
}

type SessionState struct {
	CurrentWord string   `json:"current_word"`
	Key         string   `json:"key"`
	Used        []string `json:"used"`
	Words       []string `json:"words"`
}
