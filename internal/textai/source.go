package textai

import "context"

// Source identifies one survey answer without sending participant names to the model.
type Source struct {
	ResponseID string `json:"response_id"`
	Question   string `json:"question"`
	Period     int    `json:"period"`
	Role       string `json:"role"`
}

type sourceKey struct{}

func WithSource(ctx context.Context, source Source) context.Context {
	return context.WithValue(ctx, sourceKey{}, source)
}

// The model selects a question from a fixed set. Free-form advice cannot introduce
// an unverified motive or assert a causal link between different answers.
var followUps = map[string]string{
	"example": "Как это проявилось в конкретной задаче?",
	"change":  "Что изменилось с момента заполнения анкеты?",
	"help":    "Какую поддержку вы хотели бы получить сейчас?",
}
