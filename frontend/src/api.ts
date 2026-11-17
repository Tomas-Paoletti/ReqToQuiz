export type Difficulty = "junior" | "semi" | "senior" | "mixed";
export type Language = "es" | "en";
export type Status = "generating" | "ready" | "error";
export type EngineId = "claude" | "codex";

export interface Health {
  /** Available engines, in fallback order. */
  engines: { id: EngineId; name: string }[];
}

export const engineLabel: Record<EngineId, string> = { claude: "Claude Code", codex: "Codex" };

export interface Attempt {
  score: number;
  correct: number;
  total: number;
  at: string;
}

export interface QuizMeta {
  id: string;
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
}

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
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new Error(body?.error ?? `Error ${res.status}`);
  }
  return res.status === 204 ? (undefined as T) : res.json();
}

export const getHealth = () => request<Health>("/api/health");

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
  return request<QuizMeta>("/api/quizzes", { method: "POST", body: form });
}

export const quizUrl = (id: string) => `/quizzes/${id}/`;

export const difficultyLabel: Record<Difficulty, string> = {
  junior: "Junior",
  semi: "Semi senior",
  senior: "Senior",
  mixed: "Mixta",
};

export const languageLabel: Record<Language, string> = { es: "Español", en: "English" };
