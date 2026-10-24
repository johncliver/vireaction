package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func leerArchivoComoSet(ruta string) (map[string]bool, error) {
	set := make(map[string]bool)
	file, err := os.Open(ruta)
	if os.IsNotExist(err) {
		return set, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		linea := strings.TrimSpace(scanner.Text())
		if linea != "" {
			set[linea] = true
		}
	}
	return set, scanner.Err()
}

func anexarLineaArchivo(ruta, linea string) error {
	f, err := os.OpenFile(ruta, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(linea + "\n")
	return err
}

func guardarURLsEnArchivo(urls []string, ruta string) error {
	f, err := os.Create(ruta)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, u := range urls {
		if _, err := w.WriteString(u + "\n"); err != nil {
			return err
		}
	}
	return w.Flush()
}

func archivoMasReciente(dir string) string {
	var best string
	var bestMod time.Time
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(bestMod) {
			best = filepath.Join(dir, e.Name())
			bestMod = info.ModTime()
		}
	}
	return best
}

func absPath(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}
