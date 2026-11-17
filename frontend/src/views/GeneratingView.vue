<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { engineLabel, getQuiz, type EngineId, type ProgressEvent, type QuizMeta } from "../api";

const props = defineProps<{ id: string }>();

const steps = [
  { key: "start", label: "Iniciando el generador" },
  { key: "skill", label: "Cargando la skill" },
  { key: "read", label: "Leyendo el requerimiento" },
  { key: "generate", label: "Generando las preguntas" },
  { key: "write", label: "Guardando el quiz" },
  { key: "validate", label: "Validando" },
];

const meta = ref<QuizMeta | null>(null);
const current = ref("start");
const message = ref("Conectando…");
const error = ref("");
const done = ref(false);
const engine = ref<EngineId | null>(null);
const elapsed = ref(0);
let source: EventSource | null = null;
let timer: number | undefined;

const currentIndex = computed(() => steps.findIndex((s) => s.key === current.value));

function stepState(i: number) {
  if (done.value || i < currentIndex.value) return "done";
  if (i === currentIndex.value) return error.value ? "failed" : "active";
  return "pending";
}

function handle(e: ProgressEvent) {
  if (e.engine) engine.value = e.engine;
  if (e.step && steps.some((s) => s.key === e.step)) current.value = e.step;
  if (e.message) message.value = e.message;
  if (e.type === "done" && e.url) {
    done.value = true;
    message.value = "¡Quiz listo! Abriendo…";
    source?.close();
    window.clearInterval(timer);
    window.location.href = e.url;
  }
  if (e.type === "error") {
    error.value = e.message || "Error desconocido";
    source?.close();
    window.clearInterval(timer);
  }
}

onMounted(() => {
  getQuiz(props.id).then((m) => (meta.value = m)).catch(() => {});
  const start = Date.now();
  timer = window.setInterval(() => (elapsed.value = Math.floor((Date.now() - start) / 1000)), 1000);

  source = new EventSource(`/api/quizzes/${props.id}/events`);
  for (const type of ["progress", "done", "error"]) {
    source.addEventListener(type, (ev) => {
      // A connection error also fires "error" but without data; EventSource retries by itself.
      const data = (ev as MessageEvent).data;
      if (data) handle(JSON.parse(data));
    });
  }
});

onBeforeUnmount(() => {
  source?.close();
  window.clearInterval(timer);
});

const elapsedLabel = computed(() => {
  const m = Math.floor(elapsed.value / 60);
  const s = elapsed.value % 60;
  return m > 0 ? `${m}m ${String(s).padStart(2, "0")}s` : `${s}s`;
});
</script>

<template>
  <section class="card">
    <h2>{{ error ? "No se pudo generar el quiz" : "Generando tu quiz…" }}</h2>
    <p v-if="meta" class="muted sub">
      {{ meta.source }} · {{ meta.count }} preguntas
      <template v-if="engine"> · con {{ engineLabel[engine] }}</template>
    </p>

    <ol class="steps">
      <li v-for="(s, i) in steps" :key="s.key" :class="stepState(i)">
        <span class="icon">
          <span v-if="stepState(i) === 'done'">✓</span>
          <span v-else-if="stepState(i) === 'failed'">✗</span>
          <span v-else-if="stepState(i) === 'active'" class="spinner"></span>
        </span>
        {{ s.label }}
      </li>
    </ol>

    <div v-if="!error" class="status">
      <span>{{ message }}</span>
      <span class="muted">{{ elapsedLabel }}</span>
    </div>
    <p v-if="!error" class="muted hint">Suele tardar entre 30 segundos y 3 minutos según la cantidad de preguntas.</p>

    <template v-if="error">
      <p class="error-box">{{ error }}</p>
      <RouterLink to="/"><button class="primary back">Volver al inicio</button></RouterLink>
    </template>
  </section>
</template>

<style scoped>
.sub { margin: -8px 0 16px; }
.steps { list-style: none; padding: 0; margin: 0 0 20px; display: grid; gap: 10px; }
.steps li { display: flex; align-items: center; gap: 12px; color: var(--muted); }
.steps li.active { color: var(--text); font-weight: 600; }
.steps li.done { color: var(--ok); }
.steps li.failed { color: var(--bad); font-weight: 600; }
.icon {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  border: 2px solid var(--border);
  display: grid;
  place-items: center;
  font-size: 0.8rem;
  flex-shrink: 0;
}
.done .icon { border-color: var(--ok); background: var(--ok-soft); }
.failed .icon { border-color: var(--bad); background: var(--bad-soft); }
.active .icon { border-color: var(--accent); }
.spinner {
  width: 12px;
  height: 12px;
  border: 2px solid var(--accent);
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }
.status {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  background: var(--accent-soft);
  border-radius: 8px;
}
.hint { font-size: 0.85rem; margin: 10px 0 0; }
.back { margin-top: 16px; }
@media (prefers-reduced-motion: reduce) { .spinner { animation-duration: 3s; } }
</style>
