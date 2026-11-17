<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import {
  createQuiz,
  deleteQuiz,
  difficultyLabel,
  engineLabel,
  getHealth,
  languageLabel,
  listQuizzes,
  quizUrl,
  usable,
  type Difficulty,
  type EngineChoice,
  type EngineStatus,
  type Health,
  type Language,
  type QuizMeta,
} from "../api";

const router = useRouter();

const mode = ref<"file" | "text">("file");
const file = ref<File | null>(null);
const text = ref("");
const count = ref(10);
const language = ref<Language>("es");
const difficulty = ref<Difficulty>("mixed");
const dragging = ref(false);
const submitting = ref(false);
const formError = ref("");

const history = ref<QuizMeta[]>([]);
const historyError = ref("");
const health = ref<Health | null>(null);
const checking = ref(false);

// --- Engine choice and session status -------------------------------------
const ENGINE_KEY = "reqtoquiz.engine";
const storedChoice = (): EngineChoice => {
  try {
    const v = localStorage.getItem(ENGINE_KEY);
    if (v === "auto" || v === "claude" || v === "codex") return v;
  } catch {
    /* storage unavailable */
  }
  return "auto";
};
const engineChoice = ref<EngineChoice>(storedChoice());
watch(engineChoice, (v) => {
  try {
    localStorage.setItem(ENGINE_KEY, v);
  } catch {
    /* storage unavailable */
  }
});

const engines = computed(() => health.value?.engines ?? []);
const installed = computed(() => engines.value.filter((e) => e.installed));
const usableEngines = computed(() => installed.value.filter(usable));
/** The selector only makes sense when there is something to choose. */
const showSelector = computed(() => installed.value.length > 1);

const choices = computed(() => [
  { value: "auto" as EngineChoice, label: "Automático", disabled: usableEngines.value.length === 0 },
  ...installed.value.map((e) => ({ value: e.id as EngineChoice, label: e.name, disabled: !usable(e) })),
]);

// If the stored/selected choice is no longer usable, fall back to automatic.
watch(choices, (list) => {
  const current = list.find((c) => c.value === engineChoice.value);
  if (!current || current.disabled) engineChoice.value = "auto";
});

const choiceHint = computed(() => {
  const u = usableEngines.value;
  if (engineChoice.value !== "auto") return "Se usa solo este motor, sin respaldo.";
  if (u.length > 1) return `Usa ${u[0].name}; si falla, ${u.slice(1).map((e) => e.name).join(", ")}.`;
  if (u.length === 1) return `Usa ${u[0].name} (sin respaldo).`;
  return "";
});

/** Blocking problem with the engines, if any. */
const engineBlocker = computed(() => {
  if (!health.value) return "";
  if (installed.value.length === 0) return "No hay ningún motor instalado: instalá Claude Code o Codex.";
  if (usableEngines.value.length === 0)
    return "Ningún motor tiene sesión iniciada. Iniciá sesión y tocá «Volver a comprobar».";
  return "";
});

function statusText(e: EngineStatus) {
  if (!e.installed) return "No instalado";
  switch (e.auth?.state) {
    case "logged_in":
      return e.auth.detail ? `Sesión iniciada · ${e.auth.detail}` : "Sesión iniciada";
    case "logged_out":
      return "Sin sesión iniciada";
    default:
      return "No se pudo comprobar la sesión (se intenta igual)";
  }
}

function statusClass(e: EngineStatus) {
  if (!e.installed) return "off";
  return e.auth?.state === "logged_in" ? "ok" : e.auth?.state === "logged_out" ? "bad" : "warn";
}

async function loadHealth(refresh = false) {
  checking.value = true;
  try {
    health.value = await getHealth(refresh);
  } catch {
    /* backend down: the form will show the error on submit */
  } finally {
    checking.value = false;
  }
}

const counts = [5, 10, 20, 30, 40];
const accepted = [".pdf", ".md", ".markdown", ".txt"];

const canSubmit = computed(
  () =>
    !submitting.value &&
    !engineBlocker.value &&
    (mode.value === "file" ? !!file.value : text.value.trim().length >= 40),
);

function pickFile(f: File | undefined | null) {
  formError.value = "";
  if (!f) return;
  const ext = f.name.slice(f.name.lastIndexOf(".")).toLowerCase();
  if (!accepted.includes(ext)) {
    formError.value = "Formato no soportado: subí un .pdf, .md o .txt";
    return;
  }
  if (f.size > 20 * 1024 * 1024) {
    formError.value = "El archivo supera los 20 MB";
    return;
  }
  file.value = f;
}

function onDrop(e: DragEvent) {
  dragging.value = false;
  pickFile(e.dataTransfer?.files[0]);
}

function onInput(e: Event) {
  pickFile((e.target as HTMLInputElement).files?.[0]);
  (e.target as HTMLInputElement).value = "";
}

async function submit() {
  if (!canSubmit.value) return;
  submitting.value = true;
  formError.value = "";
  try {
    const quiz = await createQuiz({
      file: mode.value === "file" ? file.value! : undefined,
      text: mode.value === "text" ? text.value : undefined,
      count: count.value,
      language: language.value,
      difficulty: difficulty.value,
      engine: showSelector.value ? engineChoice.value : "auto",
    });
    router.push(`/generating/${quiz.id}`);
  } catch (e) {
    formError.value = (e as Error).message;
    submitting.value = false;
    loadHealth(true); // the error may be a session that just expired

  }
}

async function loadHistory() {
  try {
    history.value = await listQuizzes();
    historyError.value = "";
  } catch (e) {
    historyError.value = `No se pudo cargar el historial: ${(e as Error).message}`;
  }
}

async function remove(q: QuizMeta) {
  if (!confirm(`¿Borrar "${q.title}"?`)) return;
  try {
    await deleteQuiz(q.id);
    history.value = history.value.filter((h) => h.id !== q.id);
  } catch (e) {
    alert((e as Error).message);
  }
}

function open(q: QuizMeta) {
  if (q.status === "ready") window.location.href = quizUrl(q.id);
  else if (q.status === "generating") router.push(`/generating/${q.id}`);
}

const formatDate = (iso: string) =>
  new Date(iso).toLocaleString("es-AR", { day: "2-digit", month: "short", hour: "2-digit", minute: "2-digit" });

const scoreClass = (s: number) => (s >= 80 ? "good" : s >= 60 ? "mid" : "low");

onMounted(() => {
  loadHistory();
  loadHealth();
});
</script>

<template>
  <section class="card">
    <h2>Nuevo quiz</h2>

    <div class="tabs" role="tablist">
      <button :class="{ active: mode === 'file' }" role="tab" @click="mode = 'file'">Subir archivo</button>
      <button :class="{ active: mode === 'text' }" role="tab" @click="mode = 'text'">Pegar texto</button>
    </div>

    <label
      v-if="mode === 'file'"
      class="dropzone"
      :class="{ dragging, filled: file }"
      @dragover.prevent="dragging = true"
      @dragleave="dragging = false"
      @drop.prevent="onDrop"
    >
      <input type="file" :accept="accepted.join(',')" hidden @change="onInput" />
      <template v-if="file">
        <strong>{{ file.name }}</strong>
        <span class="muted">{{ (file.size / 1024).toFixed(0) }} KB · hacé click para cambiarlo</span>
      </template>
      <template v-else>
        <strong>Arrastrá el requerimiento acá o hacé click</strong>
        <span class="muted">PDF, Markdown o TXT · máx. 20 MB</span>
      </template>
    </label>

    <textarea
      v-else
      v-model="text"
      rows="10"
      placeholder="Pegá acá el aviso o la descripción del puesto…"
    ></textarea>

    <div class="options">
      <div class="field">
        <span class="label">Preguntas</span>
        <div class="segmented">
          <button v-for="c in counts" :key="c" :class="{ active: count === c }" @click="count = c">{{ c }}</button>
        </div>
      </div>
      <div class="field">
        <span class="label">Idioma</span>
        <div class="segmented">
          <button
            v-for="(label, key) in languageLabel"
            :key="key"
            :class="{ active: language === key }"
            @click="language = key"
          >
            {{ label }}
          </button>
        </div>
      </div>
      <div class="field">
        <span class="label">Dificultad</span>
        <div class="segmented">
          <button
            v-for="(label, key) in difficultyLabel"
            :key="key"
            :class="{ active: difficulty === key }"
            @click="difficulty = key"
          >
            {{ label }}
          </button>
        </div>
      </div>
      <div v-if="showSelector" class="field">
        <span class="label">Motor</span>
        <div class="segmented">
          <button
            v-for="c in choices"
            :key="c.value"
            :class="{ active: engineChoice === c.value }"
            :disabled="c.disabled"
            :title="c.disabled ? 'Sin sesión iniciada' : ''"
            @click="engineChoice = c.value"
          >
            {{ c.label }}
          </button>
        </div>
        <span v-if="choiceHint" class="muted hint-inline">{{ choiceHint }}</span>
      </div>
    </div>

    <div v-if="health" class="engines">
      <div class="engines-head">
        <span class="label">Motores</span>
        <button class="small" :disabled="checking" @click="loadHealth(true)">
          {{ checking ? "Comprobando…" : "Volver a comprobar" }}
        </button>
      </div>
      <ul>
        <li v-for="e in engines" :key="e.id">
          <span class="dot" :class="statusClass(e)"></span>
          <strong>{{ e.name }}</strong>
          <span class="muted">{{ statusText(e) }}</span>
          <code v-if="e.auth?.state === 'logged_out' && e.auth.loginCommand">{{ e.auth.loginCommand }}</code>
        </li>
      </ul>
      <p v-if="engineBlocker" class="error-box">{{ engineBlocker }}</p>
    </div>

    <button class="primary generate" :disabled="!canSubmit" @click="submit">
      {{ submitting ? "Enviando…" : "Generar quiz" }}
    </button>
    <p v-if="mode === 'text' && text.trim().length > 0 && text.trim().length < 40" class="muted hint">
      El texto es muy corto para armar preguntas.
    </p>
    <p v-if="formError" class="error-box">{{ formError }}</p>
  </section>

  <section class="card">
    <h2>Historial</h2>
    <p v-if="historyError" class="error-box">{{ historyError }}</p>
    <p v-else-if="history.length === 0" class="muted">Todavía no generaste ningún quiz.</p>
    <ul v-else class="history">
      <li v-for="q in history" :key="q.id">
        <div class="info" :class="{ clickable: q.status !== 'error' }" @click="open(q)">
          <div class="title">{{ q.title }}</div>
          <div class="meta muted">
            {{ formatDate(q.createdAt) }} · {{ q.count }} preguntas · {{ languageLabel[q.language] }} ·
            {{ difficultyLabel[q.difficulty] }} · {{ q.source }}
            <template v-if="q.engine"> · {{ engineLabel[q.engine] }}</template>
          </div>
          <div v-if="q.previousQuizzes > 0" class="variant">
            Variante {{ q.previousQuizzes + 1 }} · sin repetir preguntas de
            {{ q.previousQuizzes === 1 ? "el quiz anterior" : `los ${q.previousQuizzes} quizzes anteriores` }}
          </div>
          <div v-if="q.status === 'error'" class="err">Error: {{ q.error }}</div>
        </div>
        <div class="side">
          <span v-if="q.status === 'generating'" class="badge">Generando…</span>
          <span v-else-if="q.status === 'error'" class="badge bad">Error</span>
          <span v-else-if="q.lastAttempt" class="score" :class="scoreClass(q.lastAttempt.score)">
            {{ Math.round(q.lastAttempt.score) }}%
          </span>
          <span v-else class="badge">Sin rendir</span>
          <button v-if="q.status === 'ready'" class="small" @click="open(q)">Abrir</button>
          <button v-if="q.status !== 'generating'" class="small danger" @click="remove(q)">Borrar</button>
        </div>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.tabs { display: flex; gap: 4px; margin-bottom: 12px; border-bottom: 1px solid var(--border); }
.tabs button {
  border: none;
  border-bottom: 2px solid transparent;
  border-radius: 0;
  background: none;
  color: var(--muted);
  padding: 8px 12px;
}
.tabs button.active { color: var(--accent); border-bottom-color: var(--accent); font-weight: 600; }

.dropzone {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-height: 150px;
  border: 2px dashed var(--border);
  border-radius: var(--radius);
  cursor: pointer;
  text-align: center;
  padding: 16px;
  transition: border-color 0.15s, background 0.15s;
}
.dropzone:hover,
.dropzone.dragging { border-color: var(--accent); background: var(--accent-soft); }
.dropzone.filled { border-style: solid; border-color: var(--accent); }

textarea {
  width: 100%;
  font: inherit;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 12px;
  resize: vertical;
}
textarea:focus { outline: 2px solid var(--accent); outline-offset: -1px; }

.options { display: grid; gap: 14px; margin: 18px 0; }
.field { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; }
.label { min-width: 90px; font-weight: 600; font-size: 0.9rem; }
.segmented { display: flex; flex-wrap: wrap; border: 1px solid var(--border); border-radius: 8px; overflow: hidden; }
.segmented button { border: none; border-radius: 0; border-right: 1px solid var(--border); padding: 6px 14px; }
.segmented button:last-child { border-right: none; }
.segmented button.active { background: var(--accent); color: #fff; }

.generate { width: 100%; padding: 12px; font-size: 1.05rem; }
.segmented button:disabled { background: none; text-decoration: line-through; }
.hint-inline { font-size: 0.82rem; }

.engines { border: 1px solid var(--border); border-radius: 8px; padding: 10px 14px; margin: 0 0 16px; }
.engines-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; }
.engines ul { list-style: none; margin: 0; padding: 0; display: grid; gap: 6px; }
.engines li { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; font-size: 0.88rem; }
.engines code { font-size: 0.8rem; background: var(--bad-soft); color: var(--bad); padding: 1px 6px; border-radius: 4px; }
.engines .error-box { margin-top: 10px; }
.dot { width: 9px; height: 9px; border-radius: 50%; flex-shrink: 0; background: var(--border); }
.dot.ok { background: var(--ok); }
.dot.bad { background: var(--bad); }
.dot.warn { background: #ca8a04; }
.hint { font-size: 0.85rem; margin: 8px 0 0; }

.history { list-style: none; margin: 0; padding: 0; }
.history li { display: flex; gap: 12px; align-items: center; padding: 12px 0; border-top: 1px solid var(--border); }
.history li:first-child { border-top: none; padding-top: 0; }
.info { flex: 1; min-width: 0; }
.info.clickable { cursor: pointer; }
.info.clickable:hover .title { color: var(--accent); }
.title { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.meta { font-size: 0.82rem; }
.err { font-size: 0.82rem; color: var(--bad); }
.variant { font-size: 0.8rem; color: var(--accent); }
.side { display: flex; align-items: center; gap: 8px; flex-shrink: 0; }
.badge { font-size: 0.78rem; padding: 2px 10px; border-radius: 999px; background: var(--accent-soft); color: var(--accent); }
.badge.bad { background: var(--bad-soft); color: var(--bad); }
.score { font-weight: 800; font-size: 1.1rem; min-width: 3ch; text-align: right; }
.score.good { color: var(--ok); }
.score.mid { color: var(--accent); }
.score.low { color: var(--bad); }

@media (max-width: 560px) {
  .history li { flex-direction: column; align-items: stretch; }
  .side { justify-content: flex-end; }
  .label { min-width: 100%; }
}
</style>
