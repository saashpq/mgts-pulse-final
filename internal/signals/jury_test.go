package signals

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func TestNum(t *testing.T) {
	for x, want := range map[float64]string{3.125: "3.13", 3.124: "3.12", 2.5: "2.50", -0.001: "0.00", -0.255: "-0.26", 4: "4.00"} {
		if got := Num(x); got != want {
			t.Errorf("Num(%v) = %s, want %s", x, got, want)
		}
	}
	if NumSigned(0.25) != "+0.25" || NumSigned(-0.1) != "-0.10" || NumSigned(0) != "+0.00" {
		t.Fatal("NumSigned")
	}
	// %.2f округляет 3.125 к чётному — именно это расхождение («3.13» в брифе, «3.12» в сигнале) и устранено
	if fmt.Sprintf("%.2f", 3.125) != "3.12" {
		t.Skip("platform rounding differs")
	}
}

// Одно и то же число в объяснении сигнала и в брифе пишется одинаково.
func TestSameNumberInSignalAndBrief(t *testing.T) {
	// индекс стажёра в M4 = (3+3+3.5+3)/4 = 3.125; R4: S1 требует базы и 4 точек для наклона Тейла–Сена
	in := onePair([][]float64{{5, 5, 5, 5}, {5, 5, 5, 5}, {4, 4, 4, 4}, {3, 3, 3.5, 3}}, []string{"", "", "", ""})
	rep := Analyze(in, nil)
	p := pairOf(rep, "PX")
	var s1 *SignalView
	for i := range p.Signals {
		if p.Signals[i].Type == "S1" {
			s1 = &p.Signals[i]
		}
	}
	if s1 == nil || !strings.Contains(s1.Explanation, "в M4 — 3.13") {
		t.Fatalf("%+v", s1)
	}
	b := MakeBrief(p, time.Now())
	if !strings.Contains(strings.Join(b.TalkingPoints, " "), "M4 — 3.13") || !strings.Contains(b.Audiences.Intern.TalkingPoints[0], "3.13") {
		t.Fatalf("%v", b.TalkingPoints)
	}
}

func TestSide(t *testing.T) {
	cases := []struct {
		s    Signal
		want string
	}{
		{Signal{ID: "P1-S1", Type: "S1"}, "intern"},
		{Signal{ID: "P1-S7", Type: "S7"}, "intern"},
		{Signal{ID: "P1-S2", Type: "S2"}, "mentor"},
		{Signal{ID: "P1-S8", Type: "S8"}, "mentor"},
		{Signal{ID: "P1-S13-mentor", Type: "S13"}, "mentor"},
		{Signal{ID: "P1-S9-intern", Type: "S9"}, "intern"},
		{Signal{ID: "P1-S12-mentor", Type: "S12"}, "mentor"},
		{Signal{ID: "P1-S11-intern", Type: "S11"}, "intern"},
		{Signal{ID: "P1-S10", Type: "S10", Evidence: []Evidence{{Role: "mentor"}}}, "mentor"},
		{Signal{ID: "P1-S10", Type: "S10"}, "both"},
		{Signal{ID: "P1-S3", Type: "S3"}, "both"},
		{Signal{ID: "P1-S4", Type: "S4"}, "both"},
	}
	for _, c := range cases {
		if got := Side(c.s); got != c.want {
			t.Errorf("%s: %s want %s", c.s.ID, got, c.want)
		}
	}
}

// Бриф по аудитории: сигналы только по анкете наставника не попадают в разговор со стажёром, и наоборот;
// «что наладилось» — только по сигналам этой стороны.
func TestBriefAudiences(t *testing.T) {
	pr := PairReport{PairID: "PX", Intern: "I", Mentor: "M",
		InternIndex: []*float64{ptr(4), ptr(3.125)}, MentorIndex: []*float64{ptr(2), nil},
		Signals: []SignalView{
			{Signal: Signal{ID: "PX-S8", Type: "S8", Title: "Наставник недоволен", Action: "Обсудить", DueDays: 5}},
			{Signal: Signal{ID: "PX-S13-mentor", Type: "S13", Title: "Формальная анкета", Action: "Уточнить", DueDays: 7}},
			{Signal: Signal{ID: "PX-S7", Type: "S7", Title: "Потеря мотивации", Action: "Пересмотреть", DueDays: 5}},
			{Signal: Signal{ID: "PX-S3", Type: "S3", Title: "Расхождение", Action: "Встреча", DueDays: 5}},
		},
		Resolved: []SignalView{
			{Signal: Signal{ID: "PX-S2", Type: "S2", Title: "Снижение оценки наставником"}},
			{Signal: Signal{ID: "PX-S1", Type: "S1", Title: "Снижение удовлетворённости"}},
		},
	}
	b := MakeBrief(pr, time.Now())
	in, mt := b.Audiences.Intern, b.Audiences.Mentor
	if strings.Join(in.Signals, ",") != "PX-S7,PX-S3" || strings.Join(mt.Signals, ",") != "PX-S8,PX-S13-mentor,PX-S3" {
		t.Fatalf("intern %v mentor %v", in.Signals, mt.Signals)
	}
	it, mtx := strings.Join(in.TalkingPoints, "\n"), strings.Join(mt.TalkingPoints, "\n")
	for _, bad := range []string{"Наставник недоволен", "Формальная анкета", "снижение оценки наставником", "Оценка наставника"} {
		if strings.Contains(it, bad) {
			t.Errorf("intern talk must not mention %q:\n%s", bad, it)
		}
	}
	for _, bad := range []string{"Потеря мотивации", "снижение удовлетворённости", "Индекс стажёра"} {
		if strings.Contains(mtx, bad) {
			t.Errorf("mentor talk must not mention %q:\n%s", bad, mtx)
		}
	}
	if !strings.Contains(it, "Прежние сигналы: снижение удовлетворённости") || !strings.Contains(mtx, "Прежние сигналы: снижение оценки наставником") {
		t.Fatalf("fixed:\n%s\n---\n%s", it, mtx)
	}
	if !strings.Contains(mtx, "Оценка наставника в M1 — 2.00") || !strings.Contains(it, "Индекс стажёра в M2 — 3.13") {
		t.Fatalf("own trend:\n%s\n---\n%s", it, mtx)
	}
	for _, s := range b.Signals {
		if s.Side == "" || s.ID == "" {
			t.Fatalf("%+v", s)
		}
	}
	// у стороны без своих сигналов — разговор о развитии
	empty := MakeBrief(PairReport{PairID: "PY", Signals: []SignalView{{Signal: Signal{ID: "PY-S8", Type: "S8", Title: "x", Action: "y"}}}}, time.Now())
	if len(empty.Audiences.Intern.Signals) != 0 || !strings.Contains(empty.Audiences.Intern.TalkingPoints[0], "Сигналов по этой стороне нет") {
		t.Fatalf("%+v", empty.Audiences.Intern)
	}
}

func ptr(x float64) *float64 { return &x }

// В каждой паре один главный сигнал: открытые выше закрытых вердиктом, затем критичность, затем приоритет типа.
func TestGroupPrimary(t *testing.T) {
	sv := func(id, typ string, sev Severity, verdict string) SignalView {
		s := SignalView{Signal: Signal{ID: id, Type: typ, Severity: sev}}
		if verdict != "" {
			s.Verdict = &Verdict{Status: verdict}
		}
		return s
	}
	cases := []struct {
		name    string
		list    []SignalView
		primary string
	}{
		{"severity first", []SignalView{sv("a", "S11", Low, ""), sv("b", "S6", High, ""), sv("c", "S10", Medium, "")}, "b"},
		{"type priority on tie", []SignalView{sv("a", "S8", High, ""), sv("b", "S1", High, ""), sv("c", "S3", High, ""), sv("d", "S10", High, "")}, "d"},
		{"S7 over S1", []SignalView{sv("a", "S1", High, ""), sv("b", "S7", High, "")}, "b"},
		{"false verdict loses", []SignalView{sv("a", "S10", High, "false"), sv("b", "S11", Low, "")}, "b"},
		{"all closed — still one primary", []SignalView{sv("a", "S2", Medium, "resolved"), sv("b", "S5", Medium, "false")}, "a"},
	}
	for _, c := range cases {
		group(c.list)
		if !c.list[0].Primary || c.list[0].ID != c.primary || len(c.list[0].Related) != len(c.list)-1 {
			t.Errorf("%s: %+v", c.name, c.list[0])
		}
		for _, s := range c.list[1:] {
			if s.Primary || len(s.Related) != 0 || s.Related == nil {
				t.Errorf("%s: %+v", c.name, s)
			}
		}
	}
	group(nil)
	// на синтетике: в каждой паре с активными сигналами ровно один главный
	rep := Analyze(synthInput(), nil)
	for _, p := range rep.Pairs {
		n := 0
		for _, s := range p.Signals {
			if s.Primary {
				n++
			}
		}
		if (len(p.Signals) > 0) != (n == 1) || n > 1 {
			t.Fatalf("%s: primaries %d", p.PairID, n)
		}
	}
	if top := pairOf(rep, "P2").Signals[0]; top.Type != "S10" && top.Type != "S7" {
		t.Fatalf("P2 primary %s", top.Type)
	}
}

// Эффект пилота: только из реальных полей, где данных нет — null.
func TestEffect(t *testing.T) {
	in := synthInput()
	rep := Analyze(in, nil)
	e := rep.Effect
	if e.PairsFlaggedEarly != rep.EarlierOrOnly || len(e.DetectionTable) != len(in.Survey.Participant) {
		t.Fatalf("%+v", e)
	}
	for _, row := range e.DetectionTable {
		if row.Pair == "P1" && (row.Ours != nil || row.Naive != nil) {
			t.Fatalf("P1 is healthy: %+v", row)
		}
		if row.Pair == "P2" && (row.Ours == nil || row.Naive != nil) {
			t.Fatalf("P2 is caught only by the system: %+v", row)
		}
	}
	// без дат поступления и без «сейчас» — просрочка неизвестна
	if e.VerdictCoverage == nil || *e.VerdictCoverage != 0 || e.Overdue != nil || e.OverdueUnknown != e.ActiveSignals || e.ActiveSignals == 0 {
		t.Fatalf("%+v", e)
	}
	// даты поступления есть: M6 пришёл 10 дней назад — сигналы со сроком < 10 дн. просрочены
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	for i := range in.Responses {
		in.Responses[i].At = now.AddDate(0, 0, -10*(7-in.Responses[i].Period)).Format(time.RFC3339)
	}
	in.Now = now
	all := Analyze(in, nil)
	var first SignalView
	for _, p := range all.Pairs {
		if len(p.Signals) > 0 {
			first = p.Signals[0]
			break
		}
	}
	verdicts := map[string]Verdict{first.ID: {Status: "confirmed"}}
	rep = Analyze(in, verdicts)
	e = rep.Effect
	if e.Overdue == nil || e.DecidedSignals != 1 || e.VerdictCoverage == nil || *e.VerdictCoverage <= 0 {
		t.Fatalf("%+v", e)
	}
	want := 0
	for _, p := range rep.Pairs {
		for _, s := range p.Signals {
			if s.SeenAt == "" {
				t.Fatalf("seen_at: %+v", s.ID)
			}
			at, _ := time.Parse(time.RFC3339, s.SeenAt)
			late := s.Verdict == nil && now.Sub(at) > time.Duration(s.DueDays)*24*time.Hour
			if late != s.Overdue {
				t.Fatalf("%s overdue %v", s.ID, s.Overdue)
			}
			if late {
				want++
			}
		}
	}
	if *e.Overdue != want || want == 0 {
		t.Fatalf("overdue %d want %d", *e.Overdue, want)
	}
	empty := Analyze(Input{Survey: survey.Default(), T: Default}, nil).Effect
	if empty.DetectionTable == nil || empty.VerdictCoverage != nil || empty.Overdue != nil {
		t.Fatalf("%+v", empty)
	}
}
func TestBriefKeepsSignalDeadlineAfterReopening(t *testing.T) {
	seen := "2026-11-12T12:00:00Z"
	p := PairReport{Signals: []SignalView{{Signal: Signal{ID: "P2-S7", Type: "S7", DueDays: 3}, SeenAt: seen}}}
	for _, day := range []int{12, 19} {
		brief := MakeBrief(p, time.Date(2026, 11, day, 12, 0, 0, 0, time.UTC))
		if got := brief.Signals[0]; got.SeenAt != seen || got.DueDays != 3 {
			t.Fatalf("deadline changed on opening day %d: %+v", day, got)
		}
	}
}
