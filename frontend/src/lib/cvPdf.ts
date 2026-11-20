// Builds an ATS-friendly PDF of the CV: real selectable text (not an image),
// one column, standard font and plain section headings.
import type { jsPDF as JsPDF } from "jspdf";
import { parseCv, stripMarks, type Run } from "./cv";

const PAGE_W = 210;
const PAGE_H = 297;
const MARGIN = 18;
const WIDTH = PAGE_W - 2 * MARGIN;
const PT = 0.3528; // mm per point

// Characters the built-in Helvetica (WinAnsi) can draw besides Latin-1.
const WIN_ANSI = "€‚ƒ„…†‡ˆ‰Š‹ŒŽ‘’“”•–—˜™š›œžŸ";
const REPLACE: Record<string, string> = { "→": "->", "←": "<-", "✓": "-", "✔": "-", "▪": "•", "●": "•", "◦": "•", "·": "·" };

/** Keeps only characters the PDF font can draw. */
function clean(s: string) {
  let out = "";
  for (const ch of s) {
    if (REPLACE[ch]) out += REPLACE[ch];
    else if (ch.charCodeAt(0) < 256 || WIN_ANSI.includes(ch)) out += ch;
  }
  return out;
}

export async function cvToPdf(markdown: string): Promise<Blob> {
  const { jsPDF } = await import("jspdf");
  const doc: JsPDF = new jsPDF({ unit: "mm", format: "a4" });
  doc.setProperties({ title: "CV", creator: "ReqToQuiz" });
  let y = MARGIN;

  const ensure = (h: number) => {
    if (y + h > PAGE_H - MARGIN) {
      doc.addPage();
      y = MARGIN;
    }
  };

  /** Draws runs with word wrap starting at x; returns nothing, advances y. */
  const drawRuns = (runs: Run[], x: number, width: number, size: number, color = 20, forceBold = false) => {
    const lh = size * PT * 1.35;
    doc.setFontSize(size);
    doc.setTextColor(color);
    type Word = { text: string; bold: boolean; space: boolean };
    const words: Word[] = [];
    for (const r of runs) {
      const text = clean(r.text);
      const parts = text.split(/(\s+)/);
      for (const p of parts) {
        if (!p) continue;
        if (/^\s+$/.test(p)) {
          if (words.length) words[words.length - 1].space = true;
        } else words.push({ text: p, bold: r.bold || forceBold, space: false });
      }
    }
    const font = (b: boolean) => doc.setFont("helvetica", b ? "bold" : "normal");
    font(false);
    const spaceW = doc.getTextWidth(" ");

    // Lay out words into lines, then draw each same-style stretch of a line
    // with one text call so extractors (and ATS) read whole phrases.
    type Seg = { text: string; bold: boolean; x: number };
    const lines: Seg[][] = [[]];
    let cx = x;
    for (const w of words) {
      font(w.bold);
      const ww = doc.getTextWidth(w.text);
      let line = lines[lines.length - 1];
      if (line.length && cx + ww > x + width) {
        line = [];
        lines.push(line);
        cx = x;
      }
      const last = line[line.length - 1];
      if (last && last.bold === w.bold) last.text += (last.text.endsWith(" ") ? "" : " ") + w.text;
      else line.push({ text: w.text, bold: w.bold, x: cx });
      cx += ww;
      if (w.space) {
        cx += spaceW;
        line[line.length - 1].text += " ";
      }
    }
    for (const line of lines) {
      ensure(lh);
      for (const s of line) {
        font(s.bold);
        doc.text(s.text.trimEnd(), s.x, y + size * PT * 0.8);
      }
      y += lh;
    }
  };

  for (const b of parseCv(stripMarks(markdown))) {
    switch (b.type) {
      case "name":
        drawRuns(b.runs, MARGIN, WIDTH, 20, 10, true);
        y += 1;
        break;
      case "heading":
        y += 3;
        ensure(12);
        drawRuns(b.runs.map((r) => ({ ...r, text: r.text.toUpperCase() })), MARGIN, WIDTH, 11.5, 10, true);
        doc.setDrawColor(150);
        doc.setLineWidth(0.3);
        doc.line(MARGIN, y, PAGE_W - MARGIN, y);
        y += 2.5;
        break;
      case "subheading":
        y += 1;
        drawRuns(b.runs, MARGIN, WIDTH, 10.5, 20, true);
        break;
      case "bullet": {
        const size = 10;
        ensure(size * PT * 1.35);
        doc.setFont("helvetica", "normal");
        doc.setFontSize(size);
        doc.setTextColor(20);
        doc.text("•", MARGIN + 1, y + size * PT * 0.8);
        drawRuns(b.runs, MARGIN + 5, WIDTH - 5, size);
        y += 0.6;
        break;
      }
      case "line":
        drawRuns(b.runs, MARGIN, WIDTH, b.header ? 9.5 : 10, b.header ? 70 : 20);
        y += 0.6;
        break;
      case "gap":
        y += 1.5;
        break;
    }
  }
  return doc.output("blob");
}

export function download(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
