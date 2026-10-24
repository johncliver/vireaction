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
// CONFIGURACIÓN YOUTUBE
// ==========================================================
const (
	archivoSubidos       = "subidos.txt"
	archivoReclamados    = "reclamados.txt"
	youtubeUploadURL     = "https://www.youtube.com/upload"
	minutosEntreVideos   = 5
	segundosEsperaChecks = 120 // tiempo máximo esperando verificaciones de copyright
)

// ==========================================================
// FLUJO PRINCIPAL DE SUBIDA
// ==========================================================
func flujoSubidaYoutube() {
	entries, err := os.ReadDir(OutputFolder)
	if err != nil {
		log.Fatalf("❌ Error leyendo '%s': %v", OutputFolder, err)
	}

	subidos, _ := leerArchivoComoSet(archivoSubidos)
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

	fmt.Printf("\n📹 Videos pendientes de subir: %d\n", len(pendientes))
	for i, v := range pendientes {
		fmt.Printf("   %d) %s\n", i+1, filepath.Base(v))
	}

	if !preguntarSiNo("¿Comenzar subida automática a YouTube?") {
		fmt.Println("⏭️  Subida cancelada.")
		return
	}

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

	total := len(pendientes)
	exitosos := 0
	reclamados := 0
	for i, videoPath := range pendientes {
		fmt.Printf("\n═══════════════════════════════════════════════\n")
		fmt.Printf("📤 Subiendo %d/%d: %s\n", i+1, total, filepath.Base(videoPath))
		fmt.Printf("═══════════════════════════════════════════════\n")

		claimed, err := subirUnVideo(browser, videoPath)
		if err != nil {
			log.Printf("❌ Error subiendo %s: %v", filepath.Base(videoPath), err)
		} else {
			abs, _ := filepath.Abs(videoPath)
			_ = anexarLineaArchivo(archivoSubidos, abs)
			fmt.Printf("🗑️  Eliminando archivo local: %s\n", filepath.Base(videoPath))
			if err := os.Remove(videoPath); err != nil {
				log.Printf("⚠️  No se pudo eliminar '%s': %v", videoPath, err)
			}
			exitosos++
			if claimed {
				reclamados++
			}
		}

		if i < total-1 {
			fmt.Printf("\n⏳ Esperando %d minutos antes del siguiente video...\n", minutosEntreVideos)
			time.Sleep(time.Duration(minutosEntreVideos) * time.Minute)
		}
	}

	fmt.Printf("\n🎉 Subidos exitosamente: %d/%d", exitosos, total)
	if reclamados > 0 {
		fmt.Printf("  (⚠️  %d con copyright → ver '%s')", reclamados, archivoReclamados)
	}
	fmt.Println()
}

// ==========================================================
// SUBIR UN VIDEO (wizard completo)
// ==========================================================
func subirUnVideo(browser *rod.Browser, videoPath string) (bool, error) {
	absVideo, err := filepath.Abs(videoPath)
	if err != nil {
		return false, fmt.Errorf("resolviendo ruta: %w", err)
	}

	page := browser.MustPage("").Timeout(10 * time.Minute)
	defer page.MustClose()

	fmt.Println("   → Abriendo YouTube Studio...")
	page.MustNavigate(youtubeUploadURL)
	page.MustWaitLoad()
	time.Sleep(4 * time.Second)

	if info, _ := page.Info(); info != nil {
		if strings.Contains(info.URL, "accounts.google.com") || strings.Contains(info.URL, "/signin") {
			return false, fmt.Errorf("no estás logueado en YouTube. Abre Brave, inicia sesión y reintenta")
		}
	}

	fmt.Println("   → Buscando input de archivos...")
	fileInput, err := page.Timeout(60 * time.Second).Element(`input[type="file"]`)
	if err != nil {
		fmt.Println("   → Fallback: abriendo diálogo con botón Crear...")
		if err2 := abrirDialogoUpload(page); err2 != nil {
			return false, fmt.Errorf("no se encontró input de archivo: %w", err)
		}
		fileInput, err = page.Timeout(30 * time.Second).Element(`input[type="file"]`)
		if err != nil {
			return false, fmt.Errorf("input file no encontrado: %w", err)
		}
	}

	fmt.Printf("   → Cargando archivo: %s\n", filepath.Base(absVideo))
	if err := fileInput.SetFiles([]string{absVideo}); err != nil {
		return false, fmt.Errorf("setFiles: %w", err)
	}

	fmt.Println("   → Esperando editor de detalles...")
	if _, err := page.Timeout(90 * time.Second).Element(`#title-textarea #textbox`); err != nil {
		return false, fmt.Errorf("editor no cargó: %w", err)
	}
	time.Sleep(2 * time.Second)

	title := strings.TrimSuffix(filepath.Base(absVideo), filepath.Ext(absVideo))
	if len(title) > 100 {
		title = title[:100]
	}
	fmt.Printf("   → Título: %s\n", title)
	if err := setContentEditable(page, `#title-textarea #textbox`, title); err != nil {
		return false, fmt.Errorf("set título: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	_ = setContentEditable(page, `#description-textarea #textbox`, title)
	time.Sleep(500 * time.Millisecond)

	fmt.Println("   → Esperando que termine la subida...")
	if err := waitUploadDone(page, 10*time.Minute); err != nil {
		return false, err
	}

	// ---- DETECCIÓN DE COPYRIGHT ----
	claimed := false
	fmt.Printf("   → Esperando verificaciones de copyright (máx %ds)...\n", segundosEsperaChecks)
	claimedRes, claimMsg, alreadyPublished, errCheck := waitForChecksComplete(
		page, time.Duration(segundosEsperaChecks)*time.Second,
	)

	switch {
	case errCheck != nil:
		log.Printf("   ⚠️  %v (continuando de todas formas)", errCheck)

	case alreadyPublished:
		// El wizard se cerró → YouTube ya publicó el video.
		// No seguir: los botones MFK / Siguiente / Público / Publicar ya no existen.
		fmt.Printf("   ✅ %s\n", claimMsg)
		fmt.Println("   ✅ Video publicado correctamente (detectado por cierre del wizard).")
		return false, nil

	default:
		claimed = claimedRes
		if claimed {
			fmt.Printf("   ⚠️  CONTENIDO RECLAMADO: %s\n", claimMsg)
			_ = anexarLineaArchivo(archivoReclamados,
				fmt.Sprintf("%s\t%s\t%s", time.Now().Format("2006-01-02 15:04:05"), absVideo, claimMsg))
		} else {
			fmt.Printf("   ✅ %s\n", claimMsg)
		}
	}

	// ---- RESTO DEL WIZARD ----
	fmt.Println("   → Marcando: No es contenido para niños")
	if err := clickRadio(page, `tp-yt-paper-radio-button[name="VIDEO_MADE_FOR_KIDS_NOT_MFK"]`); err != nil {
		log.Printf("   ⚠️  %v", err)
	}
	time.Sleep(1 * time.Second)

	for i := 1; i <= 3; i++ {
		fmt.Printf("   → Siguiente (%d/3)\n", i)
		if err := waitAndClickNext(page, 60*time.Second); err != nil {
			return claimed, fmt.Errorf("siguiente %d: %w", i, err)
		}
		time.Sleep(2 * time.Second)
	}

	fmt.Println("   → Marcando visibilidad: Público")
	if err := clickRadio(page, `tp-yt-paper-radio-button[name="PUBLIC"]`); err != nil {
		return claimed, fmt.Errorf("radio público: %w", err)
	}
	time.Sleep(1 * time.Second)

	fmt.Println("   → Publicando...")
	if err := clickPublish(page); err != nil {
		return claimed, fmt.Errorf("publicar: %w", err)
	}

	if err := waitPublishDone(page, 60*time.Second); err != nil {
		return claimed, fmt.Errorf("no se confirmó la publicación: %w", err)
	}
	fmt.Println("   ✅ Video publicado correctamente.")
	return claimed, nil
}

// ==========================================================
// DETECCIÓN DE VERIFICACIONES DE COPYRIGHT
// ==========================================================
// waitForChecksComplete espera a que YouTube muestre el mensaje
// "Se completaron las verificaciones..." y devuelve si encontró contenido reclamado.
//
//	(claimed=true, msg, nil)  → verificaciones completas CON claim
//	(claimed=false, msg, nil) → verificaciones completas SIN claim
//	(claimed=false, "", err)  → timeout, no se pudo verificar
//
// waitForChecksComplete espera a que YouTube muestre el mensaje
// "Se completaron las verificaciones...". Si el wizard desaparece antes,
// se asume que el video ya fue publicado.
//
//	(claimed, msg, false, nil) → verificaciones completas (con/sin claim)
//	(false, msg, true,  nil)   → wizard cerrado → video ya publicado
//	(false, "",   false, err)  → timeout sin señal
func waitForChecksComplete(page *rod.Page, timeout time.Duration) (claimed bool, msg string, alreadyPublished bool, err error) {
	// Dar tiempo a que aparezca la sección de verificaciones (evita falso positivo)
	time.Sleep(3 * time.Second)

	start := time.Now()
	for time.Since(start) < timeout {
		// 1) Buscar el mensaje de verificaciones completas
		res, evalErr := page.Eval(`() => {
			const labels = document.querySelectorAll('.progress-label');
			for (const l of labels) {
				const txt = (l.innerText || '').trim();
				const lower = txt.toLowerCase();
				if (lower.includes('se completaron las verificaciones') ||
				    lower.includes('checks complete') ||
				    lower.includes('check complete')) {
					return txt;
				}
			}
			return '';
		}`)
		if evalErr == nil && res != nil {
			m := res.Value.Str()
			if m != "" {
				lower := strings.ToLower(m)
				cl := strings.Contains(lower, "reclamado") ||
					strings.Contains(lower, "claimed") ||
					strings.Contains(lower, "copyrighted")
				return cl, m, false, nil
			}
		}

		// 2) Si el wizard desapareció → el video ya fue publicado
		closedRes, evalErr2 := page.Eval(`() => {
			const markers = document.querySelectorAll('#title-textarea, #next-button');
			if (markers.length === 0) return true;
			for (const m of markers) {
				if (m.offsetParent !== null) return false;
			}
			return true;
		}`)
		if evalErr2 == nil && closedRes != nil && closedRes.Value.Bool() {
			return false, "el diálogo se cerró → el video ya fue publicado", true, nil
		}

		time.Sleep(2 * time.Second)
	}
	return false, "", false, fmt.Errorf("timeout esperando verificaciones de copyright")
}

// ==========================================================
// HELPERS DE UI
// ==========================================================
func abrirDialogoUpload(page *rod.Page) error {
	createBtn, err := page.Timeout(30 * time.Second).Element(`#create-icon`)
	if err != nil {
		createBtn, err = page.Timeout(10*time.Second).ElementR("button", "(?i)crear")
		if err != nil {
			return err
		}
	}
	if err := createBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)

	uploadItem, err := page.Timeout(15*time.Second).ElementR("tp-yt-paper-item", "(?i)subir videos?")
	if err != nil {
		return err
	}
	return uploadItem.Click(proto.InputMouseButtonLeft, 1)
}

func setContentEditable(page *rod.Page, selector, text string) error {
	res, err := page.Eval(`(sel, txt) => {
		const el = document.querySelector(sel);
		if (!el) return false;
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
	}`, selector, text)
	if err != nil {
		return err
	}
	if res == nil || !res.Value.Bool() {
		return fmt.Errorf("elemento '%s' no encontrado", selector)
	}
	return nil
}

func clickRadio(page *rod.Page, selector string) error {
	el, err := page.Timeout(30 * time.Second).Element(selector)
	if err != nil {
		return fmt.Errorf("radio '%s' no encontrado: %w", selector, err)
	}
	return el.Click(proto.InputMouseButtonLeft, 1)
}

func waitUploadDone(page *rod.Page, timeout time.Duration) error {
	start := time.Now()
	time.Sleep(5 * time.Second)
	for time.Since(start) < timeout {
		res, _ := page.Eval(`() => {
			const btns = document.querySelectorAll('#next-button');
			for (const b of btns) {
				if (b.offsetParent !== null) {
					return b.getAttribute('aria-disabled') === 'false';
				}
			}
			return false;
		}`)
		if res != nil && res.Value.Bool() {
			time.Sleep(2 * time.Second)
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout esperando fin de subida")
}

func waitAndClickNext(page *rod.Page, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		res, _ := page.Eval(`() => {
			const btns = document.querySelectorAll('#next-button');
			for (const b of btns) {
				if (b.offsetParent !== null && b.getAttribute('aria-disabled') === 'false') {
					const shape = b.querySelector('button');
					if (shape && !shape.disabled) {
						shape.click();
						return true;
					}
				}
			}
			return false;
		}`)
		if res != nil && res.Value.Bool() {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout esperando botón Siguiente clickeable")
}

func clickPublish(page *rod.Page) error {
	res, err := page.Eval(`() => {
		const all = document.querySelectorAll('button');
		for (const b of all) {
			if (b.offsetParent === null) continue;
			const txt = (b.innerText || '').trim();
			if ((txt === 'Publicar' || txt === 'Publish') && !b.disabled) {
				b.click();
				return true;
			}
		}
		const done = document.querySelectorAll('#done-button');
		for (const d of done) {
			if (d.offsetParent !== null) {
				const shape = d.querySelector('button');
				if (shape && !shape.disabled) {
					shape.click();
					return true;
				}
			}
		}
		return false;
	}`)
	if err != nil {
		return err
	}
	if res == nil || !res.Value.Bool() {
		return fmt.Errorf("botón Publicar no encontrado")
	}
	return nil
}

// Devuelve nil si el video quedó publicado (por cierre del wizard
// o por aparición del diálogo "Se publicó el video").
func waitPublishDone(page *rod.Page, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		res, _ := page.Eval(`() => {
			// 1) Diálogo "Se publicó el video" / "Video published" → ÉXITO
			const dlg = document.querySelector('ytcp-video-share-dialog');
			if (dlg && dlg.offsetParent !== null) {
				const t = dlg.querySelector('#dialog-title');
				const txt = t ? (t.innerText || '').trim().toLowerCase() : '';
				if (txt.includes('se publicó el video') ||
				    txt.includes('video publicado') ||
				    txt.includes('video published') ||
				    txt.includes('video shared')) {
					return 'published';
				}
			}

			// 2) Flujo antiguo: #done-button desaparece
			const btns = document.querySelectorAll('#done-button');
			let visible = false;
			for (const b of btns) {
				if (b.offsetParent !== null) { visible = true; break; }
			}
			if (!visible) return 'closed';

			return '';
		}`)
		if res != nil {
			switch res.Value.Str() {
			case "published":
				fmt.Println("   ✅ Diálogo 'Se publicó el video' detectado. Cerrando...")
				closeShareDialog(page)
				time.Sleep(1500 * time.Millisecond)
				return nil
			case "closed":
				time.Sleep(2 * time.Second)
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout esperando cierre del diálogo de publicación")
}

// Cierra el diálogo "Se publicó el video" si está visible.
func closeShareDialog(page *rod.Page) {
	_, _ = page.Eval(`() => {
		const dlg = document.querySelector('ytcp-video-share-dialog');
		if (!dlg) return false;
		// Preferimos el id exacto que aparece en tu HTML
		let btn = dlg.querySelector('#close-icon-button');
		if (!btn) btn = dlg.querySelector('ytcp-icon-button[icon="close"]');
		if (!btn) btn = dlg.querySelector('button[aria-label="Cerrar"], button[aria-label="Close"]');
		if (btn) { btn.click(); return true; }
		return false;
	}`)
}
