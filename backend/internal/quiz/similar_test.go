package quiz

import (
	"strings"
	"testing"
)

func TestSimilar(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"¿Qué ocurre si se escribe en un map de Go desde varias goroutines sin sincronización?",
			"¿Que ocurre si se escribe en un MAP de Go desde varias goroutines sin sincronizacion?", true},
		{"¿Qué ocurre si se escribe en un map de Go desde varias goroutines sin sincronización?",
			"¿Qué pasa cuando varias goroutines escriben en un map de Go sin sincronización alguna?", true},
		{"¿Qué ocurre si se escribe en un map de Go desde varias goroutines sin sincronización?",
			"¿Para qué sirve context.WithTimeout en un handler HTTP de Go?", false},
		{"¿Qué nivel de aislamiento usa PostgreSQL por defecto?",
			"¿Qué problema evita un índice compuesto en PostgreSQL sobre consultas con ORDER BY?", false},
		{"", "algo", false},
	}
	for _, c := range cases {
		if got := Similar(c.a, c.b); got != c.want {
			t.Errorf("Similar(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestDuplicates(t *testing.T) {
	q := sample(4)
	q.Questions[0].Question = "¿Qué ocurre si se escribe en un map de Go desde varias goroutines sin sincronización?"
	q.Questions[0].Concept = "Escrituras concurrentes en maps"
	q.Questions[1].Question = "¿Para qué sirve context.WithTimeout en un handler HTTP de Go?"
	q.Questions[1].Concept = "Timeouts con context"
	q.Questions[2].Question = "¿Para qué sirve context.WithTimeout dentro de un handler HTTP en Go?"
	q.Questions[2].Concept = "Cancelación de requests"
	// Different scenario and wording, same concept as a previous question.
	q.Questions[3].Question = "Un usuario logueado cambia la URL /api/accounts/1001/statements por /1002/ y ve movimientos ajenos. ¿Qué vulnerabilidad es?"
	q.Questions[3].Concept = "IDOR/BOLA en endpoint de estados de cuenta"
	previous := []Question{
		{Question: "¿Qué pasa cuando varias goroutines escriben en un map de Go sin sincronización alguna?", Concept: "Maps y concurrencia"},
		{Question: "La API expone GET /api/accounts/{accountId}/statements y valida el JWT. Un pentest reporta acceso a cuentas ajenas. ¿Qué falta?",
			Concept: "IDOR en endpoints de cuentas"},
	}

	got := Duplicates(q, previous)
	if len(got) != 3 {
		t.Fatalf("expected 3 duplicates, got %d: %v", len(got), got)
	}
	want := []string{"pregunta 1 repite un concepto", "pregunta 3 repite la pregunta 2", "pregunta 4 repite un concepto"}
	for i, w := range want {
		if !strings.Contains(got[i], w) {
			t.Errorf("duplicate %d: want %q, got %q", i, w, got[i])
		}
	}
}

func TestSameConceptKeepsDifferentConceptsApart(t *testing.T) {
	pairs := [][2]string{
		{"Métricas para clases desbalanceadas", "Validación out-of-time en scoring"},
		{"Índices compuestos en PostgreSQL", "Niveles de aislamiento en PostgreSQL"},
		{"Instancias reservadas para carga estable", "Autoscaling ante picos"},
	}
	for _, p := range pairs {
		if sameConcept(keywords(p[0]), keywords(p[1])) {
			t.Errorf("%q and %q should be different concepts", p[0], p[1])
		}
	}
}
