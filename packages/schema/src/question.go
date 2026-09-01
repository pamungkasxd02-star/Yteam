package schema

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type QuestionInfo struct {
	Question string           `json:"question"`
	Header   string           `json:"header"`
	Options  []QuestionOption `json:"options"`
	Multiple bool             `json:"multiple,omitempty"`
	Custom   *bool            `json:"custom,omitempty"`
}

type QuestionToolRef struct {
	MessageID string `json:"messageID"`
	CallID    string `json:"callID"`
}

type QuestionRequest struct {
	ID        string           `json:"id"`
	SessionID string           `json:"sessionID"`
	Questions []QuestionInfo   `json:"questions"`
	Tool      *QuestionToolRef `json:"tool,omitempty"`
}

type QuestionAnswer []string

type QuestionReply struct {
	Answers []QuestionAnswer `json:"answers"`
}
