<div align="center">

# 🧟 DayZ Override Builder

### Build require-addons for DayZ mods — one `.exe`, no installation

**English** · [Русский](README.ru.md)

[![License: MIT](https://img.shields.io/badge/License-MIT-22c55e.svg?style=for-the-badge)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/Platform-Windows-0078D6?style=for-the-badge&logo=windows&logoColor=white)]()
[![Release](https://img.shields.io/badge/Release-v1.10.2-8b5cf6?style=for-the-badge)](../../releases)
[![UI](https://img.shields.io/badge/UI-RU%20%2F%20EN%20%2F%20UK-f59e0b?style=for-the-badge)]()

A toolbox for **DayZ server owners and modders**.
Unpack a mod's PBO, pick the items you want to change, and get a ready **require-addon (override)**
that survives mod updates. Plus `types.xml` migration, override merging, Expansion quests and
loadouts, spawn loot, mission tuning, and a strict JSON/XML validator.

Runs locally in your browser — no cloud, no account, no installation.

</div>

---

## ✨ Highlights

<table>
<tr>
<td width="33%" valign="top">

### 📦 One file
`DayZOverrideBuilder.exe` — no installer, no runtime.
Drop it anywhere and run it.

</td>
<td width="33%" valign="top">

### 🔒 Works offline
Everything runs on your machine.
Internet is only needed for the optional
"Guide by URL" and AI helper.

</td>
<td width="33%" valign="top">

### 🌍 RU / EN / UK
Full interface translation,
switchable at any moment.

</td>
</tr>
</table>

---

## 🚀 Quick Start

```text
1. Put DayZOverrideBuilder.exe in any folder and run it.
2. A black console window opens — DO NOT close it, that is the program.
   A browser tab opens with the interface.
3. If the tab did not open, use the address from the console
   window — usually  http://127.0.0.1:8017
4. SmartScreen may warn about a new file:
   "More info" → "Run anyway"  (normal for unsigned apps).
```

> ⚠️ **The program can make mistakes.** Always keep backups, check the produced files
> before uploading them to a live server, and read the **Log** tab — every action is recorded there.

---

## 🧩 Tabs & Features

| Tab | What it does |
|---|---|
| **1 · Mod** | Analyze a mod PBO or a whole `@ModFolder` — `config.bin` is unpacked and de-binarized automatically |
| **2 · Items** | Pick the classes and parameters you want to override |
| **3 · Generate** | Build the addon + DayZ Tools settings: signing, keys, binarization |
| **⇄ Version diff** | Compare an old and a new version of a mod to see what the update changed |
| **T Types** | `types.xml` migration and optimization |
| **⊕ Merge** | Merge several overrides into a single addon |
| **❓ Exp. Quests** | DayZ Expansion — quest validation and merging |
| **🎒 Exp. Loadouts** | DayZ Expansion — loadout validation and merging |
| **📦 Spawn loot** | Spawn loot configuration |
| **🌍 Mission** | Mission settings + day/night length calculator |
| **🧟 Inedia AI** | Config editor for the Inedia AI mod |
| **💊 Terje** | Config editor for the Terje mods |
| **✓ File check** | Strict JSON/XML validation with line numbers and hints |
| **☰ Log** | Every action and error the program performed |

---

## ✅ File Check — What It Catches

The validator is deliberately as strict as online tools (`codebeautify`, `jsonlint`),
because DayZ silently refuses files that "look" valid.

<details>
<summary><b>XML — structural errors</b></summary>

<br>

- Junk text **inside** the file, between tags (e.g. a stray `1231` after `</type>`) —
  formally still valid XML, but broken for the game
- Junk before or after the root tag, several root tags
- Duplicate attributes in one tag
- Unclosed and mismatched tags
- Bare `&` characters and broken entities

</details>

<details>
<summary><b>types.xml — DayZ economy rules (with the entry's line number)</b></summary>

<br>

- `nominal` / `min` / `lifetime` / `restock` / `cost` — integers, not negative
- `nominal` must not be lower than `min`; `lifetime` must not be `0`
- `quantmin` / `quantmax` — either both `-1`, or both `0..100` with `min <= max`
- `count_in_*` / `crafted` / `deloot` flags — only `0` or `1`
- Typo detection in `category` / `usage` / `value` (tier) / `tag` names
- Unknown or misspelled tags inside `<type>`
- `<type>` entries without `name`, and duplicate `name` values

</details>

<details>
<summary><b>cfgspawnabletypes.xml and cfgrandompresets.xml</b></summary>

<br>

- `chance` must be a number in `0.0 .. 1.0`
- Entries must have a `name`

</details>

<details>
<summary><b>JSON</b></summary>

<br>

Trailing commas, single quotes, comments, missing commas, truncated files,
junk after the end of the JSON, duplicate keys.

</details>

**Errors vs. warnings.** Broken format and invalid values are reported as **errors**
(the file is broken). Questionable economy (`nominal < min` and similar) and unfamiliar
names coming from mods are reported as **warnings** (it loads, but is worth a look).

---

## 📖 Guide by URL (optional)

Paste a link to a mod's guide — a GitHub wiki, a page, or a WIKI folder — and the program
downloads it and attaches the descriptions to the parameters of the config you have open.
Guides are cached, so repeat loads work without internet.

Available on the **Inedia AI**, **Terje**, **Exp. Quests** and **Exp. Loadouts** tabs.

| Mode | What it gives |
|---|---|
| **Structural parsing** | Free forever, no registration, no keys |
| **Via AI** | Translates descriptions into your language, understands free-form guides, and adds an **Ask** button for questions in your own words |

<details>
<summary><b>Enabling the AI helper</b></summary>

<br>

Press the **🤖** button in the top bar. It works with **free** tiers:
**Groq**, **OpenRouter** (`:free` models), **Google Gemini**, **GitHub Models**,
local **Ollama**, or any OpenAI-compatible server.

You only need an API key — it takes a couple of minutes to get and no bank card is required.

**Privacy:** the key is stored only on your machine in `settings.json`. Only the guide text
and parameter *names* are sent to the AI — **the values from your configs are never sent.**

The AI can be switched off at any moment with the same button; the program is fully
functional without it.

</details>

---

## 🆕 What's New

<details open>
<summary><b>v1.10.2 — the File Check tab now understands DayZ</b></summary>

<br>

- Catches junk text **inside** the file, between tags — previously skipped because the XML
  was formally valid, even though the game rejects it
- Added semantic `types.xml` validation against DayZ economy rules, with line numbers
- `cfgspawnabletypes.xml` and `cfgrandompresets.xml`: `chance` range and required `name`
- Split results into **errors** (file is broken) and **warnings** (loads, but worth checking)

</details>

<details>
<summary><b>v1.10.1 — strict validation</b></summary>

<br>

- XML checking used to miss real errors and report "everything is fine". It is now strict,
  matching online validators
- JSON validation stays strict, matching `jsonlint`
- Added a file format selector (`types.xml`, `cfgspawnabletypes.xml`, `cfgrandompresets.xml`,
  JSON, `config.cpp`/text). **Auto-detect** is usually enough — the selector helps when the file
  has no extension or you want to confirm it really is that format

</details>

<details>
<summary><b>v1.10.0 — Guide by URL</b></summary>

<br>

Download a mod's guide by link and attach parameter descriptions automatically, with an
optional AI mode for translation and free-form guides. See the section above.

</details>

---

## 🔬 Technical Details

- **Go 1.21**, a single static binary with an embedded web interface — no CGO, no DLLs.
- **Local web UI** on `127.0.0.1:8017`, driven from your browser.
- **PBO / config.bin** — own unpacker and de-binarizer (`rap` — reads and writes raP binary configs).
- **Own modules** for types.xml (`typesx`), override merging (`mergecfg`), version diff (`diffcfg`),
  Expansion quests and loadouts, missions, spawn loot, EDDS textures, and validation (`validator`).
- **Covered by Go tests** — `pbo`, `typesx`, `mergecfg`, `diffcfg`, `guide`, `tools`, `validator`.

---

## 👤 Author

<div align="center">

### **Cympak** — [@CympakHardLife](https://github.com/CympakHardLife)

*"I'm not a master at anything, I'm a simple person without education, I do what I like."*

**This program was written by me with the help of Fable 5.**

</div>

---

## 📄 License

Released under the **MIT License** — see [LICENSE](LICENSE).

You may freely use, copy, modify, merge, publish, distribute and even sell this software,
including in commercial projects. The only requirement is that the copyright notice and this
license text remain in copies. The software is provided **"as is"**, without warranty of any kind.

```text
Copyright (c) 2026 Cympak (CympakHardLife)
```

> This is an unofficial community tool. **DayZ** is a trademark of Bohemia Interactive a.s.
> This project is not affiliated with or endorsed by Bohemia Interactive.

---

<div align="center">

⭐ **If the tool saves you time, consider starring the repository.**

</div>
