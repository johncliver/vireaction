package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// ==========================================================
// FLUJOS DE DESCARGA
// ==========================================================
func flujoDescargaCompleto() {
	fmt.Println("\n🔍 Extrayendo URLs de Reels con Rod...")
	urls, err := extraerURLsReels()
	if err != nil {
		log.Printf("⚠️  No se pudieron extraer URLs: %v", err)
		log.Println("   Continuando sólo con los videos que ya estén en la carpeta...")
		urls = nil
	} else {
		fmt.Printf("📋 Se encontraron %d enlaces de Reels.\n", len(urls))
		if err := guardarURLsEnArchivo(urls, archivoURLs); err != nil {
			log.Printf("⚠️  No se pudo guardar %s: %v", archivoURLs, err)
		} else {
			fmt.Printf("💾 Lista de URLs guardada en %s\n", archivoURLs)
		}
	}

	for iteracion := 1; ; iteracion++ {
		registro, err := leerArchivoComoSet(archivoRegistro)
		if err != nil {
			log.Fatalf("Error al leer el registro: %v", err)
		}
		var nuevas []string
		for _, u := range urls {
			if !registro[u] {
				nuevas = append(nuevas, u)
			}
		}
		if len(nuevas) == 0 {
			fmt.Println("\n✨ No hay videos nuevos para descargar.")
			break
		}
		if len(nuevas) > maxVideosPorEjecucion {
			nuevas = nuevas[:maxVideosPorEjecucion]
		}
		fmt.Printf("\n═══════════════════════════════════════════════\n")
		fmt.Printf("📥 Iteración %d: descargando %d videos nuevos\n", iteracion, len(nuevas))
		fmt.Printf("═══════════════════════════════════════════════\n")

		exitosos := 0
		for _, u := range nuevas {
			if err := descargarVideoConYtDlp(u, InputFolder, false); err != nil {
				log.Printf("❌ Error al descargar %s: %v", u, err)
				continue
			}
			_ = anexarLineaArchivo(archivoRegistro, u)
			exitosos++
		}
		procesarTodosLosVideos()
		if exitosos == 0 {
			fmt.Println("⚠️  No se descargó ningún video nuevo. Saliendo del bucle.")
			break
		}
		if !preguntarSiNo("¿Descargar otro lote de videos de Facebook?") {
			fmt.Println("⏭️  Deteniendo descargas.")
			break
		}
	}
}

func flujoDescargaDesdeLista() {
	urls, err := leerURLsDesdeArchivo(archivoURLs)
	if err != nil {
		fmt.Printf("\n⚠️  No se pudo leer '%s': %v\n", archivoURLs, err)
		return
	}
	if len(urls) == 0 {
		fmt.Printf("\n⚠️  No hay URLs en '%s'.\n", archivoURLs)
		fmt.Println("   Primero usa la opción 1 para extraer URLs de Facebook,")
		fmt.Println("   o edita el archivo a mano (una URL por línea) y reintenta.")
		return
	}
	fmt.Printf("\n📋 Se encontraron %d URLs en '%s'.\n", len(urls), archivoURLs)

	var force bool
	var candidatas []string

	if preguntarSiNo("¿Re-descargar TODAS las URLs (aunque ya estén en descargados.txt)?") {
		force = true
		candidatas = urls
		fmt.Printf("🔁 Modo: re-descargar TODAS (%d URLs)\n", len(candidatas))
	} else {
		registro, _ := leerArchivoComoSet(archivoRegistro)
		for _, u := range urls {
			if !registro[u] {
				candidatas = append(candidatas, u)
			}
		}
		if len(candidatas) == 0 {
			fmt.Println("\n✨ Todas las URLs ya están descargadas. Nada que hacer.")
			fmt.Println("   (Si quieres re-descargarlas, responde 'S' en la pregunta anterior.)")
			return
		}
		fmt.Printf("📥 Pendientes por descargar: %d de %d\n", len(candidatas), len(urls))
	}

	for iteracion := 1; ; iteracion++ {
		if len(candidatas) == 0 {
			fmt.Println("\n✨ Lista terminada.")
			break
		}
		lote := candidatas
		if len(lote) > maxVideosPorEjecucion {
			lote = lote[:maxVideosPorEjecucion]
		}
		candidatas = candidatas[len(lote):]

		fmt.Printf("\n═══════════════════════════════════════════════\n")
		fmt.Printf("📥 Iteración %d: descargando %d videos (desde lista)\n", iteracion, len(lote))
		fmt.Printf("═══════════════════════════════════════════════\n")

		exitosos := 0
		for _, u := range lote {
			if err := descargarVideoConYtDlp(u, InputFolder, force); err != nil {
				log.Printf("❌ Error al descargar %s: %v", u, err)
				continue
			}
			_ = anexarLineaArchivo(archivoRegistro, u)
			exitosos++
		}
		procesarTodosLosVideos()

		if exitosos == 0 {
			fmt.Println("⚠️  No se descargó ningún video. Saliendo.")
			break
		}
		if len(candidatas) == 0 {
			fmt.Println("\n✨ Lista terminada.")
			break
		}
		if !preguntarSiNo("¿Descargar otro lote desde la lista?") {
			fmt.Println("⏭️  Deteniendo descargas.")
			break
		}
	}
}

// ==========================================================
// LECTURA / ESCRITURA DE URLS
// ==========================================================
func leerURLsDesdeArchivo(ruta string) ([]string, error) {
	file, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var urls []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		l := strings.TrimSpace(scanner.Text())
		if l != "" && !strings.HasPrefix(l, "#") {
			urls = append(urls, l)
		}
	}
	return urls, scanner.Err()
}

// ==========================================================
// BRAVE
// ==========================================================
func obtenerRutaBrave() string {
	switch runtime.GOOS {
	case "windows":
		return `C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`
	case "darwin":
		return "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
	default:
		return "/usr/bin/brave-browser"
	}
}

func obtenerUserDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("No se pudo obtener el directorio home: %v", err)
	}
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "BraveSoftware", "Brave-Browser", "User Data")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser")
	default:
		return filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser")
	}
}

// ==========================================================
// EXTRACCIÓN CON ROD
// ==========================================================
func extraerURLsReels() ([]string, error) {
	l := launcher.New().
		Bin(obtenerRutaBrave()).
		UserDataDir(obtenerUserDataDir()).
		Headless(true).
		NoSandbox(true).
		Set("disable-blink-features", "AutomationControlled").
		Set("excludeSwitches", "enable-automation").
		Set("useAutomationExtension", "false")
	url := l.MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage("").Timeout(60 * time.Second)
	page.MustNavigate(profileURL)
	page.MustWaitLoad()

	info, _ := page.Info()
	fmt.Printf("   URL actual: %s\n", info.URL)
	if strings.Contains(info.URL, "/login") {
		return nil, fmt.Errorf("Facebook redirigió a login. Inicia sesión manualmente y reintenta")
	}
	for i := 0; i < 15; i++ {
		res, _ := page.Eval(`() => document.querySelectorAll('a[href*="/reel/"]').length`)
		if res != nil && res.Value.Int() > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	for i := 0; i < 10; i++ {
		page.MustEval(`() => window.scrollTo(0, document.body.scrollHeight)`)
		time.Sleep(1500 * time.Millisecond)
	}

	js := `() => {
		const all = Array.from(document.querySelectorAll('a[href*="/reel/"]'));
		const seen = new Set(); const fromProfileGrid = []; const others = [];
		for (const a of all) {
			const href = a.href;
			if (seen.has(href)) continue; seen.add(href);
			const label = (a.getAttribute('aria-label') || '').toLowerCase();
			const isProfileReel =
				href.includes('fb_shorts_profile') || href.includes('fb_shorts_tab') ||
				href.includes('stack_idx') || label.includes('vista previa del icono del reel') ||
				label.includes('reel preview');
			if (isProfileReel) fromProfileGrid.push(href); else others.push(href);
		}
		return { fromProfileGrid, others };
	}`
	res, err := page.Eval(js)
	if err != nil {
		return nil, fmt.Errorf("error al extraer URLs: %w", err)
	}
	var urlsDelPerfil, urlsOtros []string
	for _, v := range res.Value.Get("fromProfileGrid").Arr() {
		urlsDelPerfil = append(urlsDelPerfil, v.Str())
	}
	for _, v := range res.Value.Get("others").Arr() {
		urlsOtros = append(urlsOtros, v.Str())
	}
	_ = guardarURLsDebug("reels_urls_debug.txt", urlsDelPerfil, urlsOtros)
	fmt.Printf("   ├─ Reels DEL PERFIL (grid): %d\n", len(urlsDelPerfil))
	fmt.Printf("   └─ Otros reels ignorados:   %d\n", len(urlsOtros))
	if len(urlsDelPerfil) == 0 {
		page.MustScreenshot("debug_screenshot.png")
		if html, err := page.HTML(); err == nil {
			_ = os.WriteFile("debug_page.html", []byte(html), 0644)
		}
		return nil, fmt.Errorf("no se encontraron reels del perfil")
	}
	for i, u := range urlsDelPerfil {
		if strings.HasPrefix(u, "/") {
			urlsDelPerfil[i] = "https://www.facebook.com" + u
		}
	}
	return urlsDelPerfil, nil
}

func guardarURLsDebug(ruta string, delPerfil, otros []string) error {
	f, err := os.Create(ruta)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	fmt.Fprintf(w, "# REELS DEL PERFIL (%d)\n", len(delPerfil))
	for _, u := range delPerfil {
		fmt.Fprintln(w, u)
	}
	fmt.Fprintf(w, "\n# OTROS REELS (%d)\n", len(otros))
	for _, u := range otros {
		fmt.Fprintln(w, u)
	}
	return nil
}

// ==========================================================
// DESCARGA CON YT-DLP
// ==========================================================
func descargarVideoConYtDlp(url, carpetaSalida string, force bool) error {
	if err := os.MkdirAll(carpetaSalida, os.ModePerm); err != nil {
		return fmt.Errorf("no se pudo crear '%s': %w", carpetaSalida, err)
	}

	antes := snapshotArchivos(carpetaSalida)

	selectores := []string{
		"bv*+ba/b",
		"b[acodec!=none]/b",
		"best[acodec!=none]/best",
		"best",
	}

	var ultimoDescargado string
	for i, sel := range selectores {
		if i > 0 {
			eliminarNuevos(carpetaSalida, antes)
			if force && ultimoDescargado != "" {
				_ = os.Remove(ultimoDescargado)
			}
			ultimoDescargado = ""
		}

		fmt.Printf("▶️  Descargando [%d/%d selector=%s]\n   %s\n",
			i+1, len(selectores), sel, url)

		args := []string{
			url,
			"-f", sel,
			"--merge-output-format", "mp4",
			"-o", filepath.Join(carpetaSalida, "%(title)s.%(ext)s"),
			"--no-warnings",
			"--no-part",
		}
		if force {
			args = append(args, "--force-overwrites")
		}

		cmd := exec.Command("yt-dlp", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Printf("   ⚠️  Selector %d falló, probando siguiente...\n", i+1)
			continue
		}

		nuevos := archivosNuevos(carpetaSalida, antes)
		if len(nuevos) == 0 && force {
			if reciente := archivoMasReciente(carpetaSalida); reciente != "" {
				nuevos = []string{reciente}
			}
		}
		if len(nuevos) == 0 {
			fmt.Printf("   ⚠️  No apareció archivo nuevo, probando siguiente...\n")
			continue
		}
		ultimoDescargado = nuevos[0]

		if checkHasAudio(ultimoDescargado) {
			fmt.Printf("✅ Descargado CON AUDIO: %s\n", filepath.Base(ultimoDescargado))
			return nil
		}
		fmt.Printf("   ⚠️  Sin pista de audio con este selector, reintentando...\n")
	}

	if ultimoDescargado != "" {
		fmt.Printf("⚠️  Ningún selector trajo audio. Se usará el video mudo.\n")
		return nil
	}
	return fmt.Errorf("no se pudo descargar %s con ningún selector", url)
}

// ==========================================================
// DETECCIÓN DE ARCHIVOS NUEVOS
// ==========================================================
func snapshotArchivos(dir string) map[string]bool {
	set := make(map[string]bool)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return set
	}
	for _, e := range entries {
		if !e.IsDir() {
			set[e.Name()] = true
		}
	}
	return set
}

func archivosNuevos(dir string, antes map[string]bool) []string {
	var nuevos []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !antes[e.Name()] {
			nuevos = append(nuevos, filepath.Join(dir, e.Name()))
		}
	}
	return nuevos
}

func eliminarNuevos(dir string, antes map[string]bool) {
	for _, f := range archivosNuevos(dir, antes) {
		_ = os.Remove(f)
	}
}
