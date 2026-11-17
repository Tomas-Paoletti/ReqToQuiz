---
name: job-quiz
description: Generate a multiple-choice interview practice quiz from a job requirement / job posting (PDF, Markdown or plain text). Use when asked to create quiz questions about the skills, technologies and responsibilities a job posting requires. Writes the questions as a questions.json file following schema.md.
---

# Job requirement → multiple-choice quiz

You turn a job posting into a multiple-choice quiz that helps a candidate practice for the interview.

## Inputs (given in the prompt)

- `input`: path to the job requirement file (`.pdf`, `.md` or `.txt`).
- `count`: exact number of questions to create (5, 10, 20, 30 or 40).
- `language`: `es` (Spanish) or `en` (English). Write EVERYTHING (title, topics, questions, options, explanations) in this language, even if the posting is in another language. Technology names stay as-is.
- `difficulty`: `junior`, `semi` (semi-senior / mid), `senior` or `mixed` (spread across the three; if the posting states a seniority, lean toward it).
- `output`: path where you must write the resulting JSON.
- `avoid` (optional): path to a Markdown list of questions already asked in earlier quizzes for this same requirement. When present, none of your questions may repeat them, rephrase them or ask the same thing in another language.

## Steps

1. **Read the input file** with the Read tool (it reads PDFs directly). Do not use any other tool to read it.
2. **Analyze the posting**: role title, seniority, required technologies, tools, methodologies, domain knowledge, soft skills and responsibilities. Distinguish "required" from "nice to have".
3. **Plan the topic distribution**: allocate the `count` questions across topics proportionally to their weight in the posting. Required technical skills get most questions; nice-to-haves and soft skills get a few. Avoid more than ~30% of questions on a single topic unless the posting is about one thing only.
   - If `avoid` is given, **read that file first**: it lists the concepts already tested. Keep the same topic weights, but pick *different concepts* within each topic (e.g. if Go channels already covered "reading from a closed channel", ask about `select` with `default`, buffered vs unbuffered, or nil channels instead). Changing only the scenario, numbers or wording does NOT make it a new concept. The app checks wording and concepts automatically and rejects repeats.
4. **Write the questions** following the quality rules below. Give each one a `concept`: the precise idea it tests (3-8 words, e.g. "IDOR en endpoints de cuentas", "Métricas para clases desbalanceadas"), more specific than `topic`.
5. **Self-check against `avoid`** (only if given): for every question, ask "does any listed concept test the same idea, even with another scenario, numbers or wording?". Typical hidden repeats: same vulnerability with a different URL, same metric choice with a different percentage, same pricing model with a different workload. Replace every match with a question on a genuinely different concept, and also make sure no two of your own questions test the same concept.
6. **Write the file**: use the Write tool to write ONLY the JSON (no Markdown fences, no comments) to `output`, matching `schema.md` in this skill directory exactly.
7. Finish with a one-line confirmation. Do not print the questions in your reply.

## Quality rules

- Exactly 4 options per question, exactly one correct. `answer` is the 0-based index of the correct option.
- Questions test real understanding someone in this role needs: concepts, "what happens when…", choosing the right tool/approach, debugging scenarios, best practices. Avoid trivia (release years, author names) and avoid questions answerable just by reading the posting.
- Distractors must be plausible: common misconceptions, near-correct alternatives, similar-sounding APIs. No joke options, no "all of the above" / "none of the above".
- Options should have similar length and style so the correct one does not stand out.
- Spread the correct answer position across 0-3 (the app also shuffles, but don't be lazy).
- Each `explanation` (1-3 sentences) says why the correct option is right and, when useful, why the most tempting distractor is wrong.
- Difficulty: `junior` = fundamentals and definitions; `semi` = applied usage and trade-offs; `senior` = architecture, edge cases, performance, design decisions.
- No duplicated or near-duplicated questions.
- Short code snippets inside `question` are allowed (use `\n` for newlines inside the JSON string); keep them under ~10 lines.
- `topic` is a short label (1-3 words) such as "Docker", "SQL", "REST APIs", "Comunicación".

## Output

See `schema.md` for the exact JSON format. The file must be valid JSON parseable by a strict parser (double quotes, no trailing commas) and must contain exactly `count` questions with `id` from 1 to `count`.
