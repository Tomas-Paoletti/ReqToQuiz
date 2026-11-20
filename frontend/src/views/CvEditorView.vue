<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { getAtsReport, type AtsReport, type QuizMeta } from "../api";
import { applyChanges, parseCv, placeholders, stripMarks, wordDiff, type Change } from "../lib/cv";
import { cvToPdf, download } from "../lib/cvPdf";

const props = defineProps<{ id: string }>();

type Decision = "pending" | "accepted" | "rejected";

const meta = ref<QuizMeta | null>(null);
const report = ref<AtsReport | null>(null);
const loadError = ref("");
const decisions = ref<Decision[]>([]);
/** User edits of each suggestion's "after" text. */
const edits = ref<string[]>([]);
const editing = ref<number | null>(null);
const busy = ref(false);
const downloadError = ref("");

const STORE_KEY = computed(() => `reqtoquiz.cv.${props.id}`);

function restore(n: number) {
  try {
    const saved = JSON.parse(localStorage.getItem(STORE_KEY.value) ?? "null");
    if (saved && saved.decisions?.length === n && saved.edits?.length === n) {
      decisions.value = saved.decisions;
      edits.value = saved.edits;
      return;
    }
  } catch {
    /* storage unavailable or corrupt */
  }
  decisions.value = new Array(n).fill("pending");
  edits.value = report.value!.rewrites.map((r) => r.after);
}

watch(
  [decisions, edits],
  () => {
    try {
      localStorage.setItem(STORE_KEY.value, JSON.stringify({ decisions: decisions.value, edits: edits.value }));
    } catch {
      /* storage unavailable */
    }
  },
  { deep: true },
);

onMounted(async () => {
  try {
    const res = await getAtsReport(props.id);
    meta.value = res.meta;
    report.value = res.report;
    restore(res.report.rewrites.length);
  } catch (e) {
    loadError.value = (e as Error).message;
  }
});

const rewrites = computed(() => report.value?.rewrites ?? []);
const counts = computed(() => ({
  accepted: decisions.value.filter((d) => d === "accepted").length,
  rejected: decisions.value.filter((d) => d === "rejected").length,
  pending: decisions.value.filter((d) => d === "pending").length,
}));

const result = computed(() => {
  const cv = report.value?.cvMarkdown ?? "";
  const changes: (Change | null)[] = rewrites.value.map((r, i) =>
    decisions.value[i] === "accepted" ? { before: r.before, after: edits.value[i] } : null,
  );
  return applyChanges(cv, changes);
});
const finalText = computed(() => stripMarks(result.value.marked));
const preview = computed(() => parseCv(result.value.marked));
const pendingPlaceholders = computed(() => [...new Set(placeholders(finalText.value))]);

const diffs = computed(() => rewrites.value.map((r, i) => wordDiff(r.before, edits.value[i] ?? r.after)));

function decide(i: number, d: Decision) {
  decisions.value[i] = decisions.value[i] === d ? "pending" : d;
  if (editing.value === i && d === "rejected") editing.value = null;
}

function acceptAllRephrases() {
  rewrites.value.forEach((r, i) => {
    if (r.type === "rephrase" && decisions.value[i] === "pending") decisions.value[i] = "accepted";
  });
}

function reset() {
  decisions.value = decisions.value.map(() => "pending");
  edits.value = rewrites.value.map((r) => r.after);
  editing.value = null;
}

function baseName() {
  const src = meta.value?.cvSource ?? "cv";
  return src.replace(/\.[^.]+$/, "") + "-ats";
}

async function downloadPdf() {
  downloadError.value = "";
  busy.value = true;
  try {
    download(await cvToPdf(finalText.value), baseName() + ".pdf");
  } catch (e) {
    downloadError.value = "No se pudo generar el PDF: " + (e as Error).message;
  } finally {
    busy.value = false;
  }
}

function downloadMd() {
  download(new Blob([finalText.value], { type: "text/markdown;charset=utf-8" }), baseName() + ".md");
}
</script>

<template>
  <section class="card">
    <a class="back" :href="`/ats/${id}/`">← Volver al informe</a>
    <h2>Elegí qué cambios aplicar a tu CV</h2>
    <p v-if="meta && report" class="muted sub">
      {{ meta.cvSource }} · {{ report.title }} · puntaje actual {{ report.score }}/100
    </p>

    <p v-if="loadError" class="error-box">{{ loadError }}</p>
    <p v-else-if="report && !report.cvMarkdown" class="error-box">
      Este análisis es anterior al editor y no tiene el CV transcripto. Volvé a analizar el CV para poder elegir los
      cambios y descargarlo.
    </p>

    <template v-else-if="report">
      <p class="muted">
        Cada cambio muestra lo que se borra (<del>tachado</del>) y lo que se agrega (<ins>resaltado</ins>). Aceptá o
        rechazá cada uno, editá el texto si querés, y descargá el CV con los cambios aceptados.
      </p>

      <div class="toolbar">
        <span>
          <strong>{{ counts.accepted }}</strong> aceptados · {{ counts.rejected }} rechazados ·
          {{ counts.pending }} sin decidir
        </span>
        <span class="tools">
          <button class="small" @click="acceptAllRephrases">Aceptar todas las reformulaciones</button>
          <button class="small" @click="reset">Restablecer</button>
        </span>
      </div>

      <ol class="changes">
        <li
          v-for="(r, i) in rewrites"
          :key="i"
          :class="[decisions[i], { conflict: result.conflicts.includes(i) }]"
        >
          <div class="head">
            <strong>{{ r.section }}</strong>
            <span class="type" :class="r.type">
              {{ r.type === "add_keyword" ? "Agrega keyword" : "Reformula" }}
            </span>
          </div>

          <p class="diff">
            <template v-for="(p, k) in diffs[i]" :key="k">
              <del v-if="p.kind === 'del'">{{ p.text }}</del>
              <ins v-else-if="p.kind === 'ins'">{{ p.text }}</ins>
              <span v-else>{{ p.text }}</span>
              {{ " " }}
            </template>
          </p>

          <textarea v-if="editing === i" v-model="edits[i]" rows="3"></textarea>

          <p class="why">{{ r.why }}</p>
          <p v-if="r.type === 'add_keyword'" class="warn">Aceptalo solo si realmente tenés esa experiencia.</p>
          <p v-if="result.conflicts.includes(i)" class="warn">
            No se pudo aplicar: otro cambio aceptado ya modificó este texto.
          </p>

          <div class="actions">
            <button class="small accept" :class="{ on: decisions[i] === 'accepted' }" @click="decide(i, 'accepted')">
              ✓ Aceptar
            </button>
            <button class="small reject" :class="{ on: decisions[i] === 'rejected' }" @click="decide(i, 'rejected')">
              ✗ Rechazar
            </button>
            <button class="small" @click="editing = editing === i ? null : i">
              {{ editing === i ? "Listo" : "Editar texto" }}
            </button>
          </div>
        </li>
      </ol>
    </template>
  </section>

  <section v-if="report?.cvMarkdown" class="card">
    <div class="preview-head">
      <h2>Vista previa</h2>
      <span class="muted legend"><mark>resaltado</mark> = cambios aceptados</span>
    </div>
    <div class="cv">
      <template v-for="(b, i) in preview" :key="i">
        <div v-if="b.type === 'gap'" class="gap"></div>
        <component
          :is="b.type === 'name' ? 'h1' : b.type === 'heading' ? 'h3' : b.type === 'subheading' ? 'h4' : 'p'"
          v-else
          :class="[b.type, { header: b.type === 'line' && b.header }]"
        >
          <template v-for="(run, k) in b.runs" :key="k">
            <mark v-if="run.changed" :class="{ b: run.bold }">{{ run.text }}</mark>
            <strong v-else-if="run.bold">{{ run.text }}</strong>
            <template v-else>{{ run.text }}</template>
          </template>
        </component>
      </template>
    </div>

    <p v-if="pendingPlaceholders.length" class="error-box">
      Completá estos datos antes de descargar: <strong>{{ pendingPlaceholders.join(", ") }}</strong>. Tocá «Editar
      texto» en el cambio que los tiene y poné el valor real (o sacá esa parte).
    </p>
    <div class="download">
      <button class="primary" :disabled="busy || pendingPlaceholders.length > 0" @click="downloadPdf">
        {{ busy ? "Generando…" : "Descargar CV en PDF" }}
      </button>
      <button :disabled="pendingPlaceholders.length > 0" @click="downloadMd">Descargar .md</button>
    </div>
    <p class="muted note">
      El PDF es de una columna, con texto seleccionable y títulos estándar, el formato que mejor leen los ATS. Revisalo
      antes de mandarlo.
    </p>
    <p v-if="downloadError" class="error-box">{{ downloadError }}</p>
  </section>
</template>

<style scoped>
.back { font-size: 0.9rem; text-decoration: none; }
h2 { margin-top: 8px; }
.sub { margin: -8px 0 12px; }
del { background: var(--bad-soft); color: var(--bad); text-decoration: line-through; }
ins { background: var(--ok-soft); color: var(--ok); text-decoration: none; font-weight: 600; }
.toolbar {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  padding: 10px 0;
  margin: 8px 0;
  border-top: 1px solid var(--border);
  border-bottom: 1px solid var(--border);
}
.tools { display: flex; gap: 8px; flex-wrap: wrap; }
.changes { list-style: none; margin: 0; padding: 0; display: grid; gap: 12px; }
.changes li { border: 1px solid var(--border); border-left: 4px solid var(--border); border-radius: 8px; padding: 12px 14px; }
.changes li.accepted { border-left-color: var(--ok); background: color-mix(in srgb, var(--ok-soft) 50%, transparent); }
.changes li.rejected { border-left-color: var(--bad); opacity: 0.6; }
.changes li.conflict { border-left-color: #ca8a04; }
.head { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.type { font-size: 0.75rem; font-weight: 700; padding: 1px 8px; border-radius: 999px; background: var(--accent-soft); color: var(--accent); }
.type.add_keyword { background: #fdf6e3; color: #a16207; }
.diff { margin: 8px 0; line-height: 1.7; }
.diff del, .diff ins { padding: 1px 3px; border-radius: 3px; }
.why { margin: 0; font-size: 0.88rem; color: var(--muted); }
.warn { margin: 4px 0 0; font-size: 0.85rem; color: #a16207; font-weight: 600; }
textarea {
  width: 100%;
  font: inherit;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 8px;
  margin-bottom: 6px;
}
.actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
.accept.on { background: var(--ok); border-color: var(--ok); color: #fff; }
.reject.on { background: var(--bad); border-color: var(--bad); color: #fff; }
.preview-head { display: flex; justify-content: space-between; align-items: baseline; gap: 8px; flex-wrap: wrap; }
.legend { font-size: 0.85rem; }
mark { background: var(--ok-soft); color: inherit; border-radius: 3px; padding: 0 2px; }
mark.b { font-weight: 700; }
.cv {
  background: #fff;
  color: #1a1a1a;
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 28px 32px;
  font-family: Helvetica, Arial, sans-serif;
  font-size: 0.92rem;
  line-height: 1.4;
}
.cv mark { background: #d9f5e1; }
.cv h1 { font-size: 1.6rem; margin: 0 0 4px; }
.cv h3 { font-size: 1rem; text-transform: uppercase; border-bottom: 1px solid #999; padding-bottom: 2px; margin: 16px 0 6px; }
.cv h4 { font-size: 0.95rem; margin: 8px 0 2px; }
.cv p { margin: 2px 0; }
.cv p.header { color: #555; font-size: 0.85rem; }
.cv p.bullet { padding-left: 16px; position: relative; }
.cv p.bullet::before { content: "•"; position: absolute; left: 4px; }
.cv .gap { height: 6px; }
.download { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 16px; }
.note { font-size: 0.82rem; margin: 8px 0 0; }
@media (max-width: 560px) {
  .cv { padding: 16px; }
}
</style>
