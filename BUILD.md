# Build from the published source / Сборка из исходников

The `source/` directory is a flat source archive: files from different Go packages
and embedded web assets were uploaded into one folder. Running `go build` there
directly fails. The helper restores the package folders without changing the originals.

В `source/` файлы разных Go-пакетов выложены в одну папку. Перед сборкой восстановите
структуру командой ниже. Исходные файлы сохраняются; результат — `build/source/`.

Requirements / Нужно: Python 3.8+ and Go 1.21+ on Windows.

```powershell
python prepare_source.py
cd build/source
go test ./...
go build -o DayZOverrideBuilder.exe .
```

The generated executable is `build/source/DayZOverrideBuilder.exe`.
To prepare a fresh copy, rename the existing `build/source/` folder first;
the helper refuses to overwrite an earlier build or local edits.

Готовый EXE будет в `build/source/DayZOverrideBuilder.exe`. Для повторной подготовки
переименуйте предыдущую папку `build/source/`: скрипт не перезаписывает правки.

Source version: **1.11.0** (`source/main.go`). The existing repository executable
is distributed separately; this documentation change does not rebuild or certify it.
