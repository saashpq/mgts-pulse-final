package synth

// Episode — заложенная в данные проблема: сторона (intern | mentor | pair), класс и месяц начала t0 (1-based).
// Нужна, чтобы считать полноту, точность и задержку системы против истины, а не против другого правила.
type Episode struct {
	Role  string `json:"role"`
	Class string `json:"class"`
	T0    int    `json:"t0"`
}

// Onset — истина синтетического датасета: месяц, с которого проблема заложена в ответы. P1 — контрольная пара.
var Onset = map[string][]Episode{
	"P1": {},
	"P2": {{Role: "intern", Class: "motivation", T0: 3}},
	"P3": {{Role: "intern", Class: "support", T0: 2}, {Role: "mentor", Class: "performance", T0: 2}},
	"P4": {{Role: "pair", Class: "divergence", T0: 2}},
	"P5": {{Role: "intern", Class: "dip", T0: 3}},
}
