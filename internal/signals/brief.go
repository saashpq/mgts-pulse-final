package signals

import (
	"fmt"
	"strings"
	"time"
)

// BriefQuestion — вопрос для разговора 1:1. With — с кем разговор: intern | mentor.
type BriefQuestion struct {
	With string `json:"with"`
	Text string `json:"text"`
}

// QuestionTemplates — каталог вопросов к встрече по типу сигнала. Вопросы открытые и бережные:
// про ситуацию и задачи, без оценок личности и без пересказа того, что написала другая сторона.
var QuestionTemplates = map[string][]BriefQuestion{
	"S1": {
		{"intern", "Как ты сейчас оцениваешь стажировку по сравнению с первыми месяцами? Что изменилось?"},
		{"intern", "Что за последний месяц далось труднее всего, а что получилось?"},
		{"mentor", "Замечаешь ли ты изменения в настроении или вовлечённости стажёра за последние месяцы?"},
	},
	"S2": {
		{"mentor", "Какие задачи стажёра за последний месяц тебя порадовали, а какие получились хуже ожиданий?"},
		{"mentor", "Какие ожидания к стажёру сейчас главные и знает ли он о них?"},
		{"intern", "Как ты понимаешь, что от тебя сейчас ждут в работе?"},
	},
	"S3": {
		{"intern", "Как бы ты описал, что у тебя получается хорошо, а над чем ещё стоит поработать?"},
		{"mentor", "Как ты обычно даёшь стажёру обратную связь и как он на неё реагирует?"},
		{"intern", "Как часто вы с наставником обсуждаете результаты и договорённости?"},
	},
	"S4": {
		{"intern", "Какую обратную связь ты получал по своим задачам в последнее время?"},
		{"mentor", "Какие критерии «сделано хорошо» для задач стажёра можно сформулировать вместе с ним?"},
		{"mentor", "Были ли разговоры со стажёром о том, что стоит улучшить? Как они прошли?"},
	},
	"S5": {
		{"mentor", "Как, на твой взгляд, у стажёра идут дела в последние месяцы?"},
		{"intern", "Что для тебя сейчас важнее всего в стажировке?"},
	},
	"S6": {
		{"intern", "Когда задача непонятна или нужна помощь, к кому ты обращаешься и как быстро получаешь ответ?"},
		{"intern", "Что могло бы сделать постановку задач понятнее?"},
		{"mentor", "Сколько времени в неделю сейчас получается выделять стажёру? Чем HR может помочь?"},
	},
	"S7": {
		{"intern", "Какие задачи за время стажировки были самыми интересными и почему?"},
		{"intern", "Чему ты хотел бы научиться за оставшиеся месяцы?"},
		{"mentor", "Есть ли у стажёра задачи на рост, которые можно дать в ближайший месяц?"},
	},
	"S8": {
		{"mentor", "Какие конкретные результаты ты ждёшь от стажёра в ближайший месяц?"},
		{"mentor", "Что, по-твоему, мешает стажёру достигать этих результатов и чем можно помочь?"},
		{"intern", "Какие задачи сейчас даются труднее всего и какая помощь была бы полезна?"},
	},
	"S9": {
		{"intern", "Ты несколько раз упоминал одну и ту же трудность. Что изменилось с тех пор, как она появилась?"},
		{"mentor", "Какие повторяющиеся трудности ты видишь в работе и что уже пробовали изменить?"},
	},
	"S10": {
		{"intern", "Как ты себя чувствуешь в команде и в работе с наставником? Что хотелось бы изменить?"},
		{"intern", "Как ты видишь своё продолжение после стажировки?"},
		{"mentor", "Как складывается ваше взаимодействие со стажёром? Что помогло бы его наладить?"},
	},
	"S11": {
		{"intern", "Удобно ли заполнять ежемесячную анкету? Что мешало в этом месяце?"},
		{"mentor", "Удобно ли заполнять анкету наставника и хватает ли на неё времени?"},
	},
	"S12": {
		{"intern", "Как дела в целом? Есть что-то, о чём не хочется писать в анкете, но стоит обсудить?"},
		{"mentor", "Есть ли что-то в работе со стажёром, что не попало в анкету?"},
	},
	"S13": {
		{"intern", "Насколько анкета помогает рассказать, как идут дела? Что в ней стоит изменить?"},
		{"mentor", "Насколько вопросы анкеты отражают работу стажёра? Чего в них не хватает?"},
	},
	"S14": {
		{"intern", "Что случилось при общении с наставником и чем HR может помочь?"},
		{"mentor", "Что произошло при взаимодействии со стажёром и какая поддержка нужна?"},
	},
	"S15": {
		{"intern", "Что в последней задаче хотелось бы уточнить или изменить?"},
		{"mentor", "Как можно вместе со стажёром пересобрать следующую задачу?"},
	},
	"S16": {
		{"intern", "Что повлияло на твою оценку в этом месяце и какую помощь стоит обсудить?"},
		{"mentor", "Какие наблюдения легли в основу оценки и что изменилось за этот месяц?"},
	},
}

// genericQuestions — добор до трёх вопросов, когда сигналов мало или нет.
var genericQuestions = []BriefQuestion{
	{"intern", "Что за последний месяц получилось лучше всего?"},
	{"intern", "Какие цели на следующий месяц ты бы поставил себе сам?"},
	{"mentor", "Какой следующий шаг в развитии стажёра ты видишь?"},
}

// BriefSignal — сигнал в брифе: без служебных полей, с цитатами-основаниями.
type BriefSignal struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Severity    Severity `json:"severity"`
	Explanation string   `json:"explanation"`
	Action      string   `json:"action"`
	DueDays     int      `json:"due_days"`
	SeenAt      string   `json:"seen_at,omitempty"`
	Quotes      []string `json:"quotes"`
	Side        string   `json:"side"` // чьи ответы лежат в основе: intern | mentor | both
}

// BriefAudience — тезисы для разговора с одной стороной: только сигналы, основанные на её ответах или на обеих
// анкетах. Сигнал по анкете наставника (S2, S8, S9/S12/S13 наставника) в разговор со стажёром не попадает — и наоборот:
// так HR не пересказывает одной стороне то, что написала другая.
type BriefAudience struct {
	With          string   `json:"with"`
	Signals       []string `json:"signals"` // ID сигналов брифа для этой стороны
	TalkingPoints []string `json:"talking_points"`
}

// Side — на чьих ответах основан сигнал: intern | mentor | both.
func Side(s Signal) string {
	switch {
	case strings.HasSuffix(s.ID, "-intern"):
		return "intern"
	case strings.HasSuffix(s.ID, "-mentor"):
		return "mentor"
	}
	switch s.Type {
	case "S1", "S6", "S7", "S15":
		return "intern"
	case "S2", "S8":
		return "mentor"
	case "S10", "S16": // Evidence from only one author.
		if len(s.Evidence) > 0 && (s.Evidence[0].Role == "intern" || s.Evidence[0].Role == "mentor") {
			return s.Evidence[0].Role
		}
	}
	return "both"
}

func forSide(side, with string) bool { return side == "both" || side == with }

// Brief — материалы HR к встрече 1:1 по паре.
type Brief struct {
	Pair          string          `json:"pair"`
	Intern        string          `json:"intern"`
	Mentor        string          `json:"mentor"`
	Status        string          `json:"status"`
	GeneratedAt   string          `json:"generated_at"`
	Signals       []BriefSignal   `json:"signals"`
	Questions     []BriefQuestion `json:"questions"`
	TalkingPoints []string        `json:"talking_points"`
	Audiences     struct {
		Intern BriefAudience `json:"intern"`
		Mentor BriefAudience `json:"mentor"`
	} `json:"audiences"`
	Disclaimer string `json:"disclaimer"`
}

// BriefDisclaimer — напоминание, что бриф — подсказка, а не оценка человека.
const BriefDisclaimer = "Бриф собран автоматически по анкетам и правилам. Это подсказка к разговору, а не оценка человека: " +
	"цитаты другой стороны не пересказывать, решения принимает HR."

// MakeBrief — бриф к встрече по отчёту пары. Берутся только активные сигналы без вердикта «ложный» или «решён».
func MakeBrief(pr PairReport, now time.Time) Brief {
	b := Brief{Pair: pr.PairID, Intern: pr.Intern, Mentor: pr.Mentor, Status: pr.Status,
		GeneratedAt: now.UTC().Format(time.RFC3339), Signals: []BriefSignal{}, Questions: []BriefQuestion{},
		TalkingPoints: []string{}, Disclaimer: BriefDisclaimer}
	var types []string
	for _, s := range pr.Signals {
		if s.Verdict != nil && (s.Verdict.Status == "false" || s.Verdict.Status == "resolved") {
			continue
		}
		bs := BriefSignal{ID: s.ID, Type: s.Type, Title: s.Title, Severity: s.Severity, Explanation: s.Explanation, Action: s.Action, DueDays: s.DueDays,
			SeenAt: s.SeenAt, Quotes: []string{}, Side: Side(s.Signal)}
		for _, e := range s.Evidence {
			if e.Quote != "" && !hasStr(bs.Quotes, e.Quote) {
				bs.Quotes = append(bs.Quotes, e.Quote)
			}
		}
		b.Signals = append(b.Signals, bs)
		if !hasStr(types, s.Type) {
			types = append(types, s.Type)
		}
	}
	b.Questions = pickQuestions(types, 5)
	if t := trendText(pr); t != "" {
		b.TalkingPoints = append(b.TalkingPoints, t)
	}
	for _, s := range b.Signals {
		b.TalkingPoints = append(b.TalkingPoints, fmt.Sprintf("%s — %s (в течение %d дн.)", s.Title, lowerFirst(s.Action), s.DueDays))
	}
	if len(pr.Resolved) > 0 {
		var titles []string
		for _, s := range pr.Resolved {
			if t := strings.ToLower(s.Title); !hasStr(titles, t) {
				titles = append(titles, t)
			}
		}
		b.TalkingPoints = append(b.TalkingPoints, "Прежние сигналы: "+strings.Join(titles, "; ")+". Уточните на встрече, что изменилось.")
	}
	if len(b.Signals) == 0 {
		b.TalkingPoints = append(b.TalkingPoints, "Активных сигналов нет — разговор о целях и развитии, без поиска проблем")
	}
	b.Audiences.Intern = audience(pr, b.Signals, "intern")
	b.Audiences.Mentor = audience(pr, b.Signals, "mentor")
	return b
}

// audience — тезисы к разговору с одной стороной: её собственная динамика, её сигналы и что наладилось по её сигналам.
func audience(pr PairReport, sigs []BriefSignal, with string) BriefAudience {
	a := BriefAudience{With: with, Signals: []string{}, TalkingPoints: []string{}}
	if t := ownTrend(pr, with); t != "" {
		a.TalkingPoints = append(a.TalkingPoints, t)
	}
	for _, s := range sigs {
		if forSide(s.Side, with) {
			a.Signals = append(a.Signals, s.ID)
			a.TalkingPoints = append(a.TalkingPoints, fmt.Sprintf("%s — %s (в течение %d дн.)", s.Title, lowerFirst(s.Action), s.DueDays))
		}
	}
	var fixed []string
	for _, s := range pr.Resolved {
		if t := strings.ToLower(s.Title); forSide(Side(s.Signal), with) && !hasStr(fixed, t) {
			fixed = append(fixed, t)
		}
	}
	if len(fixed) > 0 {
		a.TalkingPoints = append(a.TalkingPoints, "Прежние сигналы: "+strings.Join(fixed, "; ")+". Уточните на встрече, что изменилось.")
	}
	if len(a.Signals) == 0 {
		a.TalkingPoints = append(a.TalkingPoints, "Сигналов по этой стороне нет — разговор о целях и развитии, без поиска проблем")
	}
	return a
}

// ownTrend — только собственные оценки стороны: стажёру не называют оценку наставника, и наоборот.
func ownTrend(pr PairReport, with string) string {
	idx, label := pr.InternIndex, "Индекс стажёра"
	if with == "mentor" {
		idx, label = pr.MentorIndex, "Оценка наставника"
	}
	for i := len(idx) - 1; i >= 0; i-- {
		if idx[i] != nil {
			return fmt.Sprintf("%s в M%d — %s.", label, i+1, Num(*idx[i]))
		}
	}
	return ""
}

// pickQuestions — по кругу берёт вопросы из шаблонов сигналов (в порядке критичности), без повторов, до limit штук,
// не больше limit-2 на одну сторону; если вышло меньше трёх — добирает общими вопросами.
// В итоге есть вопросы и стажёру, и наставнику.
func pickQuestions(types []string, limit int) []BriefQuestion {
	var cand []BriefQuestion
	for round := 0; ; round++ {
		more := false
		for _, typ := range types {
			if qs := QuestionTemplates[typ]; round < len(qs) {
				more = true
				cand = append(cand, qs[round])
			}
		}
		if !more {
			break
		}
	}
	out := []BriefQuestion{}
	seen := map[string]bool{}
	per := map[string]int{}
	take := func(qs []BriefQuestion, cap int) {
		for _, q := range qs {
			if len(out) < limit && !seen[q.Text] && per[q.With] < cap {
				seen[q.Text] = true
				per[q.With]++
				out = append(out, q)
			}
		}
	}
	take(cand, max(limit-2, 1))
	for _, q := range genericQuestions { // сторона без вопросов получает общий
		if per[q.With] == 0 {
			if len(out) == limit {
				out = out[:limit-1]
			}
			take([]BriefQuestion{q}, limit)
		}
	}
	for _, q := range genericQuestions {
		if len(out) >= 3 {
			break
		}
		take([]BriefQuestion{q}, limit)
	}
	return out
}

func hasWith(qs []BriefQuestion, with string) bool {
	for _, q := range qs {
		if q.With == with {
			return true
		}
	}
	return false
}

func hasStr(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
