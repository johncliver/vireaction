package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// ==========================================================
// CONFIGURACIÓN FACEBOOK
// ==========================================================
const (
	archivoSubidosFacebook    = "subidos_facebook.txt"
	facebookReelCreateURL     = "https://www.facebook.com/reel/create"
	facebookHomeURL           = "https://www.facebook.com/"
	minutosEntreVideosFb      = 5
	segundosEsperaPublicacion = 120
)

// ==========================================================
// FLUJO PRINCIPAL DE SUBIDA A FACEBOOK
// ==========================================================
func flujoSubidaFacebook() {
	entries, err := os.ReadDir(OutputFolder)
	if err != nil {
		log.Fatalf("❌ Error leyendo '%s': %v", OutputFolder, err)
	}

	subidos, _ := leerArchivoComoSet(archivoSubidosFacebook)
	var pendientes []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".mp4") {
			continue
		}
		full := filepath.Join(OutputFolder, e.Name())
		abs, _ := filepath.Abs(full)
		if subidos[abs] {
			continue
		}
		pendientes = append(pendientes, full)
	}

	if len(pendientes) == 0 {
		fmt.Println("\n✨ No hay videos pendientes de subir en 'salida/'.")
		return
	}

	fmt.Printf("\n📹 Videos pendientes de subir a Facebook Reels: %d\n", len(pendientes))
	for i, v := range pendientes {
		fmt.Printf("   %d) %s\n", i+1, filepath.Base(v))
	}

	if !preguntarSiNo("¿Comenzar subida automática a Facebook Reels?") {
		fmt.Println("⏭️  Subida cancelada.")
		return
	}

	// Facebook es MUY estricto con la detección de bots → headless(false)
	l := launcher.New().
		Bin(obtenerRutaBrave()).
		UserDataDir(obtenerUserDataDir()).
		Headless(false).
		NoSandbox(true).
		Set("disable-blink-features", "AutomationControlled").
		Set("excludeSwitches", "enable-automation").
		Set("useAutomationExtension", "false")
	url := l.MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	defer browser.MustClose()

	total := len(pendientes)
	exitosos := 0
	for i, videoPath := range pendientes {
		fmt.Printf("\n═══════════════════════════════════════════════\n")
		fmt.Printf("📤 Subiendo %d/%d: %s\n", i+1, total, filepath.Base(videoPath))
		fmt.Printf("═══════════════════════════════════════════════\n")

		err := subirUnReelFacebook(browser, videoPath)
		if err != nil {
			log.Printf("❌ Error subiendo %s: %v", filepath.Base(videoPath), err)
		} else {
			abs, _ := filepath.Abs(videoPath)
			_ = anexarLineaArchivo(archivoSubidosFacebook, abs)
			fmt.Printf("🗑️  Eliminando archivo local: %s\n", filepath.Base(videoPath))
			if err := os.Remove(videoPath); err != nil {
				log.Printf("⚠️  No se pudo eliminar '%s': %v", videoPath, err)
			}
			exitosos++
		}

		if i < total-1 {
			fmt.Printf("\n⏳ Esperando %d minutos antes del siguiente video...\n", minutosEntreVideosFb)
			time.Sleep(time.Duration(minutosEntreVideosFb) * time.Minute)
		}
	}

	fmt.Printf("\n🎉 Subidos exitosamente a Facebook: %d/%d\n", exitosos, total)
}

// ==========================================================
// SUBIR UN VIDEO (wizard completo Facebook Reels)
// ==========================================================
func subirUnReelFacebook(browser *rod.Browser, videoPath string) error {
	absVideo, err := filepath.Abs(videoPath)
	if err != nil {
		return fmt.Errorf("resolviendo ruta: %w", err)
	}

	page := browser.MustPage("").Timeout(15 * time.Minute)
	defer page.MustClose()

	fmt.Println("   → Abriendo Facebook Reels...")
	page.MustNavigate(facebookReelCreateURL)
	page.MustWaitLoad()
	time.Sleep(4 * time.Second)

	// Verificar login
	if info, _ := page.Info(); info != nil {
		if strings.Contains(info.URL, "/login") ||
			strings.Contains(info.URL, "checkpoint") ||
			strings.Contains(info.URL, "two_step_verification") {
			return fmt.Errorf("no estás logueado en Facebook. Inicia sesión en Brave y reintenta")
		}
	}

	// Buscar input de archivos directamente (en /reel/create suele estar listo)
	fmt.Println("   → Buscando input de archivos...")
	fileInput, err := page.Timeout(30 * time.Second).Element(`input[type="file"][accept*="video"]`)
	if err != nil {
		// Fallback: hacer clic en el botón "Reel" (por si la URL directa no cargó el wizard)
		fmt.Println("   → Fallback: clic en botón Reel...")
		if err2 := clickReelButton(page); err2 != nil {
			return fmt.Errorf("no se encontró input de archivo ni botón Reel: %w", err)
		}
		fileInput, err = page.Timeout(60 * time.Second).Element(`input[type="file"][accept*="video"]`)
		if err != nil {
			return fmt.Errorf("input file no encontrado: %w", err)
		}
	}

	fmt.Printf("   → Cargando archivo: %s\n", filepath.Base(absVideo))
	if err := fileInput.SetFiles([]string{absVideo}); err != nil {
		return fmt.Errorf("setFiles: %w", err)
	}

	// Esperar a que el editor termine de cargar
	fmt.Println("   → Esperando procesamiento del video...")
	time.Sleep(4 * time.Second)

	// ──────── Siguiente #1 ────────
	fmt.Println("   → Siguiente (1/2)")
	if err := clickSiguiente(page, 120*time.Second); err != nil {
		return fmt.Errorf("siguiente 1: %w", err)
	}
	time.Sleep(2 * time.Second)

	// ──────── Siguiente #2 ────────
	fmt.Println("   → Siguiente (2/2)")
	if err := clickSiguiente(page, 90*time.Second); err != nil {
		return fmt.Errorf("siguiente 2: %w", err)
	}
	time.Sleep(2 * time.Second)

	// ──────── Descripción / Título ────────
	title := strings.TrimSuffix(filepath.Base(absVideo), filepath.Ext(absVideo))
	if len(title) > 2200 {
		title = title[:2200]
	}
	fmt.Printf("   → Descripción: %s\n", title)
	if err := setFacebookText(page, title); err != nil {
		log.Printf("   ⚠️  No se pudo escribir descripción: %v", err)
	}
	time.Sleep(1 * time.Second)

	// ──────── Publicar ────────
	fmt.Println("   → Publicando...")
	if err := clickPublicar(page); err != nil {
		return fmt.Errorf("publicar: %w", err)
	}

	// ──────── Confirmación ────────
	if err := waitFacebookPublishDone(page, time.Duration(segundosEsperaPublicacion)*time.Second); err != nil {
		log.Printf("   ⚠️  No se confirmó publicación: %v", err)
	} else {
		fmt.Println("   ✅ Reel publicado correctamente en Facebook.")
	}
	return nil
}

// ==========================================================
// HELPERS DE UI FACEBOOK
// ==========================================================

// Hace clic en el botón con aria-label="Reel"
func clickReelButton(page *rod.Page) error {
	// 1) Selector directo que nos pasó el usuario
	el, err := page.Timeout(20 * time.Second).Element(`div[aria-label="Reel"][role="button"]`)
	if err == nil {
		if err := el.Click(proto.InputMouseButtonLeft, 1); err == nil {
			time.Sleep(2 * time.Second)
			return nil
		}
	}

	// 2) Buscar por texto "Reel" en botones
	res, _ := page.Eval(`() => {
		const all = document.querySelectorAll('div[role="button"]');
		for (const b of all) {
			if (b.offsetParent === null) continue;
			const txt = (b.innerText || '').trim().toLowerCase();
			if (txt === 'reel' || txt === 'reels' || txt.includes('crear reel')) {
				b.click();
				return true;
			}
		}
		// 3) Buscar por aria-label que contenga 'reel'
		const byLabel = document.querySelector('[aria-label*="Reel"], [aria-label*="reel"]');
		if (byLabel) { byLabel.click(); return true; }
		return false;
	}`)
	if res != nil && res.Value.Bool() {
		time.Sleep(2 * time.Second)
		return nil
	}
	return fmt.Errorf("botón Reel no encontrado")
}

// Hace clic en el botón "Siguiente" (espera a que esté clickeable)
func clickSiguiente(page *rod.Page, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		res, _ := page.Eval(`() => {
			// Selector directo (aria-label="Siguiente")
			const btn = document.querySelector('div[aria-label="Siguiente"][role="button"]');
			if (btn && btn.offsetParent !== null && btn.getAttribute('aria-disabled') !== 'true') {
				btn.click();
				return true;
			}
			// Fallback: por texto
			const all = document.querySelectorAll('div[role="button"], button');
			for (const b of all) {
				if (b.offsetParent === null) continue;
				if (b.getAttribute('aria-disabled') === 'true') continue;
				if (b.disabled) continue;
				const txt = (b.innerText || '').trim();
				if (txt === 'Siguiente' || txt === 'Next') {
					b.click();
					return true;
				}
			}
			return false;
		}`)
		if res != nil && res.Value.Bool() {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout esperando botón Siguiente")
}

// Escribe en el contenteditable "Describe tu reel…"
func setFacebookText(page *rod.Page, text string) error {
	res, err := page.Eval(`(txt) => {
		// 1) Buscar por aria-label que contenga "describe" o "descrip"
		const eds = document.querySelectorAll('[contenteditable="true"][role="textbox"]');
		for (const el of eds) {
			const label = (el.getAttribute('aria-label') || '').toLowerCase();
			if (label.includes('describe') || label.includes('descrip') || label.includes('reel')) {
				el.focus();
				const range = document.createRange();
				range.selectNodeContents(el);
				const s = window.getSelection();
				s.removeAllRanges();
				s.addRange(range);
				document.execCommand('insertText', false, txt);
				el.dispatchEvent(new Event('input', {bubbles: true}));
				el.dispatchEvent(new Event('change', {bubbles: true}));
				return true;
			}
		}
		// 2) Fallback: primer contenteditable visible
		for (const el of eds) {
			if (el.offsetParent === null) continue;
			el.focus();
			const range = document.createRange();
			range.selectNodeContents(el);
			const s = window.getSelection();
			s.removeAllRanges();
			s.addRange(range);
			document.execCommand('insertText', false, txt);
			el.dispatchEvent(new Event('input', {bubbles: true}));
			return true;
		}
		return false;
	}`, text)
	if err != nil {
		return err
	}
	if res == nil || !res.Value.Bool() {
		return fmt.Errorf("contenteditable no encontrado")
	}
	return nil
}

// Hace clic en "Publicar"
func clickPublicar(page *rod.Page) error {
	start := time.Now()
	for time.Since(start) < 60*time.Second {
		res, _ := page.Eval(`() => {
			// 1) Selector directo aria-label="Publicar"
			const btn = document.querySelector('div[aria-label="Publicar"][role="button"]');
			if (btn && btn.offsetParent !== null && btn.getAttribute('aria-disabled') !== 'true') {
				btn.click();
				return true;
			}
			// 2) Fallback por texto
			const all = document.querySelectorAll('div[role="button"], button');
			for (const b of all) {
				if (b.offsetParent === null) continue;
				if (b.getAttribute('aria-disabled') === 'true') continue;
				if (b.disabled) continue;
				const txt = (b.innerText || '').trim();
				if (txt === 'Publicar' || txt === 'Publish') {
					b.click();
					return true;
				}
			}
			return false;
		}`)
		if res != nil && res.Value.Bool() {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("botón Publicar no encontrado")
}

// Espera a que el wizard se cierre o aparezca confirmación
func waitFacebookPublishDone(page *rod.Page, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		res, _ := page.Eval(`() => {
			// Mensajes de éxito conocidos
			const body = (document.body.innerText || '').toLowerCase();
			if (body.includes('se publicó tu reel') ||
			    body.includes('tu reel se publicó') ||
			    body.includes('reel published') ||
			    body.includes('tu publicación se compartió')) {
				return 'published';
			}
			// Si el botón Publicar desapareció → wizard cerrado
			const btn = document.querySelector('div[aria-label="Publicar"][role="button"]');
			if (!btn || btn.offsetParent === null) {
				return 'closed';
			}
			return '';
		}`)
		if res != nil {
			switch res.Value.Str() {
			case "published":
				fmt.Println("   ✅ Mensaje de publicación detectado.")
				return nil
			case "closed":
				time.Sleep(2 * time.Second)
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout esperando confirmación de publicación")
}
