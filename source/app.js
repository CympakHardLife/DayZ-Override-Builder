"use strict";

// ---------- state ----------
let analysis = null;
let registry = [];
let regMap = {};
// edits: itemId -> { sel:bool, params:{key:value}, customs:[{path,type,value}] }
const edits = new Map();
const MAX_ROWS = 1500;

const $ = (id) => document.getElementById(id);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

// ---------- tabs ----------
function showTab(name) {
  document.querySelectorAll(".tab").forEach((t) => t.classList.remove("active"));
  document.querySelectorAll(".step").forEach((s) => s.classList.remove("active"));
  $("tab-" + name).classList.add("active");
  document.querySelector(`.step[data-tab="${name}"]`).classList.add("active");
  setGlobalLogPolling(name === "log");
}
document.querySelectorAll(".step").forEach((btn) => {
  btn.addEventListener("click", () => { if (!btn.disabled) showTab(btn.dataset.tab); });
});

// ==================== ВКЛАДКА: GUI МОДА ====================
let guiSid = null, guiFiles = [], guiCur = null, guiCsvRows = null, guiReadonly = false;

const kindIcons = { text: "📝", image: "🖼", sound: "🔊", config: "⚙", binary: "📦" };

$("guiOpenBtn").addEventListener("click", async () => {
  const f = $("guiFile").files[0];
  const p = $("guiPath").value.trim();
  if (!f && !p) { setStatus("guiStatus", "Выберите .pbo файл или укажите путь", false); return; }
  $("guiOpenBtn").disabled = true;
  try {
    let data;
    if (f) {
      const fd = new FormData();
      fd.append("pbo", f, f.name);
      data = await xhrUpload("/api/res/open", fd, null);
    } else {
      const r = await fetch("/api/res/open", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: p }),
      });
      data = await r.json();
      if (!r.ok) throw new Error(data.error || "ошибка");
    }
    guiSid = data.sid;
    guiFiles = data.files || [];
    guiCur = null;
    $("guiModName").textContent = data.name + " (" + guiFiles.length + " файлов, prefix: " + data.prefix + ")";
    renderGuiTree();
    $("guiWork").classList.remove("hidden");
    $("guiBuildCard").classList.remove("hidden");
    setStatus("guiStatus", "PBO открыт. Выбирайте файлы слева, правьте, затем соберите изменённый PBO внизу.", true);
  } catch (e) {
    setStatus("guiStatus", "Ошибка: " + e.message, false);
  } finally {
    $("guiOpenBtn").disabled = false;
  }
});

$("guiKindFilter").addEventListener("input", renderGuiTree);

function renderGuiTree() {
  const filter = $("guiKindFilter").value;
  const box = $("guiTree");
  box.innerHTML = "";
  guiFiles.filter((f) => !filter || f.kind === filter).forEach((f) => {
    const div = document.createElement("div");
    div.className = "gui-file" + (f.mod ? " modded" : "") + (guiCur && guiCur.path === f.path ? " active" : "");
    div.innerHTML = `<span class="gf-icon">${kindIcons[f.kind] || "📦"}</span>
      <span class="gf-name">${esc(f.path)}</span>
      <span class="gf-size">${(f.size / 1024).toFixed(1)} КБ</span>`;
    div.addEventListener("click", () => openGuiFile(f));
    box.appendChild(div);
  });
}

function hideGuiEditors() {
  ["guiEditorText", "guiEditorCsv", "guiEditorImg", "guiEditorSound", "guiEditorBin"].forEach((id) => $(id).classList.add("hidden"));
}

async function openGuiFile(f) {
  guiCur = f;
  renderGuiTree();
  $("guiCurTitle").textContent = f.path;
  hideGuiEditors();
  const isCsv = f.path.toLowerCase().endsWith(".csv");
  try {
    if (f.kind === "text" || f.kind === "config") {
      const r = await fetch(`/api/res/text?sid=${guiSid}&path=${encodeURIComponent(f.path)}`);
      const d = await r.json();
      if (!r.ok) throw new Error(d.error || "ошибка чтения");
      guiReadonly = !!d.readonly;
      if (isCsv && !guiReadonly) {
        guiCsvRows = parseCsv(d.content);
        renderCsvTable();
        $("guiEditorCsv").classList.remove("hidden");
      } else {
        $("guiTextArea").value = d.content;
        $("guiTextArea").readOnly = guiReadonly;
        $("guiSaveTextBtn").disabled = guiReadonly;
        $("guiTextNote").textContent = guiReadonly ? "только чтение (config.bin)" : "";
        renderLayoutColors();
        $("guiEditorText").classList.remove("hidden");
      }
    } else if (f.kind === "image") {
      const img = $("guiImgPreview");
      img.onerror = () => { img.alt = "превью недоступно для этого формата — но заменить файл можно"; };
      img.src = `/api/res/img?sid=${guiSid}&path=${encodeURIComponent(f.path)}&t=${Date.now()}`;
      $("guiEditorImg").classList.remove("hidden");
    } else if (f.kind === "sound") {
      $("guiAudio").src = `/api/res/raw?sid=${guiSid}&path=${encodeURIComponent(f.path)}&t=${Date.now()}`;
      $("guiEditorSound").classList.remove("hidden");
    } else {
      $("guiEditorBin").classList.remove("hidden");
    }
  } catch (e) {
    setStatus("guiStatus", "Ошибка: " + e.message, false);
  }
}

// ---------- цвета в .layout ----------
const layoutColorRe = /^(\s*(?:"[^"]*color[^"]*"|color)\s+)([\d.]+)\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)\s*$/i;

function renderLayoutColors() {
  const box = $("guiColors");
  box.innerHTML = "";
  if (!guiCur || !guiCur.path.toLowerCase().endsWith(".layout")) return;
  const lines = $("guiTextArea").value.split("\n");
  let shown = 0;
  lines.forEach((line, idx) => {
    const m = line.match(layoutColorRe);
    if (!m || shown >= 60) return;
    shown++;
    const [r, g, b, a] = [m[2], m[3], m[4], m[5]].map(Number);
    const hex = "#" + [r, g, b].map((v) => Math.round(Math.min(1, v) * 255).toString(16).padStart(2, "0")).join("");
    const chip = document.createElement("label");
    chip.className = "gui-color-chip";
    chip.title = "строка " + (idx + 1) + ": " + line.trim();
    chip.innerHTML = `<span class="sw" style="--swcolor:rgba(${Math.round(r * 255)},${Math.round(g * 255)},${Math.round(b * 255)},${a})"></span>
      строка ${idx + 1}
      <input type="color" value="${hex}" style="width:0;height:0;opacity:0;position:absolute">
      <input type="number" class="alpha" value="${a}" min="0" max="1" step="0.05" style="width:56px" title="прозрачность (альфа)">`;
    const colorInput = chip.querySelector('input[type="color"]');
    const alphaInput = chip.querySelector(".alpha");
    chip.querySelector(".sw").addEventListener("click", (e) => { e.preventDefault(); colorInput.click(); });
    const apply = () => {
      const hx = colorInput.value;
      const nr = (parseInt(hx.slice(1, 3), 16) / 255).toFixed(4);
      const ng = (parseInt(hx.slice(3, 5), 16) / 255).toFixed(4);
      const nb = (parseInt(hx.slice(5, 7), 16) / 255).toFixed(4);
      const na = Math.max(0, Math.min(1, parseFloat(alphaInput.value) || 0));
      const cur = $("guiTextArea").value.split("\n");
      const mm = cur[idx] && cur[idx].match(layoutColorRe);
      if (mm) {
        cur[idx] = mm[1] + [nr, ng, nb, na].map((x) => String(parseFloat(x))).join(" ");
        $("guiTextArea").value = cur.join("\n");
        renderLayoutColors();
      }
    };
    colorInput.addEventListener("input", apply);
    alphaInput.addEventListener("change", apply);
    box.appendChild(chip);
  });
  if (shown > 0) {
    const hint = document.createElement("span");
    hint.className = "muted";
    hint.style.alignSelf = "center";
    hint.textContent = "— цвета из лайоута, кликните по квадратику";
    box.appendChild(hint);
  }
}

$("guiSaveTextBtn").addEventListener("click", async () => {
  if (!guiCur) return;
  await saveGuiText(guiCur.path, $("guiTextArea").value);
});

async function saveGuiText(path, content) {
  const r = await fetch("/api/res/text", {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sid: guiSid, path, content }),
  });
  const d = await r.json();
  if (!r.ok) { setStatus("guiStatus", "Ошибка сохранения: " + (d.error || ""), false); return; }
  const f = guiFiles.find((x) => x.path === path);
  if (f) f.mod = true;
  renderGuiTree();
  setStatus("guiStatus", "Правка сохранена: " + path + " (не забудьте собрать PBO внизу)", true);
}

// ---------- CSV (stringtable) ----------
function parseCsv(text) {
  const rows = [];
  let row = [], field = "", inQ = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (inQ) {
      if (c === '"') {
        if (text[i + 1] === '"') { field += '"'; i++; }
        else inQ = false;
      } else field += c;
    } else if (c === '"') inQ = true;
    else if (c === ",") { row.push(field); field = ""; }
    else if (c === "\n") {
      if (field !== "" || row.length) { row.push(field); rows.push(row); }
      row = []; field = "";
    } else if (c !== "\r") field += c;
  }
  if (field !== "" || row.length) { row.push(field); rows.push(row); }
  // убираем пустое поле от завершающей запятой
  return rows.map((r) => (r.length && r[r.length - 1] === "" ? r.slice(0, -1) : r));
}

function serializeCsv(rows) {
  return rows.map((r) => r.map((f) => '"' + String(f).replace(/"/g, '""') + '"').join(",") + ",").join("\n") + "\n";
}

function renderCsvTable() {
  const t = $("guiCsvTable");
  t.innerHTML = "";
  if (!guiCsvRows || !guiCsvRows.length) return;
  const head = document.createElement("tr");
  guiCsvRows[0].forEach((h) => {
    const th = document.createElement("th");
    th.textContent = h;
    head.appendChild(th);
  });
  const thead = document.createElement("thead");
  thead.appendChild(head);
  t.appendChild(thead);
  const tbody = document.createElement("tbody");
  for (let ri = 1; ri < guiCsvRows.length; ri++) {
    const tr = document.createElement("tr");
    for (let ci = 0; ci < guiCsvRows[0].length; ci++) {
      const td = document.createElement("td");
      td.textContent = guiCsvRows[ri][ci] || "";
      td.contentEditable = "true";
      td.addEventListener("input", () => {
        while (guiCsvRows[ri].length <= ci) guiCsvRows[ri].push("");
        guiCsvRows[ri][ci] = td.textContent;
      });
      tr.appendChild(td);
    }
    tbody.appendChild(tr);
  }
  t.appendChild(tbody);
}

$("guiSaveCsvBtn").addEventListener("click", async () => {
  if (!guiCur || !guiCsvRows) return;
  await saveGuiText(guiCur.path, serializeCsv(guiCsvRows));
});

// ---------- замена файлов ----------
async function guiReplace(inputEl) {
  const file = inputEl.files[0];
  if (!file || !guiCur) return;
  const fd = new FormData();
  fd.append("sid", guiSid);
  fd.append("path", guiCur.path);
  fd.append("file", file, file.name);
  const r = await fetch("/api/res/replace", { method: "POST", body: fd });
  const d = await r.json();
  inputEl.value = "";
  if (!r.ok) { setStatus("guiStatus", "Ошибка замены: " + (d.error || ""), false); return; }
  const f = guiFiles.find((x) => x.path === guiCur.path);
  if (f) f.mod = true;
  renderGuiTree();
  setStatus("guiStatus", "Файл заменён: " + guiCur.path + (d.note || "") + " — соберите PBO внизу", true);
  openGuiFile(guiCur); // обновить превью
}
$("guiImgReplace").addEventListener("change", () => guiReplace($("guiImgReplace")));
$("guiSoundReplace").addEventListener("change", () => guiReplace($("guiSoundReplace")));
$("guiBinReplace").addEventListener("change", () => guiReplace($("guiBinReplace")));

// ---------- сборка ----------
async function guiBuild(outputPath) {
  if (!guiSid) return;
  const log = $("guiBuildLog");
  log.textContent = "";
  log.classList.remove("hidden");
  try {
    const r = await fetch("/api/res/build", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        sid: guiSid,
        sign: $("guiSign").checked,
        toolsPath: $("toolsPath").value.trim(),
        privateKeyPath: $("keyPath").value.trim(),
        outputPath: outputPath || "",
      }),
    });
    const d = await r.json();
    if (!r.ok) throw new Error(d.error || "ошибка");
    const res = await pollJob(d.jobId, log);
    let msg = "Собрано: " + (res.files || []).join(", ") + " (правок: " + res.modCount + ")";
    if (res.savedTo) msg += ". Сохранено в: " + res.savedTo;
    if (res.saveError) msg += ". ОШИБКА сохранения: " + res.saveError;
    setStatus("guiBuildStatus", msg, !res.saveError);
    if (!outputPath && res.zipToken) window.location.href = "/api/zip/" + res.zipToken;
  } catch (e) {
    setStatus("guiBuildStatus", "Ошибка: " + e.message, false);
  }
}
$("guiBuildBtn").addEventListener("click", () => guiBuild(""));
$("guiBuildSaveBtn").addEventListener("click", () => {
  const p = $("guiOutPath").value.trim();
  if (!p) { setStatus("guiBuildStatus", "Укажите папку для сохранения", false); return; }
  guiBuild(p);
});

// ==================== ВКЛАДКА: EXPANSION КВЕСТЫ ====================
let expqData = null, expqMergeData = null;

function renderProblems(el, problems) {
  if (!problems || !problems.length) {
    el.innerHTML = `<p class="st-ok" style="margin-top:10px">✔ Проблем не найдено</p>`;
    return;
  }
  const errs = problems.filter((p) => p.level === "error");
  const warns = problems.filter((p) => p.level !== "error");
  let h = "";
  if (errs.length) {
    h += `<details open style="margin-top:10px"><summary class="link-sum" style="color:var(--err)">Ошибки (${errs.length})</summary><ul>` +
      errs.map((p) => `<li class="st-err">${esc(p.msg)}</li>`).join("") + "</ul></details>";
  }
  if (warns.length) {
    h += `<details style="margin-top:8px"><summary class="link-sum" style="color:var(--warn)">Предупреждения (${warns.length})</summary><ul>` +
      warns.map((p) => `<li style="color:var(--warn)">${esc(p.msg)}</li>`).join("") + "</ul></details>";
  }
  el.innerHTML = h;
}

$("expqCheckBtn").addEventListener("click", async () => {
  const zipF = $("expqPack").files[0];
  const path = $("expqPath").value.trim();
  if (!zipF && !path) { setStatus("expqStatus", "Прикрепите ZIP папки Quests или укажите путь", false); return; }
  const fd = new FormData();
  if (zipF) fd.append("pack", zipF, zipF.name);
  if (path) fd.append("packPath", path);
  if ($("expqLoadouts").files[0]) fd.append("loadouts", $("expqLoadouts").files[0]);
  if ($("expqTypes").files[0]) fd.append("typesFile", $("expqTypes").files[0]);
  $("expqCheckBtn").disabled = true;
  $("expqResult").classList.add("hidden");
  try {
    expqData = await runMultipartJob("/api/expq/check", fd, $("expqLog"));
    const s = expqData.stats || {};
    chips($("expqChips"), [
      [`квестов: ${s.quests || 0}`, "ok"],
      [`объективов: ${s.objectives || 0}`, "ok"],
      [`NPC: ${s.npcs || 0}`, "ok"],
      [`цепочек: ${s.chains || 0}`, ""],
      [`ошибок: ${s.errors || 0}`, s.errors ? "miss" : "ok"],
      [`предупреждений: ${s.warnings || 0}`, ""],
    ]);
    renderProblems($("expqProblems"), expqData.problems);
    renderChains($("expqChains"), expqData.chains);
    $("expqResult").classList.remove("hidden");
    $("expqReportBtn").classList.remove("hidden");
    setStatus("expqStatus", s.errors ? `Найдено ошибок: ${s.errors} — смотрите список ниже` : "Проверка пройдена — критических ошибок нет", !s.errors);
  } catch (e) {
    setStatus("expqStatus", "Ошибка: " + e.message, false);
  } finally {
    $("expqCheckBtn").disabled = false;
  }
});

$("expqReportBtn").addEventListener("click", () => {
  if (!expqData) return;
  let t = "ОТЧЁТ ПРОВЕРКИ КВЕСТОВ EXPANSION\n" + "=".repeat(60) + "\n\n";
  const s = expqData.stats || {};
  t += `Квестов: ${s.quests}, объективов: ${s.objectives}, NPC: ${s.npcs}, цепочек: ${s.chains}\nОшибок: ${s.errors}, предупреждений: ${s.warnings}\n\n`;
  (expqData.problems || []).forEach((p) => { t += (p.level === "error" ? "[ОШИБКА] " : "[предупр.] ") + p.msg + "\n"; });
  t += "\nЦЕПОЧКИ КВЕСТОВ:\n";
  (expqData.chains || []).forEach((ch) => {
    t += "  " + ch.map((n) => `${n.id} «${n.title}»${n.broken ? " [ОБРЫВ]" : ""}`).join(" -> ") + "\n";
  });
  if ((expqData.requiredLoadouts || []).length) t += "\nЛодауты, требуемые NPC: " + expqData.requiredLoadouts.join(", ") + "\n";
  if ((expqData.unknownClasses || []).length) t += "\nКласснеймы вне types.xml:\n  " + expqData.unknownClasses.join("\n  ") + "\n";
  downloadText("otchet_kvestov.txt", t);
});

function renderChains(el, chains) {
  if (!chains || !chains.length) { el.innerHTML = ""; return; }
  const multi = chains.filter((c) => c.length > 1);
  const single = chains.filter((c) => c.length === 1);
  const badge = (n) => {
    let b = "";
    if (n.daily) b += ` <span class="tag">дейлик</span>`;
    if (n.weekly) b += ` <span class="tag">недельный</span>`;
    if (n.repeat && !n.daily && !n.weekly) b += ` <span class="tag">повторяемый</span>`;
    if (n.pre && n.pre.length) b += ` <span class="tag" title="требуются выполненные квесты">требует: ${n.pre.join(",")}</span>`;
    return b;
  };
  const node = (n, last) =>
    `<span class="${n.broken ? "st-err" : ""}"><b>${n.id}</b> ${esc(n.title)}${badge(n)}${n.broken ? " ⚠ ОБРЫВ" : ""}</span>` + (last ? "" : ` <span class="muted">→</span> `);
  let h = `<details open style="margin-top:12px"><summary class="link-sum">Цепочки квестов (${multi.length})</summary><div style="margin-top:8px">`;
  multi.forEach((c) => {
    h += `<div class="pbo-row" style="display:block">` + c.map((n, i) => node(n, i === c.length - 1)).join("") + `</div>`;
  });
  h += `</div></details>`;
  if (single.length) {
    h += `<details style="margin-top:8px"><summary class="link-sum">Одиночные квесты (${single.length})</summary><div style="margin-top:8px">`;
    single.forEach((c) => { h += `<div class="pbo-row" style="display:block">` + node(c[0], true) + `</div>`; });
    h += `</div></details>`;
  }
  el.innerHTML = h;
}

async function expqMerge(outputPath) {
  const aF = $("expqPackA").files[0], aP = $("expqPathA").value.trim();
  const bF = $("expqPackB").files[0], bP = $("expqPathB").value.trim();
  if ((!aF && !aP) || (!bF && !bP)) { setStatus("expqMergeStatus", "Укажите ОБА пака: ваш и новый (ZIP или путь)", false); return; }
  const fd = new FormData();
  if (aF) fd.append("packA", aF, aF.name);
  if (aP) fd.append("packAPath", aP);
  if (bF) fd.append("packB", bF, bF.name);
  if (bP) fd.append("packBPath", bP);
  if (outputPath) fd.append("outputPath", outputPath);
  $("expqMergeBtn").disabled = true;
  $("expqMergeSaveBtn").disabled = true;
  try {
    expqMergeData = await runMultipartJob("/api/expq/merge", fd, $("expqMergeLog"));
    const s = expqMergeData.stats || {};
    let h = `<div class="tools-status" style="margin-top:10px">` +
      [`файлов: ${s.files}`, `перенумеровано: ${s.renames}`, `идентичных пропущено: ${s.skipped}`]
        .map((x) => `<span class="tool-chip ok">${x}</span>`).join("") + `</div>`;
    const rn = expqMergeData.renames || [];
    if (rn.length) {
      h += `<details open style="margin-top:8px"><summary class="link-sum">Перенумерация (${rn.length})</summary><ul>` +
        rn.map((r) => `<li>${r.kind === "quest" ? "квест" : r.kind === "npc" ? "NPC" : "объектив (тип " + (r.otype || "?") + ")"}: <b>${r.old} → ${r.new}</b> ${esc(r.title || "")}</li>`).join("") + "</ul></details>";
    }
    $("expqMergeResult").innerHTML = h;
    let msg = "Слияние завершено";
    if (expqMergeData.savedTo) msg += ". Сохранено в: " + expqMergeData.savedTo;
    if (expqMergeData.saveError) msg += ". ОШИБКА сохранения: " + expqMergeData.saveError;
    setStatus("expqMergeStatus", msg, !expqMergeData.saveError);
    if (!outputPath && expqMergeData.zipToken) window.location.href = "/api/zip/" + expqMergeData.zipToken;
  } catch (e) {
    setStatus("expqMergeStatus", "Ошибка: " + e.message, false);
  } finally {
    $("expqMergeBtn").disabled = false;
    $("expqMergeSaveBtn").disabled = false;
  }
}
$("expqMergeBtn").addEventListener("click", () => expqMerge(""));
$("expqMergeSaveBtn").addEventListener("click", () => {
  const p = $("expqOutPath").value.trim();
  if (!p) { setStatus("expqMergeStatus", "Укажите папку для сохранения", false); return; }
  expqMerge(p);
});

// ==================== ВКЛАДКА: EXPANSION ЛОДАУТЫ ====================
$("explCheckBtn").addEventListener("click", async () => {
  const zipF = $("explPack").files[0];
  const path = $("explPath").value.trim();
  if (!zipF && !path) { setStatus("explStatus", "Прикрепите ZIP папки Loadouts или укажите путь", false); return; }
  const fd = new FormData();
  if (zipF) fd.append("pack", zipF, zipF.name);
  if (path) fd.append("packPath", path);
  if ($("explTypes").files[0]) fd.append("typesFile", $("explTypes").files[0]);
  $("explCheckBtn").disabled = true;
  $("explResult").classList.add("hidden");
  try {
    const d = await runMultipartJob("/api/expl/check", fd, $("explLog"));
    const s = d.stats || {};
    chips($("explChips"), [
      [`файлов: ${s.files || 0}`, "ok"],
      [`проблем: ${s.problems || 0}`, s.problems ? "miss" : "ok"],
      [`класснеймов: ${s.classes || 0}`, ""],
      s.unknown !== undefined ? [`вне types.xml: ${s.unknown}`, s.unknown ? "miss" : "ok"] : ["", ""],
    ].filter((x) => x[0]));
    let extra = "";
    if ((d.unknownClasses || []).length) {
      extra = `<details style="margin-top:8px"><summary class="link-sum">Класснеймы вне types.xml (${d.unknownClasses.length})</summary><p class="muted">${d.unknownClasses.map(esc).join(", ")}</p></details>`;
    }
    renderProblems($("explProblems"), d.problems);
    $("explProblems").innerHTML += extra;
    $("explResult").classList.remove("hidden");
    setStatus("explStatus", s.problems ? `Найдено проблем: ${s.problems}` : "Все лодауты корректны", !s.problems);
  } catch (e) {
    setStatus("explStatus", "Ошибка: " + e.message, false);
  } finally {
    $("explCheckBtn").disabled = false;
  }
});

async function explMerge(outputPath) {
  const aF = $("explPackA").files[0], aP = $("explPathA").value.trim();
  const bF = $("explPackB").files[0], bP = $("explPathB").value.trim();
  if ((!aF && !aP) || (!bF && !bP)) { setStatus("explMergeStatus", "Укажите ОБЕ папки лодаутов: вашу и новую", false); return; }
  const fd = new FormData();
  if (aF) fd.append("packA", aF, aF.name);
  if (aP) fd.append("packAPath", aP);
  if (bF) fd.append("packB", bF, bF.name);
  if (bP) fd.append("packBPath", bP);
  if (outputPath) fd.append("outputPath", outputPath);
  $("explMergeBtn").disabled = true;
  $("explMergeSaveBtn").disabled = true;
  try {
    const d = await runMultipartJob("/api/expl/merge", fd, $("explMergeLog"));
    const s = d.stats || {};
    $("explMergeResult").innerHTML = `<div class="tools-status" style="margin-top:10px">` +
      [`файлов: ${s.files}`, `конфликтов: ${s.conflicts}`, `идентичных пропущено: ${s.skipped}`]
        .map((x) => `<span class="tool-chip ok">${x}</span>`).join("") + `</div>` +
      `<pre class="joblog" style="margin-top:8px">${esc(d.report || "")}</pre>`;
    let msg = "Слияние завершено";
    if (d.savedTo) msg += ". Сохранено в: " + d.savedTo;
    if (d.saveError) msg += ". ОШИБКА: " + d.saveError;
    setStatus("explMergeStatus", msg, !d.saveError);
    if (!outputPath && d.zipToken) window.location.href = "/api/zip/" + d.zipToken;
  } catch (e) {
    setStatus("explMergeStatus", "Ошибка: " + e.message, false);
  } finally {
    $("explMergeBtn").disabled = false;
    $("explMergeSaveBtn").disabled = false;
  }
}
$("explMergeBtn").addEventListener("click", () => explMerge(""));
$("explMergeSaveBtn").addEventListener("click", () => {
  const p = $("explOutPath").value.trim();
  if (!p) { setStatus("explMergeStatus", "Укажите папку для сохранения", false); return; }
  explMerge(p);
});

// ==================== ВКЛАДКА: СПАВН-ЛУТ ====================
$("spwCheckBtn").addEventListener("click", async () => {
  if (!$("spwSpawn").files[0]) { setStatus("spwStatus", "Прикрепите cfgspawnabletypes.xml", false); return; }
  const fd = new FormData();
  fd.append("spawnFile", $("spwSpawn").files[0]);
  if ($("spwPresets").files[0]) fd.append("presetsFile", $("spwPresets").files[0]);
  if ($("spwTypes").files[0]) fd.append("typesFile", $("spwTypes").files[0]);
  $("spwCheckBtn").disabled = true;
  $("spwResult").classList.add("hidden");
  try {
    const d = await runMultipartJob("/api/spawn/check", fd, $("spwLog"));
    const s = d.stats || {};
    chips($("spwChips"), [
      [`типов: ${s.types || 0}`, "ok"],
      [`cargo-пресетов: ${s.cargoPresets || 0}`, ""],
      [`attach-пресетов: ${s.attachPresets || 0}`, ""],
      [`ошибок: ${s.errors || 0}`, s.errors ? "miss" : "ok"],
      [`предупреждений: ${s.warnings || 0}`, ""],
    ]);
    renderProblems($("spwProblems"), d.problems);
    let extra = "";
    if ((d.unusedPresets || []).length) {
      extra += `<details style="margin-top:8px"><summary class="link-sum">Неиспользуемые пресеты (${d.unusedPresets.length})</summary><p class="muted">${d.unusedPresets.map(esc).join(", ")}</p></details>`;
    }
    if ((d.missingTypes || []).length) {
      extra += `<details style="margin-top:8px"><summary class="link-sum">Типы вне types.xml (${d.missingTypes.length})</summary><p class="muted">${d.missingTypes.map(esc).join(", ")}</p></details>`;
    }
    if ((d.unknownItems || []).length) {
      extra += `<details style="margin-top:8px"><summary class="link-sum">Вложенные предметы вне types.xml (${d.unknownItems.length}) — не всегда ошибка</summary><p class="muted">${d.unknownItems.map(esc).join(", ")}</p></details>`;
    }
    $("spwProblems").innerHTML += extra;
    $("spwResult").classList.remove("hidden");
    setStatus("spwStatus", s.errors ? `Найдено ошибок: ${s.errors}` : "Критических ошибок нет", !s.errors);
  } catch (e) {
    setStatus("spwStatus", "Ошибка: " + e.message, false);
  } finally {
    $("spwCheckBtn").disabled = false;
  }
});

async function spwMerge(outputPath) {
  if (!$("spwSpawnA").files[0] || !$("spwSpawnB").files[0]) {
    setStatus("spwMergeStatus", "Нужны оба cfgspawnabletypes.xml: ваш и новый", false);
    return;
  }
  const fd = new FormData();
  fd.append("spawnA", $("spwSpawnA").files[0]);
  fd.append("spawnB", $("spwSpawnB").files[0]);
  if ($("spwPresetsA").files[0]) fd.append("presetsA", $("spwPresetsA").files[0]);
  if ($("spwPresetsB").files[0]) fd.append("presetsB", $("spwPresetsB").files[0]);
  fd.append("prefer", $("spwPreferFriend").checked ? "friend" : "mine");
  if (outputPath) fd.append("outputPath", outputPath);
  $("spwMergeBtn").disabled = true;
  $("spwMergeSaveBtn").disabled = true;
  try {
    const d = await runMultipartJob("/api/spawn/merge", fd, $("spwMergeLog"));
    const s = d.stats || {};
    $("spwMergeResult").innerHTML = `<div class="tools-status" style="margin-top:10px">` +
      [`типов: ${s.types}`, `добавлено новых: ${s.added}`, `конфликтов: ${s.conflicts}`, `идентичных пропущено: ${s.skipped}`]
        .map((x) => `<span class="tool-chip ok">${x}</span>`).join("") + `</div>` +
      ((d.conflicts || []).length ? `<details open style="margin-top:8px"><summary class="link-sum">Конфликты (${d.conflicts.length})</summary><ul>` +
        d.conflicts.map((c) => `<li>${esc(c)}</li>`).join("") + "</ul></details>" : "");
    let msg = "Слияние завершено";
    if (d.savedTo) msg += ". Сохранено в: " + d.savedTo;
    if (d.saveError) msg += ". ОШИБКА: " + d.saveError;
    setStatus("spwMergeStatus", msg, !d.saveError);
    if (!outputPath && d.zipToken) window.location.href = "/api/zip/" + d.zipToken;
  } catch (e) {
    setStatus("spwMergeStatus", "Ошибка: " + e.message, false);
  } finally {
    $("spwMergeBtn").disabled = false;
    $("spwMergeSaveBtn").disabled = false;
  }
}
$("spwMergeBtn").addEventListener("click", () => spwMerge(""));
$("spwMergeSaveBtn").addEventListener("click", () => {
  const p = $("spwOutPath").value.trim();
  if (!p) { setStatus("spwMergeStatus", "Укажите папку для сохранения", false); return; }
  spwMerge(p);
});

// ==================== ВКЛАДКА: МИССИЯ ====================
let msnToken = null, msnModel = null;

$("msnOpenBtn").addEventListener("click", async () => {
  const f = $("msnFile").files[0];
  if (!f) { setStatus("msnStatus", "Выберите файл миссии", false); return; }
  const fd = new FormData();
  fd.append("file", f, f.name);
  $("msnOpenBtn").disabled = true;
  try {
    const r = await fetch("/api/mission/open", { method: "POST", body: fd });
    const d = await r.json();
    if (!r.ok) throw new Error(d.error || "ошибка");
    msnToken = d.token;
    msnModel = d.model;
    renderMission(d.model);
    setStatus("msnStatus", "Файл открыт: " + d.model.fileName + ". Правьте значения и скачайте результат.", true);
  } catch (e) {
    setStatus("msnStatus", "Ошибка: " + e.message, false);
  } finally {
    $("msnOpenBtn").disabled = false;
  }
});

function msnInput(id, value, kind) {
  if (kind === "bool01") {
    return `<select class="msn-val" data-id="${esc(id)}"><option value="1" ${value === "1" ? "selected" : ""}>1 — вкл</option><option value="0" ${value !== "1" ? "selected" : ""}>0 — выкл</option></select>`;
  }
  if (kind === "boolTF") {
    return `<select class="msn-val" data-id="${esc(id)}"><option value="true" ${value === "true" ? "selected" : ""}>true</option><option value="false" ${value !== "true" ? "selected" : ""}>false</option></select>`;
  }
  return `<input type="text" class="msn-val" data-id="${esc(id)}" value="${esc(value)}" style="width:${kind === "number" ? "140px" : "100%"};min-width:120px">`;
}

function renderMission(m) {
  $("msnTitle").textContent = m.title;
  $("msnFileDesc").textContent = m.fileDesc || "";
  $("msnInfo").innerHTML = (m.info && m.info.length)
    ? `<details ${m.kind === "eventspawns" ? "open" : ""} style="margin-bottom:10px"><summary class="link-sum">Сводка / справка (${m.info.length})</summary><ul>` +
      m.info.map((i) => `<li class="${i.includes("ДУБЛИКАТ") || i.includes("НЕТ точек") ? "st-err" : "muted"}">${esc(i)}</li>`).join("") + "</ul></details>"
    : "";
  const box = $("msnForm");
  let h = "";
  (m.groups || []).forEach((g) => {
    h += `<details open style="margin-bottom:8px"><summary class="link-sum"><b>${esc(g.name)}</b>${g.desc ? " — " + esc(g.desc) : ""}</summary><div style="margin:8px 0 4px 12px">`;
    g.fields.forEach((f) => {
      h += `<div style="margin-bottom:10px">
        <div class="row" style="align-items:center"><span class="cn" style="min-width:260px;font-family:var(--mono)">${esc(f.label)}</span>${msnInput(f.id, f.value, f.kind)}</div>
        ${f.desc ? `<div class="muted" style="font-size:12px;margin-left:2px">${esc(f.desc)}</div>` : ""}
      </div>`;
    });
    h += `</div></details>`;
  });
  if (m.events && m.events.length) {
    h += `<div class="table-wrap" style="max-height:480px"><table><thead><tr>
      <th>Событие</th><th title="1 = включено">active</th><th title="целевое число одновременно">nominal</th><th>min</th><th>max</th>
      <th title="время жизни, сек">lifetime</th><th title="пауза до респавна, сек">restock</th><th title="не спавнить ближе к игрокам, м">saferadius</th><th>детей</th>
    </tr></thead><tbody>`;
    m.events.forEach((e, i) => {
      const cell = (k, v) => `<td><input type="text" class="msn-ev" data-i="${i}" data-k="${k}" value="${esc(v)}" style="width:76px"></td>`;
      h += `<tr><td class="cn">${esc(e.name)}</td>${cell("active", e.active)}${cell("nominal", e.nominal)}${cell("min", e.min)}${cell("max", e.max)}${cell("lifetime", e.lifetime)}${cell("restock", e.restock)}${cell("saferadius", e.saferadius)}<td class="cat">${e.children}</td></tr>`;
    });
    h += `</tbody></table></div>`;
  }
  box.innerHTML = h;
  $("msnSaveBtn").disabled = m.kind === "eventspawns";
  $("msnEditor").classList.remove("hidden");
}

$("msnSaveBtn").addEventListener("click", async () => {
  if (!msnToken || !msnModel) return;
  const values = {};
  document.querySelectorAll(".msn-val").forEach((el) => { values[el.dataset.id] = el.value; });
  const events = (msnModel.events || []).map((e) => ({ ...e }));
  document.querySelectorAll(".msn-ev").forEach((el) => {
    const e = events[+el.dataset.i];
    if (!e) return;
    const k = el.dataset.k;
    if (k === "active") e.active = el.value;
    else if (k === "nominal") e.nominal = el.value;
    else if (k === "min") e.min = el.value;
    else if (k === "max") e.max = el.value;
    else if (k === "lifetime") e.lifetime = el.value;
    else if (k === "restock") e.restock = el.value;
    else if (k === "saferadius") e.saferadius = el.value;
  });
  try {
    const r = await fetch("/api/mission/save", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token: msnToken, values, events }),
    });
    const d = await r.json();
    if (!r.ok) throw new Error(d.error || "ошибка");
    downloadText(d.fileName, d.content);
    setStatus("msnStatus", "Файл сформирован и скачан: " + d.fileName + ". Сделайте бэкап старого и замените его на сервере.", true);
  } catch (e) {
    setStatus("msnStatus", "Ошибка: " + e.message, false);
  }
});

// ---------- калькулятор дня и ночи ----------
$("dnCalcBtn").addEventListener("click", () => {
  const from = +$("dnLightFrom").value, to = +$("dnLightTo").value;
  const dayReal = +$("dnDayReal").value, nightReal = +$("dnNightReal").value;
  if (!(to > from) || dayReal <= 0 || nightReal <= 0) {
    alert("Проверьте значения: «до» должно быть больше «с», длительности — больше нуля");
    return;
  }
  const clamp = (v, a, b) => Math.min(b, Math.max(a, v));
  const dayGame = to - from, nightGame = 24 - dayGame;
  let accel = clamp(dayGame * 60 / dayReal, 0.1, 64);
  let nightMult = clamp((nightGame * 60 / nightReal) / accel, 0.1, 64);
  accel = Math.round(accel * 100) / 100;
  nightMult = Math.round(nightMult * 100) / 100;
  const fmt = (min) => {
    const h = Math.floor(min / 60), mm = Math.round(min % 60);
    return (h ? h + " ч " : "") + mm + " мин";
  };
  const actDay = dayGame * 60 / accel;
  const actNight = nightGame * 60 / (accel * nightMult);
  chips($("dnChips"), [
    [`игровой день: ${dayGame} ч → реально ${fmt(actDay)}`, "ok"],
    [`игровая ночь: ${nightGame} ч → реально ${fmt(actNight)}`, "ok"],
    [`полные сутки: ${fmt(actDay + actNight)}`, ""],
  ]);
  $("dnCfg").textContent =
    `serverTimeAcceleration = ${accel};        // ускорение времени днём\n` +
    `serverNightTimeAcceleration = ${nightMult};   // доп. множитель ночью (итог ночью: x${Math.round(accel * nightMult * 100) / 100})\n` +
    `serverTime = "SystemTime";           // стартовое время (или "2026/7/25/12/00")\n` +
    `serverTimePersistent = 0;            // 0 = время сбрасывается при рестарте`;
  $("dnResult").classList.remove("hidden");
});

// ==================== ВКЛАДКИ: INEDIA AI и TERJE ====================
const modcfgState = { ind: { token: null, model: null, zipToken: null }, trj: { token: null, model: null, zipToken: null } };

function modcfgRender(prefix, model) {
  const st = modcfgState[prefix];
  st.model = model;
  $(prefix + "Title").textContent = model.title;
  $(prefix + "FileDesc").textContent = model.fileDesc || "";
  const box = $(prefix + "Form");
  const gm = (guideState[prefix] && guideState[prefix].matches) || {};
  let h = "";
  let total = 0;
  (model.groups || []).forEach((g, gi) => {
    h += `<details ${gi < 3 ? "open" : ""} class="mc-group" style="margin-bottom:8px"><summary class="link-sum"><b>${esc(g.name)}</b>${g.desc ? " — " + esc(g.desc) : ""}</summary><div style="margin:8px 0 4px 12px">`;
    g.fields.forEach((f) => {
      total++;
      const gv = gm[f.id];
      h += `<div class="mc-row" data-key="${esc((f.label + " " + (f.desc || "") + " " + (gv ? gv.d : "")).toLowerCase())}" style="margin-bottom:10px">
        <div class="row" style="align-items:center"><span class="cn" style="min-width:300px;font-family:var(--mono);word-break:break-all">${esc(f.label)}</span>${msnInput(f.id, f.value, f.kind)}</div>
        ${f.desc ? `<div class="muted" style="font-size:12px">${esc(f.desc)}</div>` : ""}
        ${gv ? `<div class="muted gdesc" style="font-size:12px">${gv.ai ? "🤖" : "📖"} ${esc(gv.d)}</div>` : ""}
      </div>`;
    });
    h += `</div></details>`;
  });
  box.innerHTML = h;
  $(prefix + "Count").textContent = "параметров: " + total;
  $(prefix + "Editor").classList.remove("hidden");
}

function modcfgFilter(prefix) {
  const q = $(prefix + "Search").value.trim().toLowerCase();
  let shown = 0;
  document.querySelectorAll("#" + prefix + "Form .mc-row").forEach((row) => {
    const hit = !q || row.dataset.key.includes(q);
    row.style.display = hit ? "" : "none";
    if (hit) shown++;
  });
  document.querySelectorAll("#" + prefix + "Form .mc-group").forEach((g) => {
    const any = [...g.querySelectorAll(".mc-row")].some((r) => r.style.display !== "none");
    g.style.display = any ? "" : "none";
    if (q && any) g.open = true;
  });
  $(prefix + "Count").textContent = q ? "найдено: " + shown : "параметров: " + document.querySelectorAll("#" + prefix + "Form .mc-row").length;
}

async function modcfgOpen(prefix, mode, extraFd) {
  const fd = extraFd || new FormData();
  fd.append("mode", mode);
  const r = await fetch("/api/modcfg/open", { method: "POST", body: fd });
  const d = await r.json();
  if (!r.ok) throw new Error(d.error || "ошибка");
  const st = modcfgState[prefix];
  if (d.picker) {
    st.zipToken = d.token;
    const sel = $(prefix + "Picker");
    sel.innerHTML = "";
    d.picker.forEach((n) => {
      const o = document.createElement("option");
      o.value = n;
      o.textContent = n;
      sel.appendChild(o);
    });
    $(prefix + "PickerRow").classList.remove("hidden");
    setStatus(prefix + "Status", "Архив открыт — выберите файл из списка и нажмите «Открыть выбранный файл»", true);
    return;
  }
  st.token = d.token;
  modcfgRender(prefix, d.model);
  setStatus(prefix + "Status", "Файл открыт: " + d.model.fileName + ". Правьте значения (поиск помогает найти нужное) и скачайте результат.", true);
}

function modcfgWire(prefix, mode) {
  $(prefix + "OpenBtn").addEventListener("click", async () => {
    const f = $(prefix + "File").files[0];
    if (!f) { setStatus(prefix + "Status", "Выберите файл конфига или ZIP", false); return; }
    const fd = new FormData();
    fd.append("file", f, f.name);
    $(prefix + "OpenBtn").disabled = true;
    try {
      await modcfgOpen(prefix, mode, fd);
    } catch (e) {
      setStatus(prefix + "Status", "Ошибка: " + e.message, false);
    } finally {
      $(prefix + "OpenBtn").disabled = false;
    }
  });
  $(prefix + "PickBtn").addEventListener("click", async () => {
    const st = modcfgState[prefix];
    const inner = $(prefix + "Picker").value;
    if (!st.zipToken || !inner) return;
    const fd = new FormData();
    fd.append("token", st.zipToken);
    fd.append("inner", inner);
    $(prefix + "PickBtn").disabled = true;
    try {
      await modcfgOpen(prefix, mode, fd);
    } catch (e) {
      setStatus(prefix + "Status", "Ошибка: " + e.message, false);
    } finally {
      $(prefix + "PickBtn").disabled = false;
    }
  });
  $(prefix + "Search").addEventListener("input", () => modcfgFilter(prefix));
  $(prefix + "SaveBtn").addEventListener("click", async () => {
    const st = modcfgState[prefix];
    if (!st.token) return;
    const values = {};
    document.querySelectorAll("#" + prefix + "Form .msn-val").forEach((el) => { values[el.dataset.id] = el.value; });
    try {
      const r = await fetch("/api/modcfg/save", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token: st.token, values }),
      });
      const d = await r.json();
      if (!r.ok) throw new Error(d.error || "ошибка");
      downloadText(d.fileName, d.content);
      setStatus(prefix + "Status", "Файл сформирован и скачан: " + d.fileName + ". Сделайте бэкап старого и проверьте новый перед заливкой.", true);
    } catch (e) {
      setStatus(prefix + "Status", "Ошибка: " + e.message, false);
    }
  });
}
modcfgWire("ind", "inedia");
modcfgWire("trj", "terje");

// ==================== ВКЛАДКА: ПРОВЕРКА ФАЙЛОВ ====================
$("chkFiles").addEventListener("change", () => { $("chkFolder").value = ""; });
$("chkFolder").addEventListener("change", () => { $("chkFiles").value = ""; });
$("chkRunBtn").addEventListener("click", async () => {
  const folder = $("chkFolder").files;
  const files = folder.length ? [...folder].filter((f) => {
    const path = f.webkitRelativePath.replace(/\\/g, "/");
    return /\.(xml|json)$/i.test(f.name) && !/(^|\/)storage[^/]*\//i.test(path);
  }) : [...$("chkFiles").files];
  if (!files.length) { setStatus("chkStatus", "Выберите файлы для проверки", false); return; }
  if (files.length > 200) { setStatus("chkStatus", "Максимум 200 файлов за одну проверку", false); return; }
  if (files.some((f) => f.size > 128 * 1024 * 1024) || files.reduce((sum, f) => sum + f.size, 0) > 255 * 1024 * 1024) {
    setStatus("chkStatus", "Слишком большой объём файлов для одной проверки", false); return;
  }
  const fd = new FormData();
  files.forEach((f) => {
    fd.append("files", f, f.name);
    fd.append("paths", f.webkitRelativePath || f.name);
  });
  const fmt = folder.length ? "" : ($("chkFormat") ? $("chkFormat").value : "");
  if (fmt) fd.append("format", fmt);
  $("chkRunBtn").disabled = true;
  $("chkResults").innerHTML = "";
  try {
    const d = await runMultipartJob("/api/validate", fd, $("chkLog"));
    renderValidation(d);
    setStatus("chkStatus", `Проверено файлов: ${d.total}, без ошибок: ${d.ok}`, d.ok === d.total);
  } catch (e) {
    setStatus("chkStatus", "Ошибка: " + e.message, false);
  } finally {
    $("chkRunBtn").disabled = false;
  }
});

function renderValidation(d) {
  const box = $("chkResults");
  let h = "";
  (d.reports || []).forEach((r) => {
    const kind = r.dayzKind || r.kind;
    const head = `${r.ok ? "✔" : "✖"} ${esc(r.name)} <span class="tag">${esc(kind)}</span> ` +
      (r.problems && r.problems.length ? `<span class="${r.ok ? "st-res" : "st-err"}">— замечаний: ${r.problems.length}</span>` : `<span class="st-ok">— ошибок нет</span>`);
    let inner = "";
    (r.problems || []).forEach((p) => {
      inner += `<div style="margin:10px 0;padding:10px;border:1px solid ${p.level === "error" ? "#84392f" : "#7d5c1d"};border-radius:8px">
        <div class="${p.level === "error" ? "st-err" : ""}" style="${p.level !== "error" ? "color:var(--warn)" : ""}">
          ${p.line ? `<b>строка ${p.line}:</b> ` : ""}${esc(p.msg)}</div>
        ${p.suggestion ? `<div class="st-ok" style="margin-top:4px">💡 Как исправить: ${esc(p.suggestion)}</div>` : ""}
        ${p.excerpt ? `<pre class="joblog" style="margin-top:6px;max-height:140px">${esc(p.excerpt)}</pre>` : ""}
      </div>`;
    });
    h += `<details ${r.ok ? "" : "open"} style="margin-top:10px"><summary class="link-sum">${head}</summary>${inner || ""}</details>`;
  });
  box.innerHTML = h;
}

// ---------- глобальный журнал (вкладка «Журнал») ----------
let glogFrom = 0;
let glogTimer = null;

function tickGlobalLog() {
  fetch(`/api/log?from=${glogFrom}`)
    .then((r) => r.json())
    .then((d) => {
      if (glogFrom === 0) $("globalLog").textContent = "";
      (d.lines || []).forEach((l) => appendLog($("globalLog"), l));
      glogFrom = d.total;
    })
    .catch(() => {});
}

function setGlobalLogPolling(on) {
  if (on && !glogTimer) {
    tickGlobalLog();
    glogTimer = setInterval(tickGlobalLog, 1000);
  } else if (!on && glogTimer) {
    clearInterval(glogTimer);
    glogTimer = null;
  }
}

$("logCopyBtn").addEventListener("click", () => {
  const text = $("globalLog").textContent;
  const done = () => {
    $("logCopyBtn").textContent = "Скопировано ✓";
    setTimeout(() => ($("logCopyBtn").textContent = "Скопировать журнал"), 1500);
  };
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(done).catch(() => fallbackCopy(text, done));
  } else fallbackCopy(text, done);
});

function fallbackCopy(text, done) {
  const ta = document.createElement("textarea");
  ta.value = text;
  document.body.appendChild(ta);
  ta.select();
  try { document.execCommand("copy"); } catch (e) {}
  document.body.removeChild(ta);
  done();
}

$("logClearBtn").addEventListener("click", () => {
  fetch("/api/log/clear", { method: "POST" }).then(() => {
    $("globalLog").textContent = "";
    glogFrom = 0;
    tickGlobalLog();
  });
});

// ---------- пульс: программа видит, что вкладка открыта ----------
const clientId = Math.random().toString(36).slice(2);
function ping() { fetch("/api/ping?id=" + clientId).catch(() => {}); }
ping();
setInterval(ping, 2000);

// ---------- load registry & settings ----------
fetch("/api/registry").then((r) => r.json()).then((r) => {
  registry = r || [];
  registry.forEach((d) => (regMap[d.key] = d));
});

let toolsFound = {}; // key -> found bool
let appSettings = {}; // последние настройки с сервера (в т.ч. ИИ)
fetch("/api/settings").then((r) => r.json()).then((d) => {
  const s = (d && d.settings) || {};
  appSettings = s;
  if (s.toolsPath) {
    $("toolsPath").value = s.toolsPath;
    detectTools(s.toolsPath, true);
  }
  if (s.privateKeyPath) $("keyPath").value = s.privateKeyPath;
  if (s.outputPath) $("outPath").value = s.outputPath;
  $("signPbo").checked = !!s.sign;
  $("binarizeCfg").checked = !!s.binarize;
  $("useFileBank").checked = !!s.useFileBank;
  $("signBlock").classList.toggle("hidden", !s.sign);
  aiUiRefresh();
}).catch(() => {});

// ---------- DayZ Tools ----------
function renderTools(list) {
  toolsFound = {};
  const box = $("toolsStatus");
  box.innerHTML = "";
  (list || []).forEach((t) => {
    toolsFound[t.key] = !!t.found;
    const chip = document.createElement("span");
    chip.className = "tool-chip " + (t.found ? "ok" : "miss");
    chip.title = t.found ? t.path : "не найден в указанной папке";
    chip.innerHTML = esc(t.label) + (t.found && t.modTime ? ` <span class="ver">${esc(t.modTime)}</span>` : "");
    box.appendChild(chip);
  });
}

function detectTools(path, quiet) {
  return fetch("/api/tools/detect", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path }),
  })
    .then(async (r) => {
      const d = await r.json();
      if (d.tools) renderTools(d.tools);
      if (!r.ok) throw new Error(d.error || "ошибка проверки");
      return d;
    })
    .catch((e) => {
      if (!quiet) alert("DayZ Tools: " + e.message);
    });
}

$("toolsCheckBtn").addEventListener("click", () => {
  const p = $("toolsPath").value.trim();
  if (!p) { alert("Укажите путь к папке DayZ Tools"); return; }
  detectTools(p, false);
});

$("toolsAutoBtn").addEventListener("click", () => {
  $("toolsAutoBtn").disabled = true;
  fetch("/api/tools/autodetect")
    .then(async (r) => {
      const d = await r.json();
      if (!r.ok) throw new Error(d.error || "не найдено");
      $("toolsPath").value = d.path;
      renderTools(d.tools);
    })
    .catch((e) => alert(e.message))
    .finally(() => ($("toolsAutoBtn").disabled = false));
});

$("signPbo").addEventListener("change", () => {
  $("signBlock").classList.toggle("hidden", !$("signPbo").checked);
  if ($("signPbo").checked) $("makePbo").checked = true;
});

$("createKeyBtn").addEventListener("click", () => {
  const name = $("newKeyName").value.trim();
  const dir = $("newKeyDir").value.trim();
  if (!name || !dir) { alert("Укажите имя ключа и папку для сохранения"); return; }
  $("createKeyBtn").disabled = true;
  fetch("/api/tools/createkey", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, dir, toolsPath: $("toolsPath").value.trim() }),
  })
    .then(async (r) => {
      const d = await r.json();
      if (!r.ok) throw new Error(d.error || "ошибка создания ключа");
      $("keyPath").value = d.privateKey;
      genStatus("Ключи созданы: " + d.privateKey + " и " + d.publicKey + ". Приватный ключ храните в секрете!", true);
    })
    .catch((e) => genStatus("Ошибка создания ключа: " + e.message, false))
    .finally(() => ($("createKeyBtn").disabled = false));
});

// ---------- tab 1: mod selection ----------
const dropZone = $("dropZone");
$("pickBtn").addEventListener("click", () => $("fileInput").click());
$("fileInput").addEventListener("change", (e) => { if (e.target.files.length) uploadFiles(e.target.files); });
["dragenter", "dragover"].forEach((ev) => dropZone.addEventListener(ev, (e) => { e.preventDefault(); dropZone.classList.add("over"); }));
["dragleave", "drop"].forEach((ev) => dropZone.addEventListener(ev, (e) => { e.preventDefault(); dropZone.classList.remove("over"); }));
dropZone.addEventListener("drop", (e) => {
  const files = [...e.dataTransfer.files].filter((f) => f.name.toLowerCase().endsWith(".pbo"));
  if (files.length) uploadFiles(files);
  else alert("Перетащите файлы с расширением .pbo");
});
$("analyzePathBtn").addEventListener("click", () => {
  const p = $("pathInput").value.trim();
  if (!p) { alert("Укажите путь к .pbo или папке мода"); return; }
  analyzeRequest(fetch("/api/analyze", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ paths: [p] }),
  }));
});

// XHR-загрузка с прогрессом (fetch не умеет прогресс отправки)
function xhrUpload(url, fd, onProgress) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", url);
    xhr.responseType = "json";
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) onProgress(e.loaded, e.total);
    };
    xhr.onload = () => {
      const d = xhr.response || {};
      if (xhr.status >= 200 && xhr.status < 300) resolve(d);
      else reject(new Error(d.error || "HTTP " + xhr.status));
    };
    xhr.onerror = () => reject(new Error("сбой передачи файлов в программу"));
    xhr.send(fd);
  });
}

function uploadFiles(files) {
  const arr = [...files];
  const total = arr.reduce((s, f) => s + f.size, 0);
  if (total > 700 * 1e6) {
    if (!confirm(
      `Выбранные файлы очень большие: ${(total / 1e9).toFixed(1)} ГБ.\n\n` +
      `Загрузка займёт время (файлы копируются во временную папку программы).\n` +
      `РЕКОМЕНДАЦИЯ: нажмите «Отмена» и укажите ПУТЬ к папке мода в поле ниже — ` +
      `анализ начнётся мгновенно, без копирования.\n\nВсё равно загрузить файлы?`
    )) return;
  }
  const fd = new FormData();
  arr.forEach((f) => fd.append("pbos", f, f.name));
  const log = $("analyzeLog");
  log.textContent = "";
  $("analyzeLogCard").classList.remove("hidden");
  $("loadMod").classList.remove("hidden");
  $("modSummary").classList.add("hidden");
  appendLog(log, `Передаю файлы в программу: ${arr.length} шт., ${(total / 1e6).toFixed(0)} МБ…`);
  let lastPct = 0, doneNote = false;
  xhrUpload("/api/analyze", fd, (loaded, tot) => {
    const pct = Math.floor((loaded / tot) * 100);
    if (pct >= lastPct + 10 && pct < 100) {
      lastPct = pct;
      appendLog(log, `  передано ${pct}% (${(loaded / 1e6).toFixed(0)} МБ из ${(tot / 1e6).toFixed(0)} МБ)`);
    } else if (pct >= 100 && !doneNote) {
      doneNote = true;
      appendLog(log, "  передано 100% — программа сохраняет файлы во временную папку, ждите…");
    }
  })
    .then((data) => {
      appendLog(log, "Файлы приняты, анализ запущен.");
      return pollJob(data.jobId, log);
    })
    .then(handleAnalysisResult)
    .catch((e) => {
      appendLog(log, "ОШИБКА: " + e.message);
      alert("Ошибка анализа: " + e.message);
    })
    .finally(() => $("loadMod").classList.add("hidden"));
}

// ---------- live job log (опрос прогресса) ----------
function appendLog(el, line) {
  el.textContent += line + "\n";
  el.scrollTop = el.scrollHeight;
}

async function pollJob(jobId, logEl) {
  let from = 0;
  for (;;) {
    const r = await fetch(`/api/job/${jobId}?from=${from}`);
    const d = await r.json();
    if (!r.ok) throw new Error(d.error || "задача потеряна");
    (d.lines || []).forEach((l) => appendLog(logEl, l));
    from = d.total;
    if (d.done) {
      if (d.error) {
        appendLog(logEl, "ОШИБКА: " + d.error);
        throw new Error(d.error);
      }
      return d.result;
    }
    await new Promise((res) => setTimeout(res, 300));
  }
}

function handleAnalysisResult(data) {
  analysis = data;
  edits.clear();
  renderModSummary();
  buildCatFilter();
  renderTable();
  prefillGeneration();
}

function analyzeRequest(promise) {
  const log = $("analyzeLog");
  log.textContent = "";
  $("analyzeLogCard").classList.remove("hidden");
  $("loadMod").classList.remove("hidden");
  $("modSummary").classList.add("hidden");
  promise
    .then(async (r) => {
      const data = await r.json();
      if (!r.ok) throw new Error(data.error || "неизвестная ошибка");
      appendLog(log, "Задача запущена, жду журнал…");
      return pollJob(data.jobId, log);
    })
    .then(handleAnalysisResult)
    .catch((e) => {
      appendLog(log, "ОШИБКА: " + e.message);
      alert("Ошибка анализа: " + e.message);
    })
    .finally(() => $("loadMod").classList.add("hidden"));
}

function renderModSummary() {
  const box = $("pboList");
  box.innerHTML = "";
  (analysis.pbos || []).forEach((p) => {
    const div = document.createElement("div");
    div.className = "pbo-row";
    const stCls = p.status === "error" ? "st-err" : (p.status === "resources" ? "st-res" : "st-ok");
    let verdict = "";
    if (p.status === "ok" && p.configsFound > 0) {
      verdict = p.itemClasses > 0
        ? `<span class="st-ok">✔ предметов для редактирования: <b>${p.itemClasses}</b> (конфигов: ${p.configsFound})</span>`
        : `<span class="st-res">предметов для редактирования нет — звуковые/служебные конфиги (${p.configsFound} шт.), для оверрайдов не нужен</span>`;
    }
    div.innerHTML = `<b>${esc(p.fileName)}</b>
      ${p.prefix ? `<span>prefix: <span class="tag">${esc(p.prefix)}</span></span>` : ""}
      <span class="${stCls}">${esc(p.configSrc || "конфиг не найден")}</span>
      ${p.numFiles ? `<span class="muted">файлов: ${p.numFiles}</span>` : ""}
      ${verdict}`;
    box.appendChild(div);
  });
  const itemsCount = (analysis.items || []).length;
  if (itemsCount === 0) {
    const note = document.createElement("div");
    note.className = "status err";
    note.style.marginTop = "10px";
    note.textContent = "Предметов не найдено. Читаемых конфигов в выбранных PBO нет — раскройте предупреждения ниже и вкладку «Журнал», там написано, что с каждым PBO.";
    box.appendChild(note);
  }
  const pb = $("patchesBox");
  const patches = analysis.patches || [];
  pb.innerHTML = patches.length
    ? `<div class="muted" style="margin-bottom:4px">CfgPatches (аддоны мода — попадут в requiredAddons):</div>` +
      patches.map((p) => `<span class="tag">${esc(p.name)}</span>`).join("")
    : `<div class="muted">CfgPatches не найден — requiredAddons заполните вручную на шаге 3.</div>`;
  const warns = analysis.warnings || [];
  const wb = $("warnBox");
  if (warns.length) {
    wb.classList.remove("hidden");
    wb.querySelector("summary").textContent = `Предупреждения (${warns.length})`;
    $("warnList").innerHTML = warns.map((w) => `<li>${esc(w)}</li>`).join("");
  } else wb.classList.add("hidden");
  // debinarized original config viewers
  const cv = $("cfgViewers");
  cv.innerHTML = "";
  const cfgs = analysis.configCpp || {};
  Object.keys(cfgs).forEach((name) => {
    const det = document.createElement("details");
    det.style.marginTop = "10px";
    const sum = document.createElement("summary");
    sum.textContent = "Показать дебинаризованный config.cpp — " + name;
    sum.style.cursor = "pointer";
    sum.style.color = "#b9d18d";
    const pre = document.createElement("pre");
    pre.className = "preview";
    pre.style.maxHeight = "420px";
    pre.textContent = cfgs[name];
    det.appendChild(sum);
    det.appendChild(pre);
    cv.appendChild(det);
  });
  $("modSummary").classList.remove("hidden");
  const n = (analysis.items || []).length;
  $("toItemsBtn").textContent = `Перейти к предметам (${n}) →`;
}

$("toItemsBtn").addEventListener("click", () => showTab("items"));
$("toGenBtn").addEventListener("click", () => { renderGenSummary(); showTab("genr"); });

// ---------- tab 2: items table ----------
function buildCatFilter() {
  const sel = $("catFilter");
  sel.innerHTML = `<option value="">Все категории</option>`;
  const cats = [...new Set((analysis.items || []).map((i) => i.category))];
  cats.forEach((c) => {
    const o = document.createElement("option");
    o.value = c; o.textContent = c;
    sel.appendChild(o);
  });
}

["searchInput", "catFilter", "scopeFilter", "editedFilter", "showEmptyCols"].forEach((id) => {
  $(id).addEventListener("input", renderTable);
});
$("checkAllBtn").addEventListener("click", () => {
  visibleItems().forEach((it) => ensureEdit(it.id).sel = true);
  renderTable();
});
$("uncheckAllBtn").addEventListener("click", () => {
  edits.forEach((e) => (e.sel = false));
  renderTable();
});

function ensureEdit(id) {
  if (!edits.has(id)) edits.set(id, { sel: false, params: {}, customs: [] });
  return edits.get(id);
}
function editOf(id) { return edits.get(id); }
function hasChanges(id) {
  const e = editOf(id);
  return !!e && (Object.keys(e.params).length > 0 || e.customs.length > 0);
}

function visibleItems() {
  if (!analysis) return [];
  const q = $("searchInput").value.trim().toLowerCase();
  const cat = $("catFilter").value;
  const scopeOnly = $("scopeFilter").checked;
  const editedOnly = $("editedFilter").checked;
  return (analysis.items || []).filter((it) => {
    if (cat && it.category !== cat) return false;
    if (scopeOnly) {
      const s = it.scope ? it.scope.display : "?";
      if (s !== "2" && s !== "?") return false;
    }
    if (editedOnly && !hasChanges(it.id)) return false;
    if (q && !(it.classname.toLowerCase().includes(q) || (it.displayName || "").toLowerCase().includes(q))) return false;
    return true;
  });
}

function applicable(def, item) {
  return !def.sections || def.sections.length === 0 || def.sections.includes(item.category);
}

function renderTable() {
  if (!analysis) return;
  const items = visibleItems();
  const showEmpty = $("showEmptyCols").checked;
  // decide visible columns
  const cols = registry.filter((def) => {
    const anyApplicable = items.some((it) => applicable(def, it));
    if (!anyApplicable) return false;
    if (showEmpty) return true;
    return items.some((it) => {
      const pv = it.params[def.key];
      const ed = editOf(it.id);
      return (pv && pv.exists) || (ed && def.key in ed.params);
    });
  });

  let html = `<table><thead><tr>
    <th></th><th>Класснейм</th><th>Категория</th><th>scope</th>`;
  cols.forEach((c) => (html += `<th title="${esc(c.path)}">${esc(c.label)}</th>`));
  html += `<th>+</th></tr></thead><tbody>`;

  const shown = items.slice(0, MAX_ROWS);
  shown.forEach((it) => {
    const ed = editOf(it.id);
    const checked = ed && ed.sel;
    html += `<tr class="${checked ? "checked" : ""}" data-id="${it.id}">
      <td><input type="checkbox" class="selbox" data-id="${it.id}" ${checked ? "checked" : ""}></td>
      <td class="cn">${esc(it.classname)}${it.displayName ? `<span class="dn">${esc(it.displayName)}</span>` : ""}
        ${it.vanillaBase ? `<span class="dn" title="Родительская цепочка уходит в ванильный класс">база: ${esc(it.vanillaBase)}</span>` : ""}</td>
      <td class="cat">${esc(it.category.replace("Cfg", ""))}</td>
      <td class="cat">${esc(it.scope ? it.scope.display : "?")}</td>`;
    cols.forEach((def) => {
      if (!applicable(def, it)) { html += `<td class="cat">·</td>`; return; }
      const pv = it.params[def.key];
      const edited = ed && def.key in ed.params;
      let inner;
      if (edited) {
        const oldV = pv && pv.exists ? `<span class="old">${esc(pv.display)}</span>` : "";
        inner = `${oldV}<span class="edited">${esc(ed.params[def.key])}</span>`;
      } else if (pv && pv.exists) {
        const cls = pv.source ? "inh" : "";
        const tip = pv.source ? ` title="унаследовано от ${esc(pv.source)}"` : "";
        inner = `<span class="${cls}"${tip}>${esc(pv.display)}</span>`;
      } else {
        inner = `<span class="abs">—</span>`;
      }
      html += `<td class="pcell" data-id="${it.id}" data-key="${def.key}">${inner}</td>`;
    });
    // customs badges + add button
    let badges = "";
    if (ed) {
      ed.customs.forEach((c, i) => {
        badges += `<span class="badge-cust" data-id="${it.id}" data-ci="${i}" title="кликните, чтобы удалить">${esc(c.path)}=${esc(c.value)}</span> `;
      });
    }
    html += `<td><span class="addp" data-id="${it.id}" title="Добавить параметр">+</span><br>${badges}</td></tr>`;
  });
  html += `</tbody></table>`;
  if (items.length > MAX_ROWS) {
    html += `<div class="trunc-note">Показаны первые ${MAX_ROWS} из ${items.length} — уточните поиск или фильтры.</div>`;
  }
  $("tableWrap").innerHTML = html;

  // events
  $("tableWrap").querySelectorAll(".selbox").forEach((cb) => {
    cb.addEventListener("change", () => {
      ensureEdit(+cb.dataset.id).sel = cb.checked;
      cb.closest("tr").classList.toggle("checked", cb.checked);
      updateCounter();
    });
  });
  $("tableWrap").querySelectorAll(".pcell").forEach((td) => {
    td.addEventListener("click", () => openEditPop(td, +td.dataset.id, td.dataset.key));
  });
  $("tableWrap").querySelectorAll(".addp").forEach((el) => {
    el.addEventListener("click", () => openAddModal(+el.dataset.id));
  });
  $("tableWrap").querySelectorAll(".badge-cust").forEach((el) => {
    el.addEventListener("click", () => {
      const ed = editOf(+el.dataset.id);
      if (ed && confirm("Удалить этот произвольный параметр?")) {
        ed.customs.splice(+el.dataset.ci, 1);
        renderTable();
      }
    });
  });
  updateCounter();
}

function updateCounter() {
  let sel = 0, chg = 0;
  edits.forEach((e, id) => {
    if (e.sel) sel++;
    chg += Object.keys(e.params).length + e.customs.length;
  });
  $("counter").textContent = `отмечено: ${sel} · правок: ${chg}`;
}

// ---------- edit popover ----------
let popCtx = null;
const pop = $("editPop");
function openEditPop(td, id, key) {
  const def = regMap[key];
  const it = analysis.items[id];
  if (!def || !it) return;
  popCtx = { id, key };
  $("popTitle").textContent = `${it.classname} · ${def.label}`;
  $("popHint").textContent = def.hint || "";
  const ed = editOf(id);
  const pv = it.params[key];
  $("popInput").value = ed && key in ed.params ? ed.params[key] : (pv && pv.exists ? pv.raw : "");
  $("popReset").classList.toggle("hidden", !(ed && key in ed.params));
  const r = td.getBoundingClientRect();
  pop.classList.remove("hidden");
  const pw = 300, ph = pop.offsetHeight || 150;
  let x = Math.min(r.left + window.scrollX, window.scrollX + document.documentElement.clientWidth - pw - 16);
  let y = r.bottom + window.scrollY + 4;
  if (r.bottom + ph + 20 > window.innerHeight) y = r.top + window.scrollY - ph - 6;
  pop.style.left = x + "px";
  pop.style.top = y + "px";
  $("popInput").focus();
  $("popInput").select();
}
function closePop() { pop.classList.add("hidden"); popCtx = null; }
$("popCancel").addEventListener("click", closePop);
$("popReset").addEventListener("click", () => {
  if (popCtx) {
    const ed = editOf(popCtx.id);
    if (ed) delete ed.params[popCtx.key];
    renderTable();
  }
  closePop();
});
$("popOk").addEventListener("click", applyPop);
$("popInput").addEventListener("keydown", (e) => {
  if (e.key === "Enter") applyPop();
  if (e.key === "Escape") closePop();
});
document.addEventListener("mousedown", (e) => {
  if (!pop.classList.contains("hidden") && !pop.contains(e.target) && !e.target.classList.contains("pcell")) closePop();
});
function applyPop() {
  if (!popCtx) return;
  const v = $("popInput").value.trim();
  const def = regMap[popCtx.key];
  if (v === "") { closePop(); return; }
  const err = validateValue(def.type, v);
  if (err) { alert(err); return; }
  const ed = ensureEdit(popCtx.id);
  ed.params[popCtx.key] = v;
  ed.sel = true;
  closePop();
  renderTable();
}

function validateValue(type, v) {
  const num = (s) => /^-?\d+([.,]\d+)?$/.test(s.trim());
  if (type === "float" || type === "int") {
    if (!num(v)) return "Ожидается число, например 0.5";
  }
  if (type === "intpair") {
    const parts = v.replace(/[{}]/g, "").split(",").map((s) => s.trim()).filter(Boolean);
    if (parts.length !== 2 || !parts.every(num)) return "Ожидаются два числа через запятую, например: 10,4";
  }
  return null;
}

// ---------- add param modal (single item or bulk) ----------
let modalItemId = null; // number = single item, null = bulk mode
function openAddModal(id) {
  modalItemId = id;
  const it = analysis.items[id];
  $("modalTitle").textContent = "Добавить параметр: " + it.classname;
  const sel = $("modalParam");
  sel.innerHTML = "";
  registry.filter((d) => applicable(d, it)).forEach((d) => {
    const pv = it.params[d.key];
    const o = document.createElement("option");
    o.value = d.key;
    o.textContent = d.label + (pv && pv.exists ? " (есть: " + pv.display + ")" : " (нет — будет добавлен)");
    sel.appendChild(o);
  });
  const oc = document.createElement("option");
  oc.value = "custom";
  oc.textContent = "Произвольный параметр (свой путь)…";
  sel.appendChild(oc);
  onModalParamChange();
  $("modalValue").value = "";
  $("modalBack").classList.remove("hidden");
}

function openBulkModal() {
  let n = 0;
  edits.forEach((e) => { if (e.sel) n++; });
  if (!n) { alert("Сначала отметьте предметы галочками (или кнопкой «Отметить видимые»)."); return; }
  modalItemId = null;
  $("modalTitle").textContent = `Массовая правка — отмечено предметов: ${n}`;
  const sel = $("modalParam");
  sel.innerHTML = "";
  registry.forEach((d) => {
    const o = document.createElement("option");
    o.value = d.key;
    o.textContent = d.label;
    sel.appendChild(o);
  });
  const oc = document.createElement("option");
  oc.value = "custom";
  oc.textContent = "Произвольный параметр (свой путь)…";
  sel.appendChild(oc);
  onModalParamChange();
  $("modalValue").value = "";
  $("modalBack").classList.remove("hidden");
}
$("bulkBtn").addEventListener("click", openBulkModal);
function onModalParamChange() {
  const key = $("modalParam").value;
  const custom = key === "custom";
  $("modalCustomFields").classList.toggle("hidden", !custom);
  $("modalHint").textContent = custom
    ? "Пример пути: Protection/biological — создаст class Protection { biological = значение; }"
    : (regMap[key] ? regMap[key].hint : "");
}
$("modalParam").addEventListener("change", onModalParamChange);
$("modalCancel").addEventListener("click", () => $("modalBack").classList.add("hidden"));
$("modalBack").addEventListener("mousedown", (e) => { if (e.target === $("modalBack")) $("modalBack").classList.add("hidden"); });
$("modalOk").addEventListener("click", () => {
  const key = $("modalParam").value;
  const v = $("modalValue").value.trim();
  if (!v) { alert("Укажите значение"); return; }
  let customPath = null, customType = null;
  if (key === "custom") {
    customPath = $("modalPath").value.trim().replace(/\\/g, "/").replace(/^\/+|\/+$/g, "");
    if (!customPath || !customPath.split("/").every((s) => /^[A-Za-z0-9_]+$/.test(s))) {
      alert("Путь: сегменты из латиницы/цифр/_ через /"); return;
    }
    customType = $("modalType").value;
  } else {
    const err = validateValue(regMap[key].type, v);
    if (err) { alert(err); return; }
  }
  const applyTo = (id) => {
    const ed = ensureEdit(id);
    if (key === "custom") {
      // replace existing custom with same path
      const i = ed.customs.findIndex((c) => c.path.toLowerCase() === customPath.toLowerCase());
      if (i >= 0) ed.customs[i] = { path: customPath, type: customType, value: v };
      else ed.customs.push({ path: customPath, type: customType, value: v });
    } else {
      ed.params[key] = v;
    }
    ed.sel = true;
  };
  if (modalItemId === null) {
    // bulk: apply to all checked items where the param is applicable
    let applied = 0;
    edits.forEach((ed, id) => {
      if (!ed.sel) return;
      const it = analysis.items[id];
      if (key !== "custom" && !applicable(regMap[key], it)) return;
      applyTo(id);
      applied++;
    });
    if (!applied) { alert("Параметр не применим ни к одному из отмеченных предметов."); return; }
  } else {
    applyTo(modalItemId);
  }
  $("modalBack").classList.add("hidden");
  renderTable();
});

// ---------- tab 3: generation ----------
function sanitizeName(s) {
  s = (s || "").replace(/[^A-Za-z0-9_]/g, "");
  if (/^\d/.test(s)) s = "M" + s;
  return s || "MyOverride";
}
function prefillGeneration() {
  const prefix = analysis.pbos && analysis.pbos.length ? analysis.pbos[0].prefix : "MyMod";
  $("addonName").value = sanitizeName(prefix.split(/[\\/]/)[0]) + "_Override";
  const names = [...new Set((analysis.patches || []).map((p) => p.name))];
  $("reqAddons").value = names.join(", ");
}

function collectRequest() {
  const items = [];
  edits.forEach((ed, id) => {
    if (!ed.sel || !hasChanges(id)) return;
    const it = analysis.items[id];
    const changes = [];
    for (const [k, v] of Object.entries(ed.params)) changes.push({ param: k, value: v });
    ed.customs.forEach((c) => changes.push({ param: "custom", path: c.path, type: c.type, value: c.value }));
    items.push({ classname: it.classname, category: it.category, parent: it.parent, changes });
  });
  return {
    addonName: sanitizeName($("addonName").value.trim()),
    requiredAddons: $("reqAddons").value.split(/[,;\n]/).map((s) => s.trim()).filter(Boolean),
    makePbo: $("makePbo").checked,
    items,
    toolsPath: $("toolsPath").value.trim(),
    sign: $("signPbo").checked,
    privateKeyPath: $("keyPath").value.trim(),
    binarize: $("binarizeCfg").checked,
    useFileBank: $("useFileBank").checked,
  };
}

function validateToolsChoices(req) {
  if (req.sign && !req.privateKeyPath) {
    return "Включена подпись, но не указан приватный ключ (.biprivatekey). Укажите ключ или создайте новый.";
  }
  if ((req.sign || req.binarize || req.useFileBank) && !req.toolsPath) {
    return "Выбраны функции DayZ Tools (подпись/бинаризация/FileBank), но не указан путь к DayZ Tools.";
  }
  return null;
}

function renderGenSummary() {
  const req = collectRequest();
  const box = $("editsSummary");
  if (!req.items.length) {
    box.innerHTML = `<div class="muted">Пока нет предметов с правками. Вернитесь на шаг 2: отметьте предметы и измените значения (или добавьте параметры через «+»).</div>`;
    return;
  }
  let html = `<div style="margin-bottom:6px">Войдут в аддон: <b>${req.items.length}</b> предм.</div>`;
  req.items.forEach((it) => {
    const chg = it.changes.map((c) => c.param === "custom" ? `${c.path}=${c.value}` : `${(regMap[c.param] || { label: c.param }).label}: ${c.value}`).join("; ");
    html += `<div><b>${esc(it.classname)}</b> — <span class="chg">${esc(chg)}</span></div>`;
  });
  box.innerHTML = html;
}

function genStatus(msg, ok) {
  const el = $("genStatus");
  el.textContent = msg;
  el.className = "status " + (ok ? "ok" : "err");
  el.classList.remove("hidden");
}

async function callGenerate(extra) {
  const req = Object.assign(collectRequest(), extra || {});
  if (!req.items.length) throw new Error("нет предметов с правками (шаг 2)");
  const verr = validateToolsChoices(req);
  if (verr) throw new Error(verr);
  const log = $("genLog");
  log.textContent = "";
  log.classList.remove("hidden");
  const r = await fetch("/api/generate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  const data = await r.json();
  if (!r.ok) throw new Error(data.error || "неизвестная ошибка");
  return pollJob(data.jobId, log);
}

function warnSuffix(data) {
  const w = data && data.warnings;
  return w && w.length ? " ⚠ " + w.join(" ") : "";
}

$("previewBtn").addEventListener("click", async () => {
  try {
    renderGenSummary();
    const data = await callGenerate();
    $("cfgPreview").textContent = data.configCpp;
    genStatus("Предпросмотр обновлён. Архив уже готов — можно скачивать." + warnSuffix(data), true);
  } catch (e) { genStatus("Ошибка: " + e.message, false); }
});

$("downloadBtn").addEventListener("click", async () => {
  try {
    renderGenSummary();
    const data = await callGenerate();
    $("cfgPreview").textContent = data.configCpp;
    genStatus("ZIP сформирован (" + (data.files || []).length + " файлов): " + (data.files || []).join(", ") + "." + warnSuffix(data), true);
    window.location.href = "/api/zip/" + data.zipToken;
  } catch (e) { genStatus("Ошибка: " + e.message, false); }
});

// ==================== ОБЩИЕ ХЕЛПЕРЫ НОВЫХ ВКЛАДОК ====================
function downloadText(filename, text) {
  const blob = new Blob([text], { type: "application/octet-stream" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = filename;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 10000);
}

async function runMultipartJob(url, fd, logEl) {
  logEl.textContent = "";
  logEl.classList.remove("hidden");
  let lastPct = 0, doneNote = false;
  const data = await xhrUpload(url, fd, (loaded, tot) => {
    if (tot < 20 * 1e6) return; // мелкие файлы — без прогресса
    const pct = Math.floor((loaded / tot) * 100);
    if (pct >= lastPct + 10 && pct < 100) {
      lastPct = pct;
      appendLog(logEl, `  передача файлов: ${pct}% (${(loaded / 1e6).toFixed(0)} МБ)`);
    } else if (pct >= 100 && !doneNote) {
      doneNote = true;
      appendLog(logEl, "  файлы переданы, обработка…");
    }
  });
  return pollJob(data.jobId, logEl);
}

function setStatus(id, msg, ok) {
  const el = $(id);
  el.textContent = msg;
  el.className = "status " + (ok ? "ok" : "err");
  el.classList.remove("hidden");
}

function chips(el, items) {
  el.innerHTML = items.map(([label, cls]) => `<span class="tool-chip ${cls || ""}">${esc(label)}</span>`).join("");
}

// ==================== ВКЛАДКА: СВЕРКА ВЕРСИЙ ====================
let diffData = null;

$("diffRunBtn").addEventListener("click", async () => {
  const oldFiles = $("diffOldFiles").files, newFiles = $("diffNewFiles").files;
  const oldPath = $("diffOldPath").value.trim(), newPath = $("diffNewPath").value.trim();
  if (!oldFiles.length && !oldPath) { setStatus("diffStatus", "Укажите файлы или путь СТАРОЙ версии", false); return; }
  if (!newFiles.length && !newPath) { setStatus("diffStatus", "Укажите файлы или путь НОВОЙ версии", false); return; }
  const fd = new FormData();
  [...oldFiles].forEach((f) => fd.append("oldPbos", f, f.name));
  [...newFiles].forEach((f) => fd.append("newPbos", f, f.name));
  if (oldPath) fd.append("oldPath", oldPath);
  if (newPath) fd.append("newPath", newPath);
  $("diffRunBtn").disabled = true;
  $("diffResult").classList.add("hidden");
  try {
    diffData = await runMultipartJob("/api/diff", fd, $("diffLog"));
    renderDiff(diffData);
    setStatus("diffStatus", "Сверка завершена", true);
    $("diffReportBtn").classList.remove("hidden");
  } catch (e) {
    setStatus("diffStatus", "Ошибка: " + e.message, false);
  } finally {
    $("diffRunBtn").disabled = false;
  }
});

$("diffReportBtn").addEventListener("click", () => {
  if (diffData) downloadText("otchet_sverki.txt", diffData.report || "");
});

function briefTable(list) {
  let h = `<table style="width:100%"><thead><tr><th>Класснейм</th><th>Категория</th><th>Родитель</th><th>Название</th></tr></thead><tbody>`;
  list.forEach((c) => {
    h += `<tr><td class="cn">${esc(c.classname)}</td><td class="cat">${esc(c.category)}</td><td class="cat">${esc(c.parent)}</td><td class="cat">${esc(c.displayName || "")}</td></tr>`;
  });
  return h + "</tbody></table>";
}

function renderDiff(d) {
  chips($("diffChips"), [
    [`новых: ${(d.added || []).length}`, "ok"],
    [`удалённых: ${(d.removed || []).length}`, (d.removed || []).length ? "miss" : ""],
    [`изменённых: ${(d.changed || []).length}`, (d.changed || []).length ? "ok" : ""],
    [`без изменений: ${d.unchanged}`, ""],
  ]);
  const box = $("diffLists");
  let h = "";
  const det = (title, inner, open) => `<details ${open ? "open" : ""} style="margin-top:10px"><summary class="link-sum">${title}</summary><div class="table-wrap" style="max-height:340px;margin-top:8px">${inner}</div></details>`;
  if ((d.added || []).length) h += det(`Новые класснеймы (${d.added.length})`, briefTable(d.added), true);
  if ((d.removed || []).length) h += det(`Удалённые класснеймы (${d.removed.length}) — оверрайды по ним перестанут работать`, briefTable(d.removed), true);
  if ((d.changed || []).length) {
    let inner = "";
    d.changed.forEach((c) => {
      let rows = "";
      if (c.parentChanged) rows += `<tr><td class="cn">БАЗОВЫЙ КЛАСС</td><td class="old">${esc(c.oldParent)}</td><td class="edited">${esc(c.newParent)}</td></tr>`;
      (c.changes || []).forEach((ch) => {
        const oldV = ch.kind === "added" ? "—" : ch.old;
        const newV = ch.kind === "removed" ? "— (удалён)" : ch.new;
        rows += `<tr><td class="cn">${esc(ch.path)}</td><td class="old" style="text-decoration:none;color:#93a893">${esc(oldV)}</td><td class="edited">${esc(newV)}</td></tr>`;
      });
      inner += `<details style="margin:6px 0"><summary class="link-sum">[${esc(c.category)}] ${esc(c.classname)} — правок: ${(c.changes || []).length + (c.parentChanged ? 1 : 0)}</summary>
        <table style="width:100%;margin-top:6px"><thead><tr><th>Параметр</th><th>Было</th><th>Стало</th></tr></thead><tbody>${rows}</tbody></table></details>`;
    });
    h += det(`Изменённые классы (${d.changed.length})`, inner, true);
  }
  if ((d.warnings || []).length) {
    h += det(`Предупреждения (${d.warnings.length})`, "<ul><li>" + d.warnings.map(esc).join("</li><li>") + "</li></ul>", false);
  }
  box.innerHTML = h || "<p class='muted'>Различий не найдено — версии идентичны по классам.</p>";
  $("diffResult").classList.remove("hidden");
}

// ==================== ВКЛАДКА: TYPES ====================
let tmData = null, toData = null;

$("tmRunBtn").addEventListener("click", async () => {
  if (!$("tmMy").files.length || !$("tmNew").files.length) {
    setStatus("tmStatus", "Выберите оба файла: ваш и новый types.xml", false);
    return;
  }
  const fd = new FormData();
  fd.append("myTypes", $("tmMy").files[0]);
  fd.append("newTypes", $("tmNew").files[0]);
  fd.append("keepMissing", $("tmKeepMissing").checked ? "1" : "0");
  $("tmRunBtn").disabled = true;
  try {
    tmData = await runMultipartJob("/api/types/migrate", fd, $("tmLog"));
    chips($("tmChips"), [
      [`итоговых записей: ${tmData.total}`, "ok"],
      [`перенесено ваших: ${tmData.transferred}`, "ok"],
      [`новых класснеймов: ${(tmData.newNames || []).length}`, (tmData.newNames || []).length ? "miss" : ""],
      [`ваших вне нового: ${(tmData.missingKept || []).length}`, ""],
    ]);
    setStatus("tmStatus", "Готово. Скачайте объединённый файл и отчёт (в отчёте — список новых класснеймов).", true);
    $("tmDownloadBtn").classList.remove("hidden");
    $("tmReportBtn").classList.remove("hidden");
  } catch (e) {
    setStatus("tmStatus", "Ошибка: " + e.message, false);
  } finally {
    $("tmRunBtn").disabled = false;
  }
});
$("tmDownloadBtn").addEventListener("click", () => { if (tmData) downloadText("types.xml", tmData.mergedXml); });
$("tmReportBtn").addEventListener("click", () => { if (tmData) downloadText("otchet_migracii_types.txt", tmData.report); });

$("toRunBtn").addEventListener("click", async () => {
  if (!$("toFile").files.length) {
    setStatus("toStatus", "Выберите types.xml", false);
    return;
  }
  const fd = new FormData();
  fd.append("typesFile", $("toFile").files[0]);
  fd.append("simplify", $("toSimplify").checked ? "1" : "0");
  $("toRunBtn").disabled = true;
  try {
    toData = await runMultipartJob("/api/types/optimize", fd, $("toLog"));
    chips($("toChips"), [
      [`записей: ${toData.total}`, "ok"],
      [`исправлений: ${toData.fixes}`, toData.fixes ? "miss" : "ok"],
      [`дубликатов: ${toData.duplicates}`, ""],
      [`упрощено: ${toData.simplified}`, ""],
    ]);
    setStatus("toStatus", toData.fixes ? "Найдены и исправлены проблемы — подробности в отчёте." : "Файл корректен — выполнено только форматирование.", true);
    $("toDownloadBtn").classList.remove("hidden");
    $("toReportBtn").classList.remove("hidden");
  } catch (e) {
    setStatus("toStatus", "Ошибка: " + e.message, false);
  } finally {
    $("toRunBtn").disabled = false;
  }
});
$("toDownloadBtn").addEventListener("click", () => { if (toData) downloadText("types.xml", toData.fixedXml); });
$("toReportBtn").addEventListener("click", () => { if (toData) downloadText("otchet_proverki_types.txt", toData.report); });

// ==================== ВКЛАДКА: СЛИЯНИЕ ====================
let mrgData = null;

$("mrgFiles").addEventListener("change", () => {
  const ol = $("mrgList");
  ol.innerHTML = "";
  [...$("mrgFiles").files].forEach((f, i, arr) => {
    const li = document.createElement("li");
    li.textContent = f.name + (i === arr.length - 1 ? "  ← высший приоритет" : "");
    ol.appendChild(li);
  });
});

async function runMerge(outputPath) {
  const files = $("mrgFiles").files;
  if (files.length < 2) { setStatus("mrgStatus", "Выберите минимум два файла оверрайдов", false); return; }
  const name = sanitizeName($("mrgName").value.trim());
  $("mrgName").value = name;
  const fd = new FormData();
  [...files].forEach((f) => fd.append("overrides", f, f.name));
  fd.append("addonName", name);
  fd.append("makePbo", $("mrgMakePbo").checked ? "1" : "0");
  fd.append("sign", $("mrgSign").checked ? "1" : "0");
  fd.append("binarize", $("mrgBinarize").checked ? "1" : "0");
  fd.append("toolsPath", $("toolsPath").value.trim());
  fd.append("privateKeyPath", $("keyPath").value.trim());
  if (outputPath) fd.append("outputPath", outputPath);
  $("mrgRunBtn").disabled = true;
  $("mrgSaveBtn").disabled = true;
  try {
    mrgData = await runMultipartJob("/api/merge", fd, $("mrgLog"));
    renderMerge(mrgData);
    let msg = "Слияние завершено: " + (mrgData.files || []).length + " файлов";
    if (mrgData.savedTo) msg += ". Сохранено в: " + mrgData.savedTo;
    if (mrgData.saveError) msg += ". ОШИБКА сохранения: " + mrgData.saveError;
    const confl = (mrgData.conflicts || []).length;
    setStatus("mrgStatus", msg + (confl ? ` ⚠ Конфликтов: ${confl} (см. ниже)` : ". Конфликтов нет."), !mrgData.saveError);
    $("mrgZipBtn").classList.remove("hidden");
  } catch (e) {
    setStatus("mrgStatus", "Ошибка: " + e.message, false);
  } finally {
    $("mrgRunBtn").disabled = false;
    $("mrgSaveBtn").disabled = false;
  }
}

$("mrgRunBtn").addEventListener("click", () => runMerge(""));
$("mrgSaveBtn").addEventListener("click", () => {
  const p = $("mrgOutPath").value.trim();
  if (!p) { setStatus("mrgStatus", "Укажите папку для сохранения", false); return; }
  runMerge(p);
});
$("mrgZipBtn").addEventListener("click", () => {
  if (mrgData && mrgData.zipToken) window.location.href = "/api/zip/" + mrgData.zipToken;
});

function renderMerge(d) {
  chips($("mrgChips"), [
    [`слито аддонов: ${(d.merged || []).length}`, "ok"],
    [`requiredAddons: ${(d.required || []).length}`, "ok"],
    [`конфликтов: ${(d.conflicts || []).length}`, (d.conflicts || []).length ? "miss" : "ok"],
    [`предупреждений: ${(d.warnings || []).filter(Boolean).length}`, ""],
  ]);
  const cb = $("mrgConflicts");
  let h = "";
  if ((d.conflicts || []).length) {
    h += `<div class="status err" style="margin-top:8px">Конфликты (побеждает файл, выбранный позже):</div><ul>` +
      d.conflicts.map((c) => `<li>${esc(c)}</li>`).join("") + "</ul>";
  }
  const warns = (d.warnings || []).filter(Boolean);
  if (warns.length) {
    h += `<details style="margin-top:8px"><summary class="link-sum">Предупреждения (${warns.length})</summary><ul>` +
      warns.map((c) => `<li>${esc(c)}</li>`).join("") + "</ul></details>";
  }
  cb.innerHTML = h || "<p class='muted' style='margin-top:8px'>Конфликтов нет — оверрайды не пересекались или значения совпадали.</p>";
  $("mrgPreview").textContent = d.configCpp || "";
  $("mrgResult").classList.remove("hidden");
}

$("saveDirBtn").addEventListener("click", async () => {
  const p = $("outPath").value.trim();
  if (!p) { genStatus("Укажите папку для сохранения", false); return; }
  try {
    renderGenSummary();
    const data = await callGenerate({ outputPath: p });
    $("cfgPreview").textContent = data.configCpp;
    if (data.saveError) genStatus("Ошибка сохранения: " + data.saveError, false);
    else genStatus("Сохранено в: " + data.savedTo + warnSuffix(data), true);
  } catch (e) { genStatus("Ошибка: " + e.message, false); }
});

// ==================== ИИ-ПОМОЩНИК: НАСТРОЙКИ (кнопка 🤖) ====================
const AI_PRESETS_FALLBACK = [
  { id: "groq", name: "Groq — быстрый, до 1000+ запросов/день бесплатно", baseURL: "https://api.groq.com/openai/v1", model: "llama-3.3-70b-versatile", hint: "Ключ: console.groq.com/keys (регистрация, карта не нужна)." },
  { id: "openrouter", name: "OpenRouter — модели с пометкой :free", baseURL: "https://openrouter.ai/api/v1", model: "deepseek/deepseek-chat-v3.1:free", hint: "Ключ: openrouter.ai/settings/keys. Бесплатно ~50 запросов/день. Выбирайте модели с «:free»." },
  { id: "gemini", name: "Google Gemini (AI Studio) — щедрый бесплатный лимит", baseURL: "https://generativelanguage.googleapis.com/v1beta/openai", model: "gemini-2.5-flash", hint: "Ключ: aistudio.google.com/apikey (Google-аккаунт). Из некоторых стран API недоступен напрямую." },
  { id: "github", name: "GitHub Models — по GitHub-аккаунту", baseURL: "https://models.github.ai/inference", model: "openai/gpt-4o-mini", hint: "Ключ: github.com/settings/tokens → Fine-grained token с разрешением «Models: read»." },
  { id: "ollama", name: "Локальный Ollama — без интернета и ключей", baseURL: "http://localhost:11434/v1", model: "llama3.1", hint: "Нужен установленный Ollama (ollama.com) и скачанная модель. Ключ не требуется." },
  { id: "custom", name: "Свой сервер (OpenAI-совместимый API)", baseURL: "", model: "", hint: "Любой сервис с OpenAI-совместимым API: адрес, ключ и имя модели." },
];
let aiPresets = AI_PRESETS_FALLBACK;
fetch("/api/ai/presets").then((r) => r.json()).then((p) => { if (Array.isArray(p) && p.length) aiPresets = p; buildAiPresetSel(); }).catch(() => {});

function buildAiPresetSel() {
  const sel = $("aiPreset");
  sel.innerHTML = "";
  aiPresets.forEach((p) => {
    const o = document.createElement("option");
    o.value = p.id;
    o.textContent = p.name;
    sel.appendChild(o);
  });
}
buildAiPresetSel();

function aiPresetById(id) { return aiPresets.find((p) => p.id === id) || null; }

function aiUiRefresh() {
  // радио «через ИИ» на панелях гайдов
  document.querySelectorAll(".gmode-ai").forEach((r) => {
    r.disabled = !appSettings.aiEnabled;
    r.title = appSettings.aiEnabled ? "" : "ИИ-помощник выключен — включите его кнопкой 🤖 вверху";
    if (r.disabled && r.checked) {
      const st = r.closest(".guide-box");
      const s = st && st.querySelector(".gmode-struct");
      if (s) s.checked = true;
    }
  });
  document.querySelectorAll(".guide-ask-row").forEach((el) => {
    el.classList.toggle("hidden", !appSettings.aiEnabled);
  });
  $("aiCfgBtn").style.opacity = appSettings.aiEnabled ? "1" : "0.55";
}

function aiModalFill() {
  $("aiEnabled").checked = !!appSettings.aiEnabled;
  const preset = appSettings.aiPreset || "groq";
  $("aiPreset").value = preset;
  const p = aiPresetById(preset);
  $("aiPresetHint").textContent = p ? p.hint : "";
  $("aiBaseUrl").value = appSettings.aiBaseURL || (p ? p.baseURL : "");
  $("aiKey").value = appSettings.aiKey || "";
  $("aiModel").value = appSettings.aiModel || (p ? p.model : "");
  $("aiTestStatus").textContent = "";
  $("aiFields").style.opacity = $("aiEnabled").checked ? "1" : "0.5";
}

$("aiCfgBtn").addEventListener("click", () => {
  aiModalFill();
  $("aiBack").classList.remove("hidden");
});
$("aiCancel").addEventListener("click", () => $("aiBack").classList.add("hidden"));
$("aiBack").addEventListener("mousedown", (e) => { if (e.target === $("aiBack")) $("aiBack").classList.add("hidden"); });
$("aiEnabled").addEventListener("change", () => { $("aiFields").style.opacity = $("aiEnabled").checked ? "1" : "0.5"; });
$("aiPreset").addEventListener("change", () => {
  const p = aiPresetById($("aiPreset").value);
  if (!p) return;
  $("aiPresetHint").textContent = p.hint;
  $("aiBaseUrl").value = p.baseURL;
  $("aiModel").value = p.model;
  $("aiModelList").innerHTML = "";
  $("aiTestStatus").textContent = "";
});

async function aiFetchModels(silent) {
  const st = $("aiTestStatus");
  st.textContent = "запрашиваю список моделей…";
  try {
    const r = await fetch("/api/ai/test", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ baseURL: $("aiBaseUrl").value.trim(), apiKey: $("aiKey").value.trim(), model: $("aiModel").value.trim() }),
    });
    const d = await r.json();
    if (!r.ok) throw new Error(d.error || "ошибка");
    const dl = $("aiModelList");
    dl.innerHTML = "";
    (d.models || []).forEach((m) => {
      const o = document.createElement("option");
      o.value = m;
      dl.appendChild(o);
    });
    st.textContent = "✓ " + (d.message || "работает") + ((d.models || []).length ? ` Моделей: ${d.models.length} — начните печатать в поле «Модель», появятся подсказки.` : "");
    st.style.color = "var(--ok, #7fbf5f)";
    return d;
  } catch (e) {
    st.textContent = "✗ " + e.message;
    st.style.color = "var(--err, #e07b7b)";
    if (!silent) return null;
    return null;
  }
}
$("aiModelsBtn").addEventListener("click", () => aiFetchModels(false));
$("aiTestBtn").addEventListener("click", () => aiFetchModels(false));

$("aiSave").addEventListener("click", async () => {
  const body = {
    aiEnabled: $("aiEnabled").checked,
    aiPreset: $("aiPreset").value,
    aiBaseURL: $("aiBaseUrl").value.trim(),
    aiKey: $("aiKey").value.trim(),
    aiModel: $("aiModel").value.trim(),
  };
  if (body.aiEnabled && (!body.aiBaseURL || !body.aiModel)) {
    alert("Заполните адрес API и модель (или выключите ИИ-помощника)");
    return;
  }
  try {
    const r = await fetch("/api/settings", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!r.ok) throw new Error((await r.json()).error || "ошибка");
    Object.assign(appSettings, body);
    aiUiRefresh();
    $("aiBack").classList.add("hidden");
  } catch (e) {
    alert("Не удалось сохранить настройки: " + e.message);
  }
});

// ==================== ПАНЕЛЬ «ГАЙД ПО ССЫЛКЕ» ====================
const guideState = {}; // prefix -> { url, matches: {id:{d,ai}}, list }

const GUIDE_DEFAULTS = {
  ind: "https://github.com/ysaroka/InediaInfectedAI/wiki/Description",
  trj: "https://github.com/TerjeBruoygard/TerjeMods/tree/master/WIKI/ru",
  expq: "https://github.com/salutesh/DayZ-Expansion-Scripts/wiki",
  expl: "https://github.com/salutesh/DayZ-Expansion-Scripts/wiki",
};

function guidePanelHTML(prefix, hasForm) {
  const modes = hasForm
    ? `<label class="chk" style="white-space:nowrap"><input type="radio" name="${prefix}gmode" class="gmode-struct" value="struct" checked> структурный разбор</label>
       <label class="chk" style="white-space:nowrap"><input type="radio" name="${prefix}gmode" class="gmode-ai" value="ai"> через ИИ (перевод на русский)</label>`
    : "";
  return `<details class="card guide-box" id="${prefix}GuideBox" style="margin-top:10px">
    <summary class="link-sum" style="font-size:15px"><b>📖 Гайд по ссылке</b> — описания и ответы из гайда мода</summary>
    <p class="hint" style="margin:8px 0 10px">${hasForm
      ? "Вставьте ссылку на гайд (github wiki, страница или папка WIKI) — программа скачает его и подставит описания к параметрам открытого файла. Режим «через ИИ» дополнительно переводит описания на русский и понимает свободный текст (настройка — кнопка 🤖 вверху). Гайд кэшируется: повторная загрузка не тратит интернет."
      : "Вставьте ссылку на гайд (github wiki или страница) — программа скачает его, покажет справочник «параметр → описание» и ответит на вопросы по гайду через ИИ (настройка — кнопка 🤖 вверху). Гайд кэшируется."}</p>
    <div class="row">
      <input type="text" id="${prefix}GuideUrl" class="grow" placeholder="https://github.com/автор/мод/wiki/Страница">
      ${modes}
      <button class="btn primary" id="${prefix}GuideBtn">Загрузить гайд</button>
      <button class="btn" id="${prefix}GuideRefreshBtn" title="Скачать заново, минуя кэш">⟳</button>
    </div>
    <pre id="${prefix}GuideLog" class="joblog hidden" style="margin-top:8px"></pre>
    <div id="${prefix}GuideStatus" class="status hidden"></div>
    <details id="${prefix}GuideListBox" class="hidden" style="margin-top:8px"><summary class="link-sum">Извлечённый справочник (параметр → описание)</summary>
      <div class="row" style="margin:8px 0 4px"><input type="text" id="${prefix}GuideFilter" class="grow" placeholder="Фильтр по справочнику…"></div>
      <div id="${prefix}GuideList" style="max-height:340px;overflow:auto"></div>
    </details>
    <div class="row guide-ask-row hidden" id="${prefix}GuideAskRow" style="margin-top:10px">
      <input type="text" id="${prefix}GuideQ" class="grow" placeholder="Вопрос по гайду, например: как сделать зомби медленнее?">
      <button class="btn" id="${prefix}GuideAskBtn">🤖 Спросить</button>
    </div>
    <div id="${prefix}GuideAnswer" class="hidden" style="margin-top:8px;white-space:pre-wrap;background:rgba(255,255,255,.04);border-radius:8px;padding:10px 12px;font-size:13px"></div>
  </details>`;
}

function guideCollectFields(prefix) {
  const st = modcfgState[prefix];
  if (!st || !st.model) return [];
  const out = [];
  (st.model.groups || []).forEach((g) => (g.fields || []).forEach((f) => out.push({ id: f.id, path: f.label })));
  return out;
}

// подставить описания в уже открытую форму, не трогая введённые значения
function guideApplyDescs(prefix) {
  const gm = (guideState[prefix] && guideState[prefix].matches) || {};
  document.querySelectorAll("#" + prefix + "Form .mc-row").forEach((row) => {
    const inp = row.querySelector(".msn-val");
    if (!inp) return;
    const gv = gm[inp.dataset.id];
    const old = row.querySelector(".gdesc");
    if (old) old.remove();
    if (!gv) return;
    const div = document.createElement("div");
    div.className = "muted gdesc";
    div.style.fontSize = "12px";
    div.textContent = (gv.ai ? "🤖 " : "📖 ") + gv.d;
    row.appendChild(div);
    row.dataset.key = (row.dataset.key || "") + " " + gv.d.toLowerCase();
  });
}

function guideRenderList(prefix) {
  const st = guideState[prefix];
  const box = $(prefix + "GuideList");
  const q = ($(prefix + "GuideFilter").value || "").trim().toLowerCase();
  if (!st || !st.list || !st.list.length) { box.innerHTML = ""; return; }
  let shown = 0;
  let h = "";
  st.list.forEach((kv) => {
    if (q && !(kv.k + " " + kv.d).toLowerCase().includes(q)) return;
    shown++;
    if (shown > 400) return;
    h += `<div style="margin-bottom:8px"><span class="cn" style="font-family:var(--mono)">${esc(kv.k)}</span><div class="muted" style="font-size:12px">${esc(kv.d)}</div></div>`;
  });
  box.innerHTML = h || "<p class='muted'>ничего не найдено</p>";
}

function wireGuidePanel(prefix, hasForm) {
  const mount = $(prefix + "GuideMount");
  if (!mount) return;
  mount.innerHTML = guidePanelHTML(prefix, hasForm);
  $(prefix + "GuideUrl").value = GUIDE_DEFAULTS[prefix] || "";
  $(prefix + "GuideFilter").addEventListener("input", () => guideRenderList(prefix));

  async function load(refresh) {
    const url = $(prefix + "GuideUrl").value.trim();
    if (!url) { setStatus(prefix + "GuideStatus", "Вставьте ссылку на гайд", false); return; }
    const modeEl = document.querySelector(`input[name="${prefix}gmode"]:checked`);
    const mode = hasForm && modeEl ? modeEl.value : "struct";
    const fields = hasForm ? guideCollectFields(prefix) : [];
    if (mode === "ai" && !fields.length) {
      setStatus(prefix + "GuideStatus", "Сначала откройте файл конфига — ИИ подставляет описания к его параметрам", false);
      return;
    }
    const log = $(prefix + "GuideLog");
    log.textContent = "";
    log.classList.remove("hidden");
    $(prefix + "GuideBtn").disabled = true;
    $(prefix + "GuideRefreshBtn").disabled = true;
    try {
      const r = await fetch("/api/guide/load", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ url, mode, refresh: !!refresh, fields }),
      });
      const d0 = await r.json();
      if (!r.ok) throw new Error(d0.error || "ошибка");
      const d = await pollJob(d0.jobId, log);
      guideState[prefix] = { url, matches: d.matches || {}, list: d.list || [] };
      guideRenderList(prefix);
      $(prefix + "GuideListBox").classList.toggle("hidden", !(d.list || []).length);
      if (hasForm && modcfgState[prefix] && modcfgState[prefix].model) guideApplyDescs(prefix);
      let msg = (d.fromCache ? "Гайд из кэша (скачан " + d.fetchedAt + ")" : "Гайд скачан") +
        ". Пар в справочнике: " + d.kvCount;
      if (hasForm) {
        msg += d.fieldCount
          ? `. Подставлено описаний к параметрам: ${d.matchCount} из ${d.fieldCount}` + (d.aiCount ? ` (через ИИ: ${d.aiCount})` : "")
          : ". Откройте файл конфига и загрузите гайд ещё раз — описания подставятся к параметрам.";
        if (d.fieldCount && !d.matchCount && !d.aiCount) {
          msg += " Похоже, гайд написан свободным текстом без имён параметров — включите режим «через ИИ» (кнопка 🤖 вверху) или задайте вопрос по гайду.";
        }
      }
      const ok = !(d.warnings || []).length;
      setStatus(prefix + "GuideStatus", msg + ((d.warnings || []).length ? " ⚠ " + d.warnings.join(" ") : ""), ok);
    } catch (e) {
      setStatus(prefix + "GuideStatus", "Ошибка: " + e.message, false);
    } finally {
      $(prefix + "GuideBtn").disabled = false;
      $(prefix + "GuideRefreshBtn").disabled = false;
    }
  }
  $(prefix + "GuideBtn").addEventListener("click", () => load(false));
  $(prefix + "GuideRefreshBtn").addEventListener("click", () => load(true));

  $(prefix + "GuideAskBtn").addEventListener("click", async () => {
    const url = $(prefix + "GuideUrl").value.trim();
    const q = $(prefix + "GuideQ").value.trim();
    if (!url || !q) { setStatus(prefix + "GuideStatus", "Нужны ссылка на гайд и текст вопроса", false); return; }
    const log = $(prefix + "GuideLog");
    log.textContent = "";
    log.classList.remove("hidden");
    $(prefix + "GuideAskBtn").disabled = true;
    const ans = $(prefix + "GuideAnswer");
    ans.classList.add("hidden");
    try {
      const r = await fetch("/api/guide/ask", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ url, question: q }),
      });
      const d0 = await r.json();
      if (!r.ok) throw new Error(d0.error || "ошибка");
      const d = await pollJob(d0.jobId, log);
      ans.textContent = "🤖 " + (d.answer || "(пустой ответ)") + "\n\n⚠ ИИ может ошибаться — сверьтесь с гайдом.";
      ans.classList.remove("hidden");
    } catch (e) {
      setStatus(prefix + "GuideStatus", "Ошибка: " + e.message, false);
    } finally {
      $(prefix + "GuideAskBtn").disabled = false;
    }
  });
}

wireGuidePanel("ind", true);
wireGuidePanel("trj", true);
wireGuidePanel("expq", false);
wireGuidePanel("expl", false);
