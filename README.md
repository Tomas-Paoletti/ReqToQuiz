# ReqToQuiz

Generá un quiz multiple choice para practicar una entrevista a partir del **requerimiento de un puesto**.

Subís el aviso (PDF, Markdown o texto pegado), elegís cuántas preguntas querés (5, 10, 20, 30 o 40), el idioma y la dificultad, y **Claude Code** genera las preguntas por detrás usando una skill propia del repo. Cuando termina, la app te lleva al quiz: lo rendís, ves tu **porcentaje**, revisás las explicaciones y podés **imprimir las respuestas**. Cada quiz queda guardado en un historial para volver a rendirlo.

También tiene un **filtro ATS**: subís el aviso y **tu CV**, y te dice qué puntaje le pondría un ATS (el software que filtra postulaciones), qué palabras clave te faltan, qué requisitos excluyentes no cumplís, qué partes del formato el ATS no puede leer y cómo reescribir tus bullets, sin inventar nada que no esté en tu CV.

Es una app **local**: corre en tu máquina, usa tu sesión de Claude Code y no necesita base de datos ni servidor externo. Si Claude Code no está instalado o falla, usa **Codex** como respaldo automático.

---

## Índice

- [Qué hace](#qué-hace)
- [Requisitos](#requisitos)
- [Instalación y puesta en marcha](#instalación-y-puesta-en-marcha)
- [Uso](#uso)
- [Cómo funciona](#cómo-funciona)
- [Motores: elección, sesión y respaldo](#motores-elección-sesión-y-respaldo)
- [Arquitectura](#arquitectura)
- [Estructura del repo](#estructura-del-repo)
- [La skill `job-quiz`](#la-skill-job-quiz)
- [Filtro ATS: la skill `ats-check`](#filtro-ats-la-skill-ats-check)
- [API del backend](#api-del-backend)
- [Datos guardados](#datos-guardados)
- [Desarrollo](#desarrollo)
- [Solución de problemas](#solución-de-problemas)

---

## Qué hace

1. **Entrada**: un requerimiento de trabajo como archivo `.pdf`, `.md` o `.txt` (máx. 20 MB), o texto pegado.
2. **Opciones**:
   - Cantidad de preguntas: 5, 10, 20, 30 o 40.
   - Idioma: español o inglés (aunque el aviso esté en otro idioma).
   - Dificultad: junior, semi senior, senior o mixta.
3. **Generación**: Claude analiza el puesto (rol, seniority, tecnologías, responsabilidades) y reparte las preguntas entre los temas según su peso en el aviso. Cada pregunta tiene 4 opciones, una sola correcta y una explicación.
4. **Progreso en vivo**: mientras se genera, ves cada paso ("Leyendo el requerimiento…", "Escribiendo pregunta 7 de 20…").
5. **Quiz**: al terminar te redirige a la página del quiz, donde:
   - respondés las preguntas, con una barra de cuántas llevás;
   - al tocar **Corregir** ves el porcentaje, las correctas en verde, las incorrectas en rojo y la explicación de cada una;
   - **Imprimir respuestas** genera una versión para imprimir o guardar como PDF con tu respuesta, la correcta y la explicación;
   - **Reintentar** vuelve a empezar con las opciones mezcladas de nuevo.
6. **Historial**: la pantalla de inicio lista todos los quizzes generados con su último puntaje. Desde ahí los abrís o los borrás.
7. **Elegir motor y respaldo con Codex**: si tenés Claude Code y Codex, elegís cuál usar o dejás **Automático** (Claude primero y, si no tiene sesión o falla, Codex). La app comprueba si cada uno tiene **sesión iniciada**, lo muestra en pantalla y, si falta, te dice el comando para iniciarla. El historial indica con qué motor se generó cada quiz.
8. **Sin preguntas repetidas**: si subís un requerimiento que ya usaste (el mismo PDF, o el mismo texto aunque cambien las mayúsculas o los espacios), el quiz nuevo **no repite los conceptos** de los anteriores. En el historial aparece como "Variante 2", "Variante 3", etc.

---

## Requisitos

| Herramienta | Versión | Para qué |
|---|---|---|
| [Claude Code](https://claude.com/claude-code) | reciente | Motor principal. Tiene que tener la **sesión iniciada**. |
| [Codex CLI](https://github.com/openai/codex) | reciente | **Opcional**: motor de respaldo. También con sesión iniciada. |
| [Go](https://go.dev/dl/) | 1.24 o más | Backend. |
| [Node.js](https://nodejs.org/) | 20 o más | Compilar el frontend y correr los scripts. |

Alcanza con tener **uno** de los dos motores. Con los dos, Codex queda de respaldo.

Para comprobar que está todo instalado:

```bash
claude --version
codex --version    # opcional
go version
node -v
```

Si nunca usaste Claude Code, ejecutá `claude` una vez en una terminal e iniciá sesión. Para Codex: `npm install -g @openai/codex` y después `codex login`. La app usa esas sesiones (tus suscripciones) para generar los quizzes.

Al arrancar, el servidor muestra en la consola qué motores encontró:

```
motor disponible: Claude Code (C:\Users\...\npm\claude)
motor no disponible: Codex (no se encontró "codex" en el PATH)
sin motor de respaldo: solo se usará Claude Code
```

---

## Instalación y puesta en marcha

```bash
git clone <url-del-repo> ReqToQuiz
cd ReqToQuiz

npm run setup     # instala las dependencias de la raíz y del frontend
npm run build     # compila el frontend y genera reqtoquiz.exe con todo incluido
npm start         # levanta la app en http://localhost:8080 y abre el navegador
```

Después de compilar, también podés ejecutar el binario directamente **desde la raíz del repo**:

```bash
./reqtoquiz.exe
```

### Opciones del binario

| Flag | Default | Descripción |
|---|---|---|
| `-port` | `8080` | Puerto HTTP. Solo escucha en `127.0.0.1`. |
| `-engine` | `auto` | `auto` (Claude Code y, si falla, Codex), `claude` (solo Claude) o `codex` (solo Codex). |
| `-model` | (el de Claude Code) | Modelo de Claude Code, por ejemplo `sonnet` (más rápido) u `opus`. |
| `-codex-model` | (el de Codex) | Modelo de Codex. |
| `-claude` / `-codex` | `claude` / `codex` | Ruta o nombre del ejecutable de cada motor, si no está en el PATH. |
| `-open` | `false` | Abre el navegador al iniciar (`npm start` lo activa). |
| `-root` | se detecta sola | Ruta de la raíz del repo, donde está `.claude/skills/job-quiz`. |

El binario necesita estar dentro del repo (o recibir `-root`) porque Claude Code se ejecuta desde la raíz para encontrar la skill, y los quizzes se guardan en `data/`.

---

## Uso

1. Abrí `http://localhost:8080`.
2. En **Nuevo quiz**, arrastrá el archivo del aviso o pasá a la pestaña **Pegar texto**.
3. Elegí cantidad, idioma y dificultad, y tocá **Generar quiz**.
4. Esperá en la pantalla de progreso: suele tardar entre 30 segundos y 3 minutos según la cantidad de preguntas.
5. Respondé y tocá **Corregir**. Desde los resultados podés **Imprimir respuestas**, **Reintentar** o **Volver al inicio**.

Para probar rápido, usá el aviso de ejemplo `examples/backend-go-developer.md` con 5 preguntas.

---

## Cómo funciona

```
1. Vue  ── POST /api/quizzes (archivo + opciones) ──►  Go
2. Go   guarda el archivo en data/quizzes/<id>/ y responde el id al instante
3. Vue  abre /generating/<id> y escucha GET /api/quizzes/<id>/events (SSE)
4. Go   ejecuta en segundo plano:  claude -p  (desde la raíz del repo)
          └─ Claude usa la skill job-quiz → lee el aviso → escribe questions.json
5. Go   lee la salida de Claude en vivo y la convierte en eventos de progreso
6. Go   valida questions.json (si está mal, le pide a Claude que lo corrija una vez)
7. Go   mezcla las opciones y arma quiz.html con una plantilla fija
8. Vue  recibe el evento "done" y redirige a /quizzes/<id>/
9. Al corregir, el quiz manda el puntaje a POST /api/quizzes/<id>/attempts
```

### El comando que ejecuta el backend

```bash
claude -p \
  --output-format stream-json --verbose --include-partial-messages \
  --tools Read,Write,Skill --allowedTools Read,Write,Skill \
  --permission-mode acceptEdits --strict-mcp-config
```

El prompt se pasa por stdin e indica la ruta del archivo de entrada, la cantidad, el idioma, la dificultad y la ruta de salida.

- **`--tools Read,Write,Skill`**: Claude solo puede leer y escribir archivos y cargar la skill. No puede ejecutar comandos ni acceder a la web.
- **`--strict-mcp-config`**: no carga servidores MCP, así arranca más rápido.
- **`--include-partial-messages`**: permite ver el JSON mientras se escribe y contar cuántas preguntas lleva ("Escribiendo pregunta 3 de 10…").

### Por qué Claude devuelve JSON y no HTML

Claude escribe solo los datos (`questions.json`) y el backend arma la página con una plantilla fija. Así:

- el diseño es el mismo en todos los quizzes;
- el puntaje lo calcula código, no el modelo;
- el JSON se puede validar y corregir automáticamente;
- se gastan menos tokens.

Además, el backend **mezcla las opciones** de cada pregunta para que la respuesta correcta no quede siempre en la misma posición.

## Motores: elección, sesión y respaldo

### Elegir el motor

Si hay **más de un motor instalado**, el formulario muestra un selector **Motor**:

| Opción | Qué hace |
|---|---|
| **Automático** (por defecto) | Usa Claude Code; si no tiene sesión o falla, pasa a Codex. |
| **Claude Code** | Usa solo Claude Code. Si falla, muestra el error, **sin** pasar a Codex. |
| **Codex** | Usa solo Codex, sin respaldo. |

La elección se recuerda en el navegador. Las opciones sin sesión iniciada aparecen deshabilitadas.

### Comprobación de sesión

Debajo del formulario hay un panel **Motores** con el estado de cada uno:

- 🟢 **Sesión iniciada**, con el detalle (por ejemplo `claude.ai · plan pro`).
- 🔴 **Sin sesión iniciada**, con el comando para iniciarla (`claude auth login` / `codex login`).
- 🟡 **No se pudo comprobar**: el comando de estado falló, así que se intenta igual.
- ⚪ **No instalado**.

El botón **Volver a comprobar** actualiza el estado, por ejemplo después de iniciar sesión en una terminal.

Cómo se comprueba:

| Motor | Comando | Sesión iniciada si… |
|---|---|---|
| Claude Code | `claude auth status` | devuelve JSON con `"loggedIn": true` |
| Codex | `codex login status` | termina con código 0, o hay `OPENAI_API_KEY` / `CODEX_API_KEY` en el entorno |

Se comprueba en estos momentos:

1. **Al arrancar**: la consola muestra `sesión Claude Code: iniciada (…)` o `sesión Codex: NO iniciada, ejecutá codex login`.
2. **Al cargar la página** (`/api/health`, con 20 s de caché).
3. **Al crear el quiz**: si el motor elegido no tiene sesión, el pedido se rechaza al instante con el comando para iniciarla, y no se crea ningún quiz.
4. **Justo antes de generar**, sin caché. En automático, los motores sin sesión se **omiten** ("Claude Code no tiene sesión iniciada… Se omite.") y se pasa al siguiente.

### Respaldo con Codex

El backend tiene una interfaz `Engine` con dos implementaciones (`backend/internal/runner/claude.go` y `codex.go`). En modo automático las prueba **en orden**:

```
Claude Code ──falla──► Codex ──falla──► error con los motivos de los dos
     │                   │
   éxito               éxito
     ▼                   ▼
          quiz listo (meta.engine = "claude" | "codex")
```

**Cuándo pasa a Codex** (solo en modo automático): cuando Claude Code no está instalado o no tiene sesión (se omite sin ejecutarlo), o cuando una generación con Claude falla por cualquier motivo: límite de uso, modelo inválido, error de red, más de 10 minutos, o JSON inválido incluso después del reintento. La pantalla de progreso lo avisa: *"Claude Code no pudo generar el quiz: (motivo). Probando con Codex…"*.

**Cómo genera Codex**: ejecuta

```bash
codex exec --json --sandbox read-only --skip-git-repo-check --ephemeral \
  --cd <repo> --output-schema codex-schema.json --output-last-message questions.json -
```

- Lee **la misma skill** (`.claude/skills/job-quiz/SKILL.md` y `schema.md`), así las preguntas siguen las mismas reglas. También recibe `avoid.md`, así que tampoco repite preguntas.
- Corre en **solo lectura**: no escribe archivos. Su respuesta final está restringida por un JSON Schema (`backend/internal/runner/codex_schema.json`) y Codex la guarda en `questions.json`.
- **PDF**: Codex no lee PDFs, así que el backend extrae el texto antes (`backend/internal/pdftext`) y le pasa `input-extracted.txt`. Si el PDF es un escaneo sin texto, Codex no puede usarlo y lo informa.
- Después se valida igual que con Claude: formato, cantidad y repetidas, con un reintento si algo está mal.


### Cómo evita repetir preguntas

1. **Detecta el mismo requerimiento**: al subirlo se guarda una huella SHA-256 (`inputHash`). Los textos (`.md`, `.txt` o pegados) se normalizan antes (minúsculas, espacios, saltos de línea, BOM), así que un `.md` subido y el mismo texto pegado coinciden. Los PDF se comparan byte a byte. Un PDF y el mismo texto pegado **no** se reconocen como iguales.
2. **Le pasa a Claude lo ya preguntado**: junta las preguntas de todos los quizzes anteriores (listos) con la misma huella y escribe `avoid.md` con cada **concepto** evaluado. La skill recibe `avoid:`, elige conceptos distintos y antes de escribir revisa que ninguna pregunta evalúe lo mismo con otro escenario.
3. **Lo verifica en el backend**: compara cada pregunta nueva con las anteriores y con las del mismo quiz, por redacción (palabras clave con raíz simple) y por concepto (el campo `concept`). Si encuentra repeticiones, le pide a Claude que reemplace esas preguntas puntuales.

La comparación del backend es una red de seguridad que agarra copias, reformulaciones y conceptos con nombre parecido. Lo que evita los conceptos repetidos con otro escenario es el punto 2.

---

## Arquitectura

```
Navegador
  ├─ App Vue (inicio, progreso)  ──HTTP / SSE──┐
  └─ quiz.html (página del quiz) ──HTTP────────┤
                                               ▼
                  Backend Go  (reqtoquiz.exe, 127.0.0.1:8080)
                  ├─ api     endpoints REST, SSE y frontend embebido
                  ├─ runner  ejecuta `claude -p` y traduce su salida a progreso
                  ├─ quiz    valida, mezcla opciones y arma quiz.html
                  └─ store   carpetas en data/quizzes/<id>/
                                               │ proceso aparte
                                               ▼
                  Claude Code headless + skill job-quiz
                  (solo Read, Write y Skill)
```

| Capa | Tecnología | Responsabilidad |
|---|---|---|
| Frontend | Vue 3, Vite, TypeScript, vue-router | Formulario, pantalla de progreso e historial. Se compila y queda **embebido** en el binario de Go. |
| Backend | Go, solo librería estándar | API, ejecución de Claude Code, validación, armado del quiz y almacenamiento. |
| IA | Claude Code en modo headless (`claude -p`) | Leer el aviso y escribir las preguntas siguiendo la skill. |
| Quiz | HTML + CSS + JavaScript sin frameworks | Página independiente: corrige, muestra el porcentaje, imprime y reintenta. Funciona incluso abierta como archivo. |
| Datos | Archivos JSON en `data/` | Historial sin base de datos. |

Comportamientos del backend:

- **Un quiz por vez**: si generás otro mientras uno está en curso, queda en cola.
- **Límite de 10 minutos** por generación. Si se corta, se termina el proceso de Claude y sus hijos.
- **Reintento automático**: si `questions.json` no pasa la validación, se retoma la misma sesión de Claude (`--resume`) pidiendo la corrección.
- **Recuperación**: si cerrás el servidor en medio de una generación, al volver a iniciarlo ese quiz queda marcado con error.

---

## Estructura del repo

```
ReqToQuiz/
├── .claude/skills/job-quiz/
│   ├── SKILL.md              instrucciones de la skill que genera las preguntas
│   └── schema.md             formato exacto de questions.json
├── .claude/skills/ats-check/
│   ├── SKILL.md              instrucciones del filtro ATS (CV vs puesto)
│   └── schema.md             formato exacto de ats.json
├── backend/                  servidor Go
│   ├── main.go               flags, detección del repo y de `claude`, arranque
│   ├── internal/
│   │   ├── api/              endpoints HTTP y SSE
│   │   ├── runner/           ejecución de Claude Code y eventos de progreso
│   │   ├── quiz/             validación, mezcla y armado del quiz
│   │   │   ├── quiz.html.tmpl    plantilla de la página del quiz
│   │   │   └── quiz_test.go      tests
│   │   ├── ats/              validación, puntaje y armado del informe ATS
│   │   │   └── report.html.tmpl  plantilla del informe
│   │   └── store/            persistencia en data/quizzes/
│   └── web/                  frontend compilado (embebido con go:embed)
├── frontend/                 app Vue
│   ├── src/views/HomeView.vue        formulario + historial
│   ├── src/views/GeneratingView.vue  progreso en vivo
│   ├── src/api.ts                    cliente de la API
│   └── vite.config.ts                build hacia backend/web/dist y proxy en dev
├── examples/                 aviso y CV ficticio de ejemplo para probar
├── data/                     quizzes generados (ignorado por git)
└── package.json              scripts: setup, dev, build, start, test
```

---

## La skill `job-quiz`

Está en [`.claude/skills/job-quiz/SKILL.md`](.claude/skills/job-quiz/SKILL.md) y es la que define **qué preguntas** se generan. Recibe:

| Parámetro | Valores |
|---|---|
| `input` | ruta al aviso (`.pdf`, `.md`, `.txt`) |
| `count` | 5, 10, 20, 30 o 40 |
| `language` | `es` o `en` |
| `difficulty` | `junior`, `semi`, `senior` o `mixed` |
| `output` | ruta donde escribir `questions.json` |
| `avoid` (opcional) | ruta a `avoid.md` con los conceptos ya evaluados para este requerimiento |

Reglas de calidad principales: 4 opciones con una sola correcta; distractores plausibles (sin "todas las anteriores"); preguntas sobre entender y aplicar, no trivia; reparto proporcional entre los temas del aviso; y una explicación de 1 a 3 oraciones en cada pregunta.

El formato de salida está en [`schema.md`](.claude/skills/job-quiz/schema.md):

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
      "question": "¿Qué ocurre si…?",
      "options": ["…", "…", "…", "…"],
      "answer": 0,
      "explanation": "…"
    }
  ]
}
```

**Para mejorar las preguntas, editá `SKILL.md`**: no hace falta tocar código. Si cambiás el formato en `schema.md`, actualizá también la validación en `backend/internal/quiz/quiz.go`.

También podés usar la skill sin la app, desde Claude Code en este repo:

```
/job-quiz input: examples/backend-go-developer.md count: 5 language: es difficulty: mixed output: data/test/questions.json
```

---

## Filtro ATS: la skill `ats-check`

En la pantalla principal elegí **Filtro ATS**, subí (o pegá) el requerimiento y subí tu CV (`.pdf`, `.md` o `.txt`). Al terminar te lleva al informe en `/ats/<id>/`, que se puede imprimir.

**Qué revisa**, simulando un ATS (Workday, Greenhouse, Lever…) y al recruiter que mira su ranking:

| Sección | Qué te dice |
|---|---|
| Puntaje 0-100 y veredicto | *Pasaría el filtro* (75+), *En el límite* (55-74) o *Probablemente filtrado*. Lo calcula el backend con pesos fijos: palabras clave 40 %, experiencia 30 %, formación 15 %, formato 15 %. |
| Requisitos excluyentes | Años de experiencia, título, idioma, ubicación…: *Cumple*, *No queda claro* o *No cumple*. Si alguno no se cumple, el veredicto no puede ser "pasaría", porque muchos ATS descartan antes de mirar el puntaje. |
| Palabras clave | Cada skill o tecnología del aviso, obligatoria o deseable, con *Está*, *Parcial* o *Falta*, la cita de tu CV donde aparece y qué hacer. |
| Dónde puede fallar | Problemas concretos ordenados por prioridad, cada uno con su arreglo. |
| Puntos fuertes y reescrituras | Bullets de tu CV reescritos con las palabras del aviso. Solo reformulan lo que ya está: si falta un número, ponen `[X%]` para que lo completes. |

**Elegir los cambios y descargar el CV.** Desde el informe, *Elegir cambios y descargar CV* abre `/cv/<id>`:

- Cada sugerencia se ve como un diff palabra por palabra (lo que se borra tachado, lo que se agrega resaltado) con **Aceptar**, **Rechazar** y **Editar texto**. Las de tipo *Agrega keyword* avisan que las aceptes solo si es verdad.
- Abajo hay una vista previa del CV con los cambios aceptados resaltados.
- **Descargar CV en PDF** genera un PDF de una columna con texto real (no una imagen), que es lo que mejor leen los ATS. También se puede bajar en `.md`.
- Si quedó algún dato por completar, como `[X%]`, la descarga se bloquea hasta que lo edites.
- Tus decisiones se guardan en el navegador, así que podés volver más tarde.

Para que esto funcione, la skill transcribe el CV completo a Markdown (`cvMarkdown`) y cada sugerencia cita texto exacto de esa transcripción. El backend lo verifica y, si una cita no aparece, le pide al motor que la corrija.

**El formato también cuenta.** Si el CV es un PDF, antes de llamar al motor el backend extrae el texto con un extractor común (`cv-parsed.txt`), como haría un ATS. La skill compara ese texto con el PDF y detecta lo que se pierde: columnas mezcladas, tablas, íconos en vez de datos de contacto, texto dentro de imágenes. Si no se puede extraer nada (un PDF escaneado), lo marca como problema grave.

**Privacidad:** el CV se guarda solo en `data/quizzes/<id>/cv.*`, que está en `.gitignore`, y solo lo lee el motor que elijas (Claude Code o Codex, con tu sesión).

Se puede usar sin la app, desde Claude Code en este repo:

```
/ats-check job: examples/backend-go-developer.md cv: examples/cv-ejemplo.md language: es output: data/test/ats.json
```

El formato de `ats.json` está en [`schema.md`](.claude/skills/ats-check/schema.md) y la validación en `backend/internal/ats/ats.go`.

---

## API del backend

| Método | Ruta | Descripción |
|---|---|---|
| `GET` | `/api/health` | Estado de cada motor: `{ "engines": [{ "id": "claude", "name": "Claude Code", "installed": true, "auth": { "state": "logged_in", "detail": "claude.ai · plan pro", "loginCommand": "claude auth login" } }, { "id": "codex", "installed": false, … }] }`. `state` puede ser `logged_in`, `logged_out` o `unknown`. Con `?refresh=1` no usa la caché. |
| `GET` | `/api/quizzes` | Historial, del más nuevo al más viejo. |
| `POST` | `/api/quizzes` | Crea un quiz. Multipart: `file` **o** `text`, `count`, `language`, `difficulty` y `engine` opcional (`auto`, `claude` o `codex`; por defecto `auto`). Responde el `meta` con el `id`. Devuelve **400** si el motor elegido no está instalado o no tiene sesión. |
| `GET` | `/api/quizzes/{id}` | Datos de un quiz. |
| `DELETE` | `/api/quizzes/{id}` | Borra un quiz (no se puede mientras se genera). |
| `GET` | `/api/quizzes/{id}/events` | Progreso por Server-Sent Events: `progress`, `done` (con `url`) o `error`. |
| `POST` | `/api/ats` | Crea un análisis ATS. Multipart: `file` **o** `text` (el puesto), `cv` (archivo), `language` y `engine` opcional. Aparece en el mismo historial con `"kind": "ats"` y, cuando termina, `"ats": { "score": 66, "verdict": "borderline" }`. |
| `POST` | `/api/quizzes/{id}/attempts` | Guarda un intento: `{ "score": 80, "correct": 4, "total": 5 }`. |
| `GET` | `/quizzes/{id}/` | Página del quiz. |
| `GET` | `/ats/{id}/` | Informe ATS. |
| `GET` | `/api/ats/{id}/report` | Informe ATS en JSON (con `cvMarkdown` y `rewrites`), lo usa el editor del CV. |
| `GET` | `/*` | Frontend Vue. |

Ejemplo con curl:

```bash
curl -X POST localhost:8080/api/quizzes \
  -F file=@examples/backend-go-developer.md \
  -F count=5 -F language=es -F difficulty=mixed

curl -N localhost:8080/api/quizzes/<id>/events
```

---

## Datos guardados

Cada quiz vive en `data/quizzes/<id>/`, con un id del tipo `20261117-112145-0b230f`:

| Archivo | Contenido |
|---|---|
| `input.pdf` / `input.md` | el requerimiento original |
| `meta.json` | título, rol, opciones, estado (`generating`, `ready` o `error`), fecha, huella del requerimiento (`inputHash`), cuántos quizzes anteriores se excluyeron (`previousQuizzes`), último intento y cantidad de intentos |
| `avoid.md` | solo si el requerimiento ya se había usado: los conceptos que no se podían repetir |
| `input-extracted.txt`, `codex-schema.json` | solo si intervino Codex: el texto extraído del PDF y el schema de su respuesta |
| `questions.json` | lo que escribió Claude |
| `quiz.html` | la página del quiz, que funciona sola y se puede abrir sin el servidor |
| `cv.pdf` / `cv.md`, `cv-parsed.txt` | solo en análisis ATS: tu CV y el texto que extrae un ATS de él |
| `ats.json`, `report.html` | solo en análisis ATS: lo que escribió el motor y el informe |

`data/` está en `.gitignore`. Para borrar todo el historial, eliminá esa carpeta.

---

## Desarrollo

```bash
npm run dev       # backend Go en :8080 + Vite en :5173 con recarga en caliente
npm test          # tests del backend (validación, mezcla y armado del quiz)
npm --prefix frontend run typecheck
```

En desarrollo abrí **http://localhost:5173**: Vite redirige `/api` y `/quizzes` al backend en `:8080`.

| Quiero cambiar… | Archivo |
|---|---|
| Qué preguntas se generan | `.claude/skills/job-quiz/SKILL.md` |
| Diseño o comportamiento de la página del quiz | `backend/internal/quiz/quiz.html.tmpl` |
| Qué revisa el filtro ATS | `.claude/skills/ats-check/SKILL.md` |
| Pesos del puntaje ATS y diseño del informe | `backend/internal/ats/ats.go`, `report.html.tmpl` |
| Validaciones del JSON | `backend/internal/quiz/quiz.go` |
| Qué cuenta como pregunta repetida | `backend/internal/quiz/similar.go` (umbrales y stopwords) |
| Cómo se reconoce el mismo requerimiento | `HashInput` en `backend/internal/store/store.go` |
| Orden de motores y lógica de respaldo | `backend/internal/runner/runner.go` |
| Cómo se invoca a Claude Code / Codex | `backend/internal/runner/claude.go` / `codex.go` |
| Formulario e historial | `frontend/src/views/HomeView.vue` |
| Pantalla de progreso | `frontend/src/views/GeneratingView.vue` |

Los cambios en la plantilla del quiz se aplican a los quizzes **nuevos**; los ya generados conservan su `quiz.html`.

---

## Solución de problemas

| Problema | Causa y solución |
|---|---|
| `no se encontró ningún motor` | Instalá Claude Code o Codex y abrí una terminal nueva. |
| "Claude Code no pudo generar el quiz… Probando con Codex" | Claude falló (sesión, límite de uso, etc.). El motivo aparece en el mensaje; mientras tanto el quiz se genera con Codex. |
| "Codex no tiene sesión iniciada" / "Claude Code no tiene sesión iniciada" | Ejecutá `codex login` / `claude auth login` y tocá **Volver a comprobar** en el panel Motores. |
| El selector de motor no aparece | Solo aparece si hay más de un motor instalado. Con uno solo se usa ese. |
| Una opción del selector está tachada | Ese motor no tiene sesión iniciada. |
| "Codex necesita el texto del PDF" | El PDF es un escaneo o una imagen sin texto. Usá Claude Code (lee PDFs escaneados) o pegá el texto. |
| La generación falla enseguida con un error de autenticación | Ejecutá `claude` en una terminal e iniciá sesión (o `codex login`). |
| `no se encontró la raíz del repo` | Ejecutá el binario desde la carpeta del repo o pasá `-root <ruta>`. |
| La página dice "El frontend no está compilado" | Ejecutá `npm run build`, o en desarrollo usá `http://localhost:5173`. |
| `no se pudo escuchar en 127.0.0.1:8080` | El puerto está ocupado: usá `-port 8081` (en desarrollo, cambiá también `backend` en `frontend/vite.config.ts`). |
| Tarda mucho | Más preguntas tardan más. Probá con `-model sonnet` o con menos preguntas. |
| `questions.json inválido` | Claude no respetó el formato ni siquiera después del reintento. Volvé a generar. Si se repite, revisá las reglas de `SKILL.md`. |
| Un quiz quedó con error "la generación se interrumpió" | Se cerró el servidor mientras se generaba. Borralo y generalo de nuevo. |
| Error "preguntas repetidas" | Claude no logró evitar conceptos ya preguntados ni con el reintento. Pasa cuando el aviso es corto y ya se generaron muchos quizzes con él: probá con menos preguntas, o borrá quizzes viejos de ese requerimiento para liberar conceptos. |
| `go build` falla o el binario no se actualiza | En Windows no se puede sobrescribir `reqtoquiz.exe` mientras está corriendo: cerralo antes de `npm run build`. |
