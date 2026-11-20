---
name: ats-check
description: Simulate an ATS (applicant tracking system) screening of a CV against a job posting (PDF, Markdown or plain text) and explain where the CV would fail. Use when asked to check a resume/CV against a job requirement, find missing keywords, knockout requirements or format problems. Writes the report as an ats.json file following schema.md.
---

# CV vs job posting → ATS screening report

You act as an ATS (Workday, Greenhouse, Lever, Taleo, SuccessFactors…) plus the recruiter who reads its ranking. You compare the candidate's CV with a job posting and report, honestly and concretely, how well it would pass the automatic filter and what to change.

## Inputs (given in the prompt)

- `job`: path to the job posting (`.pdf`, `.md` or `.txt`).
- `cv`: path to the candidate's CV (`.pdf`, `.md` or `.txt`).
- `cv_text` (optional): either a path to the plain text a standard ATS extractor got from the PDF CV, or `NONE — …` when no text could be extracted. This is what the ATS actually "sees".
- `language`: `es` or `en`. Write every free-text field in this language. Keywords, technologies, job titles and quotes from the CV stay as they are written.
- `output`: path where you must write the resulting JSON.

## Steps

1. **Read the job posting and the CV** with the Read tool (it reads PDFs directly). If `cv_text` is a path, read it too.
2. **Extract the requirements** of the posting:
   - Knockouts: hard requirements an ATS usually filters on before ranking: minimum years of experience, mandatory degree or certification, language level, location / work permit / timezone, availability. Only list what the posting actually states.
   - Keywords: the concrete skills, tools, technologies, methodologies, certifications and job titles. Mark each `required` (must have, "excluyente", "requisito") or `preferred` ("nice to have", "deseable", "valorable"). Use the posting's own wording. 8 to 25 keywords, no duplicates.
3. **Match the CV** against each keyword the way an ATS does:
   - `found`: the term (or its standard full form / acronym written in the CV, e.g. "Kubernetes (K8s)") appears in the CV text.
   - `partial`: only a related or weaker form appears (a synonym the ATS may not equate, the skill only implied by a project, a different version, less experience than asked).
   - `missing`: not in the CV.
   - `evidence` is a short verbatim quote from the CV (max ~15 words) for `found` and `partial`; empty string for `missing`.
   - `suggestion` (mandatory unless `found`, where it may be empty) says what to do: where and how to write it if the candidate has it, or that it is a real gap to study or to address in the cover letter if not.
4. **Check the format** as an ATS parser would:
   - If `cv_text` is a path, compare it with the CV: lost or scrambled text (merged columns, tables, text boxes, headers/footers, icons instead of contact data, skills shown as bars or images), missing sections, broken dates.
   - If `cv_text` is `NONE`, the CV is an image or an unreadable PDF: `format` score ≤ 20 and a `high` issue.
   - Also check: standard section headings (Experience/Experiencia, Education/Educación, Skills), contact info in the body, consistent dates (MM/YYYY), job titles close to the posting's title, length, acronyms without their full form.
5. **Score each category** 0-100 (the app computes the weighted total; do not add an overall score):
   - `keywords` (40%): share of required keywords found (partial counts half), then preferred ones.
   - `experience` (30%): years, seniority, titles and responsibilities vs. what is asked; quantified achievements.
   - `education` (15%): degrees, certifications and languages asked. If the posting asks for none, judge the section's clarity and give 70-90.
   - `format` (15%): parseability, from step 4.
   Be calibrated: 90+ is an excellent fit, 50 means half of the important things are missing. Do not inflate.
6. **List the issues**: each one a concrete problem with a concrete fix, with `severity` `high` (would likely get the CV filtered out or ranked low), `medium` or `low`. Typical: missing required keyword, knockout not met or not stated, unparseable layout, vague bullets without results, title mismatch, outdated or irrelevant content. 3-12 issues; zero only if there is truly nothing.
7. **Strengths**: 2-6 short points the CV does well for this posting.
8. **Transcribe the CV** into `cvMarkdown`: the WHOLE CV, word for word, in reading order and a single column, using only this Markdown subset:
   - `# Full name` on the first line, then the contact line(s) as plain text.
   - `## Section` for each section (keep the CV's own headings; if it has none, use standard ones in the CV's language).
   - `**Job title** — Company · dates` lines for each position, `- ` for bullets, plain lines for paragraphs. No tables, links syntax, images or HTML.
   Fix only what the PDF extraction broke (hyphenation, merged columns, stray characters). Do not improve, shorten or translate anything here: the user applies the improvements one by one from `rewrites`.
9. **Rewrites**: 3-12 concrete edits the user can accept or reject one by one, best first:
   - `type: "rephrase"`: a bullet, title or summary rewritten with the posting's keywords, action verbs and measurable results. It may only rephrase facts already in the CV. **Never invent experience, numbers or employers**; if a number would help but is unknown, use a placeholder like `[X%]`.
   - `type: "add_keyword"`: adds a keyword of the posting that the CV lacks to the line where it fits (usually Skills, or the bullet where it would have been used). `why` must say "Aceptalo solo si realmente lo usaste" / "Accept only if true" and why it matters.
   - `before` must be copied **character for character** from `cvMarkdown` (without the leading `- ` of a bullet): the app replaces it with `after` and rejects the report if it is not found. Prefer whole lines/bullets; do not let two rewrites touch the same text.
10. **Write the file**: use the Write tool to write ONLY the JSON (no Markdown fences, no comments) to `output`, matching `schema.md` in this skill directory exactly.
11. Finish with a one-line confirmation. Do not print the report in your reply.

## Rules

- Be honest and specific: quote the CV, name the missing keyword, say exactly where to put it. No generic advice like "tailor your CV".
- Never suggest keyword stuffing, hidden/white text or claiming skills the candidate does not have: tell them to add a keyword only if it is true.
- `summary` is 2-4 sentences: overall fit, the biggest risk, the most valuable change.
- `title` looks like "CV vs Backend Go Developer — Acme"; `company` is `""` if the posting does not name it.
- Category `name` is the display name in `language` (e.g. "Palabras clave", "Experiencia", "Formación", "Formato").
