# ats.json schema

Write a single JSON object with exactly these fields (no extra ones):

```json
{
  "title": "CV vs Backend Go Developer — Acme",
  "role": "Backend Go Developer",
  "company": "Acme",
  "language": "es",
  "summary": "El CV cubre bien Go y PostgreSQL pero no menciona Kafka ni Kubernetes, ambos excluyentes. El mayor riesgo es el diseño en dos columnas, que el ATS lee mezclado. Agregar Kubernetes (si lo usaste) y pasar a una columna sube bastante el puntaje.",
  "categories": [
    { "id": "keywords",   "name": "Palabras clave", "score": 62, "comment": "7 de 11 obligatorias presentes; faltan Kafka y Kubernetes." },
    { "id": "experience", "name": "Experiencia",    "score": 75, "comment": "4 años en backend, piden 3+. Faltan logros medibles." },
    { "id": "education",  "name": "Formación",      "score": 80, "comment": "Título afín; el inglés B2 pedido no figura." },
    { "id": "format",     "name": "Formato",        "score": 40, "comment": "Dos columnas: el texto extraído mezcla experiencia y skills." }
  ],
  "knockouts": [
    { "requirement": "3+ años de experiencia con Go", "status": "met", "detail": "Go desde 2021 en Empresa X." },
    { "requirement": "Inglés B2", "status": "unclear", "detail": "El CV no menciona el nivel de inglés." }
  ],
  "keywords": [
    { "keyword": "Go", "importance": "required", "status": "found", "evidence": "Desarrollo de microservicios en Go", "suggestion": "" },
    { "keyword": "Kubernetes", "importance": "required", "status": "missing", "evidence": "", "suggestion": "Si desplegaste en K8s, agregalo en Skills y en el bullet de despliegues de Empresa X." },
    { "keyword": "CI/CD", "importance": "preferred", "status": "partial", "evidence": "automaticé los deploys", "suggestion": "Nombrá la herramienta (GitHub Actions, GitLab CI) y escribí CI/CD literal." }
  ],
  "issues": [
    { "severity": "high", "area": "format", "problem": "El CV está en dos columnas y el ATS mezcla el texto.", "fix": "Usá una sola columna con secciones estándar." }
  ],
  "strengths": ["Experiencia en Go y PostgreSQL alineada con el puesto."],
  "rewrites": [
    { "type": "rephrase", "section": "Experiencia — Empresa X", "before": "Trabajé en la API de pagos.", "after": "Desarrollé la API de pagos en Go y PostgreSQL, reduciendo la latencia un [X%].", "why": "Usa las keywords del aviso, un verbo de acción y un resultado medible." },
    { "type": "add_keyword", "section": "Skills", "before": "Go, PHP, PostgreSQL, Docker, Git", "after": "Go, PHP, PostgreSQL, Docker, Kubernetes, Git", "why": "Kubernetes es excluyente en el aviso. Aceptalo solo si realmente lo usaste." }
  ],
  "cvMarkdown": "# Juan Pérez\nBackend Developer · Buenos Aires · juan@example.com\n\n## Experiencia\n**Desarrollador Backend** — Empresa X · 03/2021 – actualidad\n- Trabajé en la API de pagos.\n\n## Skills\nGo, PHP, PostgreSQL, Docker, Git\n"
}
```

| Field | Rules |
|---|---|
| `title`, `role`, `summary` | non-empty |
| `company` | string, `""` if unknown |
| `language` | `es` or `en` |
| `categories` | exactly 4, ids `keywords`, `experience`, `education`, `format`, once each; `score` integer 0-100; `name` and `comment` non-empty |
| `knockouts[]` | `status` `met`, `unclear` or `not_met`; `requirement` and `detail` non-empty. May be `[]` |
| `keywords[]` | at least 5, no duplicates; `importance` `required` or `preferred`; `status` `found`, `partial` or `missing`; `evidence` non-empty unless `missing` (then `""`); `suggestion` non-empty unless `found` (may be `""` then) |
| `issues[]` | `severity` `high`, `medium` or `low`; `area` one of `format`, `keywords`, `experience`, `education`, `content`; `problem` and `fix` non-empty |
| `strengths` | array of non-empty strings |
| `rewrites[]` | at least 1; `type` `rephrase` or `add_keyword`; `section`, `before`, `after`, `why` non-empty; `before` must appear character for character in `cvMarkdown` (whitespace may differ) and `after` must differ from it |
| `cvMarkdown` | the whole CV transcribed with `#` name, `##` sections, `**bold**`, `- ` bullets and plain lines only |
