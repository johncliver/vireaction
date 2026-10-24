package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ==========================================================
// CONFIGURACIÓN
// ==========================================================
const (
	// --- Descarga (Facebook Reels) ---
	profileURL            = "https://www.facebook.com/SeanBridon/reels/"
	maxVideosPorEjecucion = 20
	archivoURLs           = "reels_urls.txt"
	archivoRegistro       = "descargados.txt"
	archivoProcesados     = "procesados.txt"

	// --- Carpetas ---
	InputFolder  = "videos"
	OutputFolder = "salida"

	// --- Edición ---
	MemeVideo  = "meme.mp4"
	RisaVideo  = "risa.mp4"
	TitleText  = "Africa Lore Love"
	VideoSpeed = 1.0

	// --- Codificación ---
	FFmpegPreset = "superfast"
	FFmpegCRF    = "23"

	// --- Anti-copyright ---
	MirrorVideo = true
)

// ==========================================================
// ASSETS PRE-CODIFICADOS
// ==========================================================
var (
	preparedMeme = "temp_prepared_meme.mp4"
	preparedRisa = "temp_prepared_risa.mp4"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	opcion := preguntarModo()

	// Opción 4: solo subir a YouTube → no requiere ffmpeg/yt-dlp/assets
	if opcion == 4 {
		flujoSubidaYoutube()
		fmt.Println("\n🏁 Proceso finalizado.")
		return
	}

	// ⭐ NUEVO: Opción 5 → solo subir a Facebook
	if opcion == 5 {
		flujoSubidaFacebook()
		fmt.Println("\n🏁 Proceso finalizado.")
		return
	}

	for _, bin := range []string{"ffmpeg", "ffprobe", "yt-dlp"} {
		if _, err := exec.LookPath(bin); err != nil {
			log.Fatalf("❌ '%s' no está instalado o no está en el PATH.", bin)
		}
	}
	if _, err := os.Stat(MemeVideo); os.IsNotExist(err) {
		log.Fatalf("❌ Falta el archivo '%s'.", MemeVideo)
	}
	if _, err := os.Stat(RisaVideo); os.IsNotExist(err) {
		log.Fatalf("❌ Falta el archivo '%s'.", RisaVideo)
	}
	if err := os.MkdirAll(InputFolder, 0755); err != nil {
		log.Fatalf("❌ Error al crear carpeta '%s': %v", InputFolder, err)
	}
	if err := os.MkdirAll(OutputFolder, 0755); err != nil {
		log.Fatalf("❌ Error al crear carpeta '%s': %v", OutputFolder, err)
	}

	if err := prepararAssets(); err != nil {
		log.Fatalf("❌ Error preparando assets: %v", err)
	}
	defer os.Remove(preparedMeme)
	defer os.Remove(preparedRisa)

	switch opcion {
	case 1:
		flujoDescargaCompleto()
	case 2:
		flujoDescargaDesdeLista()
	case 3:
		fmt.Println("\n⏭️  Omitiendo descarga. Procesando videos existentes...")
	}

	procesarTodosLosVideos()
	fmt.Println("\n🏁 Proceso finalizado.")
}

func preguntarSiNo(pregunta string) bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("\n❓ %s (S/N): ", pregunta)
		resp, err := reader.ReadString('\n')
		if err != nil {
			return false
		}
		resp = strings.ToLower(strings.TrimSpace(resp))
		if resp == "s" || resp == "si" || resp == "sí" || resp == "y" || resp == "yes" {
			return true
		}
		if resp == "n" || resp == "no" {
			return false
		}
		fmt.Println("   ⚠️  Responde 'S' (sí) o 'N' (no).")
	}
}

func preguntarModo() int {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("\n┌────────────────────────────────────────────────┐")
		fmt.Println("│ ¿Qué deseas hacer?                              │")
		fmt.Println("├────────────────────────────────────────────────┤")
		fmt.Println("│ 1) Descargar videos NUEVOS de Facebook          │")
		fmt.Println("│    (abre Brave y extrae URLs con Rod)           │")
		fmt.Println("│ 2) Descargar desde lista guardada               │")
		fmt.Println("│    (reels_urls.txt, SIN abrir Facebook)         │")
		fmt.Println("│ 3) Solo procesar videos ya existentes           │")
		fmt.Println("│    (videos/ → salida/)                          │")
		fmt.Println("│ 4) Subir videos de 'salida/' a YouTube          │")
		fmt.Println("│    (título=nombre, público, borra local)        │")
		fmt.Println("│ 5) Subir videos de 'salida/' a Facebook Reels   │") // ⭐ NUEVO
		fmt.Println("│    (descripción=nombre, borra local)            │") // ⭐ NUEVO
		fmt.Println("└────────────────────────────────────────────────┘")
		fmt.Print("Opción [1/2/3/4/5]: ")
		resp, err := reader.ReadString('\n')
		if err != nil {
			return 3
		}
		switch strings.TrimSpace(resp) {
		case "1":
			return 1
		case "2":
			return 2
		case "3":
			return 3
		case "4":
			return 4
		case "5": // ⭐ NUEVO
			return 5
		}
		fmt.Println("   ⚠️  Elige 1, 2, 3, 4 o 5.")
	}
}
