package quiz

import (
	"fmt"
	"strings"
	"unicode"
)

// Two questions count as repeated when, after normalizing, they are equal or
// their significant words overlap this much. Tuned to catch copies and light
// rephrasings without flagging different questions about the same topic.
const (
	jaccardThreshold     = 0.7
	containmentThreshold = 0.85
	containmentMinWords  = 6
)

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
		que qué cual cuál cuales cuáles como cómo cuando cuándo donde dónde por para con sin una uno unos unas los las del
		el la lo al es son ser está están esta este estos estas ese esa eso más mas muy entre sobre según siguiente siguientes
		hay tiene tienen debe deben puede pueden caso forma mejor ocurre ocurrir pasa pasar sucede suceder desde alguna alguno
		algun varias varios dentro usar utiliza utilizar
		happens happen occurs occur some several inside using use
		the and for with without what which when where how why who this that these those are is was were be been being
		from into about between following does do did can could should would will has have had best most`) {
		stopwords[w] = true
	}
}

// stemLen cuts words to a crude stem so "escribe"/"escriben" or
// "goroutine"/"goroutines" match.
const stemLen = 6

// keywords returns the set of significant (stemmed) words of a question.
func keywords(s string) map[string]bool {
	s = strings.Map(func(r rune) rune {
		switch r {
		case 'á', 'à', 'ä':
			return 'a'
		case 'é', 'è', 'ë':
			return 'e'
		case 'í', 'ì', 'ï':
			return 'i'
		case 'ó', 'ò', 'ö':
			return 'o'
		case 'ú', 'ù', 'ü':
			return 'u'
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, strings.ToLower(s))
	set := map[string]bool{}
	for _, w := range strings.Fields(s) {
		r := []rune(w)
		if len(r) < 3 || stopwords[w] {
			continue
		}
		if len(r) > stemLen {
			r = r[:stemLen]
		}
		set[string(r)] = true
	}
	return set
}

// Similar reports whether two questions are the same or a light rephrasing.
func Similar(a, b string) bool {
	return similarSets(keywords(a), keywords(b))
}

func similarSets(ka, kb map[string]bool) bool {
	if len(ka) == 0 || len(kb) == 0 {
		return false
	}
	inter := 0
	for w := range ka {
		if kb[w] {
			inter++
		}
	}
	union := len(ka) + len(kb) - inter
	if float64(inter)/float64(union) >= jaccardThreshold {
		return true
	}
	small := min(len(ka), len(kb))
	return small >= containmentMinWords && float64(inter)/float64(small) >= containmentThreshold
}

// sameConcept compares the short `concept` labels: besides the general rule,
// one label containing (almost) all the words of another counts as the same
// concept, e.g. "IDOR en endpoints de cuentas" vs "IDOR/BOLA en endpoint de
// estados de cuenta".
func sameConcept(a, b map[string]bool) bool {
	if similarSets(a, b) {
		return true
	}
	small := min(len(a), len(b))
	if small < 3 {
		return false
	}
	inter := 0
	for w := range a {
		if b[w] {
			inter++
		}
	}
	return float64(inter)/float64(small) >= 0.75
}

type fingerprint struct {
	question, concept map[string]bool
}

func fingerprintOf(q Question) fingerprint {
	return fingerprint{keywords(q.Question), keywords(q.Concept)}
}

func (a fingerprint) repeats(b fingerprint) bool {
	return similarSets(a.question, b.question) || sameConcept(a.concept, b.concept)
}

// Duplicates returns one message per question of q that repeats a previous
// question (from earlier quizzes of the same requirement) or an earlier
// question of q itself, by wording or by concept. Empty means no repeats.
func Duplicates(q *Quiz, previous []Question) []string {
	prev := make([]fingerprint, len(previous))
	for i, p := range previous {
		prev[i] = fingerprintOf(p)
	}
	var out []string
	seen := make([]fingerprint, 0, len(q.Questions))
	for i, qu := range q.Questions {
		f := fingerprintOf(qu)
		found := false
		for j, p := range prev {
			if f.repeats(p) {
				what := previous[j].Concept
				if what == "" {
					what = previous[j].Question
				}
				out = append(out, fmt.Sprintf("pregunta %d repite un concepto ya evaluado en un quiz anterior (%q)", i+1, truncate(what, 90)))
				found = true
				break
			}
		}
		if !found {
			for j, s := range seen {
				if f.repeats(s) {
					out = append(out, fmt.Sprintf("pregunta %d repite la pregunta %d de este mismo quiz", i+1, j+1))
					break
				}
			}
		}
		seen = append(seen, f)
	}
	return out
}

func truncate(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
