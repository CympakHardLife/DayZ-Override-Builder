// DayZ Override Builder — создание рекваер-аддонов (override-модов) для DayZ.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"dayzoverride/analyze"
)

var version = "1.11.0"

func main() {
	analyzeFlag := flag.String("analyze", "", "CLI: проанализировать PBO и вывести сводку")
	dumpFlag := flag.String("dumpcfg", "", "CLI: сохранить дебинаризованный config.cpp в файл")
	jsonFlag := flag.Bool("json", false, "CLI: вывести результат анализа в JSON")
	port := flag.Int("port", 8017, "порт локального веб-интерфейса")
	noOpen := flag.Bool("no-open", false, "не открывать браузер автоматически")
	flag.Parse()

	if *analyzeFlag != "" {
		runCLI(*analyzeFlag, *dumpFlag, *jsonFlag)
		return
	}
	runServer(*port, *noOpen)
}

func runCLI(path, dump string, asJSON bool) {
	logf := func(format string, args ...interface{}) {
		fmt.Fprintf(os.Stderr, "[*] "+format+"\n", args...)
	}
	res, err := analyze.AnalyzePBOs([]string{path}, logf)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка:", err)
		os.Exit(1)
	}
	if dump != "" {
		for name, txt := range res.ConfigCPP {
			if err := os.WriteFile(dump, []byte(txt), 0644); err != nil {
				fmt.Fprintln(os.Stderr, "Ошибка записи:", err)
				os.Exit(1)
			}
			fmt.Printf("config из %s сохранён в %s\n", name, dump)
			break
		}
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(res)
		return
	}
	for _, p := range res.Pbos {
		fmt.Printf("PBO: %s | prefix=%s | файлов=%d | %s\n", p.FileName, p.Prefix, p.NumFiles, p.ConfigSrc)
	}
	for _, pt := range res.Patches {
		fmt.Printf("CfgPatches: %s | requiredAddons=%v\n", pt.Name, pt.RequiredAddons)
	}
	for _, w := range res.Warnings {
		fmt.Println("ПРЕДУПРЕЖДЕНИЕ:", w)
	}
	fmt.Printf("Классов найдено: %d\n", len(res.Items))
	for _, it := range res.Items {
		fmt.Printf("- [%s] %s : %s (scope=%s)\n", it.Category, it.Classname, it.Parent, it.Scope.Display)
		for _, def := range analyze.Registry {
			if pv, ok := it.Params[def.Key]; ok && pv.Exists {
				src := ""
				if pv.Source != "" {
					src = " (насл. от " + pv.Source + ")"
				}
				fmt.Printf("    %s = %s%s\n", def.Label, pv.Display, src)
			}
		}
	}
}
