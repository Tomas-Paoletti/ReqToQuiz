export type Difficulty = "junior" | "semi" | "senior" | "mixed";
export type Language = "es" | "en";
export type Status = "generating" | "ready" | "error";
export type EngineId = "claude" | "codex";

export type EngineChoice = "auto" | EngineId;
export type AuthState = "logged_in" | "logged_out" | "unknown";

export interface EngineStatus {
  id: EngineId;
  name: string;
  installed: boolean;
  /** Present only when installed. */
  auth?: { state: AuthState; detail?: string; loginCommand?: string };
}

export interface Health {
  /** Installed engines first (in fallback order), then the missing ones. */
  engines: EngineStatus[];
}

/** An engine can be used unless it is missing or known to be logged out. */
export const usable = (e: EngineStatus) => e.installed && e.auth?.state !== "logged_out";

export const engineLabel: Record<EngineId, string> = { claude: "Claude Code", codex: "Codex" };

export interface Attempt {
  score: number;
  correct: number;
  total: number;
  at: string;
}

export type Kind = "quiz" | "ats";
export type Verdict = "pass" | "borderline" | "reject";

export interface QuizMeta {
  id: string;
  /** Absent on quizzes. */
  kind?: "ats";
  title: string;
  role: string;
  seniority: string;
  count: number;
  language: Language;
  difficulty: Difficulty;
  source: string;
  /** Earlier quizzes from the same requirement whose questions were excluded. */
  previousQuizzes: number;
  /** Engine that generated the quiz. */
  engine?: EngineId;
  status: Status;
  error?: string;
  createdAt: string;
  lastAttempt?: Attempt;
  attempts: number;
  /** ATS reports only. */
  cvSource?: string;
  ats?: { score: number; verdict: Verdict };
}

export const kindOf = (m: QuizMeta): Kind => (m.kind === "ats" ? "ats" : "quiz");

export const verdictLabel: Record<Verdict, string> = {
  pass: "Pasaría el filtro",
  borderline: "En el límite",
  reject: "Probablemente filtrado",
};

export interface ProgressEvent {
  type: "progress" | "done" | "error";
  step?: string;
  message?: string;
  url?: string;
  engine?: EngineId;
}

export interface CreateQuizInput {
  file?: File;
  text?: string;
  count: number;
  language: Language;
  difficulty: Difficulty;
  engine: EngineChoice;
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new Error(body?.error ?? `Error ${res.status}`);
  }
  return res.status === 204 ? (undefined as T) : res.json();
}

export const getHealth = (refresh = false) => request<Health>(`/api/health${refresh ? "?refresh=1" : ""}`);

export const listQuizzes = () => request<QuizMeta[]>("/api/quizzes");

export const getQuiz = (id: string) => request<QuizMeta>(`/api/quizzes/${id}`);

export const deleteQuiz = (id: string) => request<void>(`/api/quizzes/${id}`, { method: "DELETE" });

export function createQuiz(input: CreateQuizInput) {
  const form = new FormData();
  if (input.file) form.append("file", input.file);
  else form.append("text", input.text ?? "");
  form.append("count", String(input.count));
  form.append("language", input.language);
  form.append("difficulty", input.difficulty);
  form.append("engine", input.engine);
  return request<QuizMeta>("/api/quizzes", { method: "POST", body: form });
}

export interface CreateAtsInput {
  file?: File;
  text?: string;
  cv: File;
  language: Language;
  engine: EngineChoice;
}

export function createAts(input: CreateAtsInput) {
  const form = new FormData();
  if (input.file) form.append("file", input.file);
  else form.append("text", input.text ?? "");
  form.append("cv", input.cv);
  form.append("language", input.language);
  form.append("engine", input.engine);
  return request<QuizMeta>("/api/ats", { method: "POST", body: form });
}

export interface Rewrite {
  type: "rephrase" | "add_keyword";
  section: string;
  before: string;
  after: string;
  why: string;
}

export interface AtsReport {
  title: string;
  role: string;
  company: string;
  language: Language;
  summary: string;
  score: number;
  verdict: Verdict;
  rewrites: Rewrite[];
  /** Whole CV transcribed to simple Markdown; absent on reports made before the editor existed. */
  cvMarkdown?: string;
}

export const getAtsReport = (id: string) =>
  request<{ meta: QuizMeta; report: AtsReport }>(`/api/ats/${id}/report`);

/** Page of a ready quiz or ATS report. */
export const pageUrl = (m: QuizMeta) => (kindOf(m) === "ats" ? `/ats/${m.id}/` : `/quizzes/${m.id}/`);

export const difficultyLabel: Record<Difficulty, string> = {
  junior: "Junior",
  semi: "Semi senior",
  senior: "Senior",
  mixed: "Mixta",
};

export const languageLabel: Record<Language, string> = { es: "Español", en: "English" };
