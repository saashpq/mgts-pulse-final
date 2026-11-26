package ingest

import (
	"strings"
	"testing"

	"github.com/ElzaraA/mgts-ai/internal/survey"
)

func TestOrdinalWorkbookRejectsDuplicatesAndFractionsWithRows(t *testing.T) {
	sv := survey.Default()
	sv.FractionalHistory = false
	tables, err := ReadFile("forms.csv", []byte("intern_id,period,i_overall\nI-01,1,4\nI-01,1,2\nI-01,2,3.5\nI-01,3,5\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := Import(tables, &sv, survey.Intern, "import")
	if got.Accepted != 2 || got.Rejected != 2 || !strings.Contains(got.Sheets[0].Errors[0], "строка 3") || !strings.Contains(got.Sheets[0].Errors[1], "строка 4") || got.Responses[0].Scores["i_overall"] != 4 {
		t.Fatalf("%+v", got)
	}
}
