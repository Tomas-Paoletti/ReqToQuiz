// CV editing helpers: apply accepted rewrites to the CV Markdown, word diffs
// for the review screen and a tiny parser for the Markdown subset the
// ats-check skill writes (# name, ## sections, **bold**, - bullets, lines).

/** Marks around inserted text, so the preview can highlight changes. */
export const MARK_START = "\u0001";
export const MARK_END = "\u0002";

const escapeRe = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** Regex that finds `text` ignoring whitespace differences (like the backend). */
function looseRe(text: string) {
  const words = text.trim().split(/\s+/).map(escapeRe);
  return new RegExp(words.join("\\s+"));
}

/** Whether position i falls inside a MARK_START…MARK_END region. */
function insideMark(text: string, i: number) {
  return text.lastIndexOf(MARK_START, i) > text.lastIndexOf(MARK_END, i);
}

export interface Change {
  before: string;
  after: string;
}

export interface Applied {
  /** CV with the changes, inserted text wrapped in MARK_START/MARK_END. */
  marked: string;
  /** Indexes of changes whose `before` was not found (overlapping edits). */
  conflicts: number[];
}

/** Applies changes in order; each replaces the first match of its `before`. */
export function applyChanges(cv: string, changes: (Change | null)[]): Applied {
  let text = cv;
  const conflicts: number[] = [];
  changes.forEach((c, i) => {
    if (!c) return;
    const re = new RegExp(looseRe(c.before).source, "g");
    let m: RegExpExecArray | null;
    // Skip matches inside (or across) text inserted by an earlier change.
    while ((m = re.exec(text)) && (insideMark(text, m.index) || /[\u0001\u0002]/.test(m[0]))) {
      re.lastIndex = m.index + 1;
    }
    if (!m) {
      conflicts.push(i);
      return;
    }
    text = text.slice(0, m.index) + MARK_START + c.after.trim() + MARK_END + text.slice(m.index + m[0].length);
  });
  return { marked: text, conflicts };
}

export const stripMarks = (s: string) => s.replace(/[\u0001\u0002]/g, "");

/** Unfilled placeholders like [X%] or [N] left by the suggestions. */
export const placeholders = (s: string) => s.match(/\[[^\]\n]{1,12}\]/g) ?? [];

// --- word diff ---------------------------------------------------------------

export interface DiffPart {
  text: string;
  kind: "same" | "del" | "ins";
}

/** Word-level diff (LCS) between two short texts. */
export function wordDiff(a: string, b: string): DiffPart[] {
  const x = a.trim().split(/\s+/);
  const y = b.trim().split(/\s+/);
  const n = x.length;
  const m = y.length;
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--)
    for (let j = m - 1; j >= 0; j--)
      dp[i][j] = x[i] === y[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);

  const out: DiffPart[] = [];
  const push = (text: string, kind: DiffPart["kind"]) => {
    const last = out[out.length - 1];
    if (last && last.kind === kind) last.text += " " + text;
    else out.push({ text, kind });
  };
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (x[i] === y[j]) {
      push(x[i], "same");
      i++;
      j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) push(x[i++], "del");
    else push(y[j++], "ins");
  }
  while (i < n) push(x[i++], "del");
  while (j < m) push(y[j++], "ins");
  return out;
}

// --- Markdown subset -----------------------------------------------------------

export interface Run {
  text: string;
  bold: boolean;
  changed: boolean;
}

export type Block =
  | { type: "name"; runs: Run[] }
  | { type: "heading"; runs: Run[] }
  | { type: "subheading"; runs: Run[] }
  | { type: "bullet"; runs: Run[] }
  | { type: "line"; runs: Run[]; header: boolean }
  | { type: "gap" };

/** Splits a line into runs by **bold** and change marks. */
export function parseInline(line: string, changedAtStart: boolean): { runs: Run[]; changed: boolean } {
  const runs: Run[] = [];
  let bold = false;
  let changed = changedAtStart;
  let buf = "";
  const flush = () => {
    if (buf) runs.push({ text: buf, bold, changed });
    buf = "";
  };
  for (let i = 0; i < line.length; i++) {
    const c = line[i];
    if (c === "*" && line[i + 1] === "*") {
      flush();
      bold = !bold;
      i++;
    } else if (c === MARK_START || c === MARK_END) {
      flush();
      changed = c === MARK_START;
    } else buf += c;
  }
  flush();
  return { runs, changed };
}

/** Parses the CV Markdown into blocks; lines before the first ## are the header. */
export function parseCv(md: string): Block[] {
  const blocks: Block[] = [];
  let changed = false;
  let inHeader = true;
  for (const raw of md.replace(/\r\n?/g, "\n").split("\n")) {
    const line = raw.trim();
    // Marks may sit at the start of a line; keep tracking them across lines.
    const bare = stripMarks(line);
    if (!bare) {
      ({ changed } = parseInline(line, changed));
      if (blocks.length && blocks[blocks.length - 1].type !== "gap") blocks.push({ type: "gap" });
      continue;
    }
    let type: Block["type"] = "line";
    let content = line;
    const prefix = (p: RegExp) => {
      // Remove the Markdown marker even when a change mark precedes it.
      const m = line.match(new RegExp(`^([\\u0001\\u0002]*)${p.source}`));
      if (!m) return false;
      content = m[1] + line.slice(m[0].length);
      return true;
    };
    if (prefix(/#\s+/)) type = "name";
    else if (prefix(/##\s+/)) type = "heading";
    else if (prefix(/###+\s+/)) type = "subheading";
    else if (prefix(/[-*•]\s+/)) type = "bullet";
    if (type === "heading") inHeader = false;
    const parsed = parseInline(content, changed);
    changed = parsed.changed;
    if (type === "line") blocks.push({ type, runs: parsed.runs, header: inHeader });
    else blocks.push({ type, runs: parsed.runs } as Block);
  }
  return blocks;
}
