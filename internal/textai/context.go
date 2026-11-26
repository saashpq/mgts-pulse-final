package textai

import (
	"regexp"
	"strings"
)

// Двусмысленные слова «уйти» и «конфликт» требуют контекста работы или
// отношений. Отрицание проверяем в той же части предложения, цитату сохраняем дословно.
const person = "(?:наставни|руководител|коллег|куратор|стажер|начальни|ментор)[а-я]*"

const target = "(?:меня|нас|стажер(?:а|ов|ку|ок)|коллегу|наставницу|наставника|куратора)(?:[^а-я]|$)"
const ridicule = "(?:высме(?:ял[аи]?|ивали?|ивала|ива[ею]т)|унизил[аи]?|унижа(?:л[аи]?|[ею]т)|оскорб(?:ил[аи]?|ля[ею]т))"
const ridiculeOver = "(?:сме(?:ял[аи]?сь|ется|ются)|насмеха(?:л[аи]?сь|ется|ются)|издева(?:л[аи]?сь|ется|ются))"
const overTarget = "(?:надо мной|над (?:нами|стажер(?:ом|ами|кой)|коллегой))"
const intent = "(?:хочу|думаю|планирую|решил[аи]?|собираюсь|намерен[а]?|хотел[а]? бы)"
const program = "(?:(?:эту|свою) )?(?:стажировку|программу стажировки)"
const early = "(?:досрочно|раньше срока)"

var (
	workloadCue       = regexp.MustCompile(`(?:высок|чрезмерн|непосильн)[а-я]* (?:на|за)грузк|(?:на|за)грузк[а-я]* (?:была |стала |слишком |очень )*(?:высок|выше комфорт|чрезмерн|непосильн)`)
	clauseBoundary    = regexp.MustCompile("[,;:!?\\n]+| (?:но|зато|однако) ")
	technicalConflict = regexp.MustCompile("конфликт[а-я]* (?:зависимост|верси|пакет|веток|слияни|портов|адресов|имен|ресурс)[а-я]*")
	conflictCue       = regexp.MustCompile(
		"(?:конфликт[а-я]* (?:с (?:" + person + "|ним|ней|нами|ними)|в (?:паре|команде))|" +
			"(?:в паре|между нами|между мной и " + person + "|с " + person + ")[^,;.!?]{0,48}конфликт|" +
			ridicule + " " + target + "|" + ridiculeOver + " " + overTarget + "|" + overTarget + " " + ridiculeOver + "|" +
			"руга[а-я]* (?:меня|нас|при всех)|меня (?:оскорб|унижа|униз|высме)|поссорил[а-я]* с |с [а-я]+ поссорил|" +
			"грубит|грубят|грубил|груба? (?:со мной|с нами)|(?:наставник|руководитель|коллега)(?: [^ ]+){0,3} груб(?:а|о)?(?:[ .]|$))")
	conflictDenied = regexp.MustCompile(
		"(?:без|нет|никаких) конфликт|конфликт[а-я]*(?: [а-я]+){0,3} (?:нет|не было|исчерпан|разрешен)|" +
			"не (?:грубит|грубил|ругает|ругал|оскорб|унижа|униз|высме|насмех|сме[ея]|издева|поссор)")
	departureCue = regexp.MustCompile(
		"увольняюсь|(?:подаю|подал[аи]?) заявление (?:об увольнении|на увольнение)|" +
			intent + " (?:уволиться|увольняться|уйти[ .]*$)|" +
			"(?:" + intent + " (?:уйти|покинуть)|ухожу|покидаю) (?:из (?:программы стажировки|команды|компании)|со стажировки|стажировку|программу стажировки|команду|компанию)(?:[^а-я]|$)|" +
			"(?:" + intent + " (?:" + early + " )?(?:прекратить|прервать)|прекращаю|прерываю) " + program + "|" +
			intent + " (?:" + early + " (?:завершить|закончить) " + program + "|(?:закончить|завершить) " + program + " " + early + ")|" +
			"(?:ищу|смотрю|рассматриваю|пода[а-я]* заявку на|отправ[а-я]* заявку на) (?:другие стажировки|другую стажировку)")
	departureDenied = regexp.MustCompile(
		"не (?:" + intent + "|буду) (?:" + early + " )?(?:уходить|уйти|покидать|покинуть|увольняться|уволиться|прекра|прерыв|прерва|законч|заверш)|" +
			"не (?:увольняюсь|ухожу|покидаю|прерываю|прекращаю|ищу|смотрю|рассматриваю|подаю)")
)

// MatchesClause checks whether a particular clause supports this theme.
func (t Theme) MatchesClause(clause string) bool {
	return matchesTheme(t, normText(clause))
}

func matchesTheme(t Theme, sentence string) bool {
	if t.Code == "overload" {
		for _, clause := range clauseBoundary.Split(sentence, -1) {
			if !containsAny(clause, t.exclude) && (workloadCue.MatchString(clause) || containsAny(clause, t.markers)) {
				return true
			}
		}
		return false
	}
	if t.Code != "conflict" && t.Code != "attrition" {
		return !containsAny(sentence, t.exclude) && containsAny(sentence, t.markers)
	}
	cue, denied := conflictCue, conflictDenied
	if t.Code == "attrition" {
		cue, denied = departureCue, departureDenied
	}
	for _, clause := range clauseBoundary.Split(sentence, -1) {
		clause = strings.TrimSpace(clause)
		if t.Code == "conflict" {
			clause = technicalConflict.ReplaceAllString(clause, " ")
		}
		if !denied.MatchString(clause) && (containsAny(clause, t.markers) || cue.MatchString(clause)) {
			return true
		}
	}
	return false
}
