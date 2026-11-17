# questions.json schema

```json
{
  "title": "Backend Developer Go – Fintech",
  "role": "Backend Developer",
  "seniority": "Semi-senior",
  "language": "es",
  "difficulty": "mixed",
  "questions": [
    {
      "id": 1,
      "topic": "Go",
      "concept": "Escrituras concurrentes en maps",
      "question": "¿Qué ocurre si se escribe en un map de Go desde varias goroutines sin sincronización?",
      "options": [
        "El runtime puede abortar el programa con 'concurrent map writes'",
        "Go serializa las escrituras automáticamente",
        "Solo se pierde la última escritura",
        "El compilador lo detecta y no compila"
      ],
      "answer": 0,
      "explanation": "Los maps no son seguros para escrituras concurrentes; el runtime detecta la condición y hace panic fatal. Hay que usar sync.Mutex o sync.Map."
    }
  ]
}
```

## Fields

| Field | Type | Rules |
|---|---|---|
| `title` | string | Short title of the quiz, usually role + company/domain. |
| `role` | string | Role name extracted from the posting. |
| `seniority` | string | Seniority stated or inferred from the posting. |
| `language` | `"es"` \| `"en"` | Same as requested. |
| `difficulty` | `"junior"` \| `"semi"` \| `"senior"` \| `"mixed"` | Same as requested. |
| `questions` | array | Exactly `count` items. |
| `questions[].id` | integer | 1..count, sequential. |
| `questions[].topic` | string | 1-3 words. |
| `questions[].concept` | string | 3-8 words: the precise idea tested. Used to avoid repeating concepts across quizzes of the same requirement. |
| `questions[].question` | string | Non-empty. |
| `questions[].options` | string[4] | Exactly 4 distinct, non-empty strings. |
| `questions[].answer` | integer | 0-3, index of the correct option. |
| `questions[].explanation` | string | Non-empty, 1-3 sentences. |
