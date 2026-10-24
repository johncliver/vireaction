package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ==========================================================
// ASSETS
// ==========================================================
func prepararAssets() error {
	fmt.Println("\n🔧 Pre-codificando assets (meme.mp4 y risa.mp4) UNA sola vez...")
	if err := prepararAsset(MemeVideo, preparedMeme,
		"scale=1080:860:force_original_aspect_ratio=increase,crop=1080:860"); err != nil {
		return fmt.Errorf("meme: %w", err)
	}
	if err := prepararAsset(RisaVideo, preparedRisa,
		"scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920"); err != nil {
		return fmt.Errorf("risa: %w", err)
	}
	// fmt.Println("✅ Assets listos.\n")
	return nil
}

func prepararAsset(input, output, vf string) error {
	cmd := exec.Command("ffmpeg", "-y",
		"-i", input,
		"-vf", vf+",fps=30,format=yuv420p",
		"-c:v", "libx264", "-preset", FFmpegPreset, "-crf", FFmpegCRF,
		"-profile:v", "high", "-level", "4.2",
		"-c:a", "aac", "-b:a", "128k", "-ar", "44100", "-ac", "2",
		"-movflags", "+faststart",
		"-stats", "-loglevel", "warning",
		output,
	)
	return runCmdLive(cmd)
}

// ==========================================================
// PROCESAMIENTO MASIVO
// ==========================================================
func procesarTodosLosVideos() {
	entries, err := os.ReadDir(InputFolder)
	if err != nil {
		log.Printf("⚠️  Error al leer '%s': %v", InputFolder, err)
		return
	}
	validExtensions := map[string]bool{".mp4": true, ".mov": true, ".mkv": true, ".avi": true}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileName := entry.Name()
		ext := strings.ToLower(filepath.Ext(fileName))
		if !validExtensions[ext] {
			continue
		}
		inputPath := filepath.Join(InputFolder, fileName)
		nameWithoutExt := strings.TrimSuffix(fileName, ext)
		outputPath := filepath.Join(OutputFolder,
			fmt.Sprintf("short_%s_%s.mp4", nameWithoutExt, time.Now().Format("20060102-150405")))

		fmt.Printf("\n═══════════════════════════════════════════════\n")
		fmt.Printf("🎬 Procesando: %s\n", fileName)
		fmt.Printf("   → Salida:    %s\n", outputPath)
		fmt.Printf("═══════════════════════════════════════════════\n")

		if err := processVideo(inputPath, outputPath); err != nil {
			log.Printf("❌ Error procesando %s: %v (se conserva original)", fileName, err)
			continue
		}

		info, statErr := os.Stat(outputPath)
		if statErr != nil || info.Size() < 1024 {
			log.Printf("⚠️  Salida inválida (%v), NO se borra el original", statErr)
			continue
		}

		fmt.Printf("✅ Video generado: %s (%.1f MB)\n", outputPath, float64(info.Size())/1048576)
		logProcesado(inputPath, outputPath)

		if err := os.Remove(inputPath); err != nil {
			log.Printf("⚠️  No se pudo eliminar el original '%s': %v", inputPath, err)
		} else {
			fmt.Printf("🗑️  Original eliminado: %s\n", fileName)
		}
		count++
	}
	if count == 0 {
		fmt.Println("📂 No hay videos pendientes en la carpeta de entrada.")
	} else {
		fmt.Printf("🎉 Total procesados en esta ronda: %d\n", count)
	}
}

// ==========================================================
// EDICIÓN + ANTI-COPYRIGHT
// ==========================================================
func processVideo(inputVideo, outputFile string) error {
	duration, err := getVideoDuration(inputVideo)
	if err != nil {
		return fmt.Errorf("ffprobe: %w", err)
	}
	fmt.Printf("   Duración original: %.2f s\n", duration)

	speed := VideoSpeed
	if speed == 1.0 {
		speed = 1.02 + rand.Float64()*0.04
	}
	contrast := 1.06 + rand.Float64()*0.06
	saturation := 1.12 + rand.Float64()*0.10
	brightness := 0.02 + rand.Float64()*0.03
	pitchFactor := 1.02 + rand.Float64()*0.03
	ptsFactor := 1.0 / speed

	mirror := ""
	if MirrorVideo {
		mirror = "hflip,"
	}

	tempEdited := fmt.Sprintf("temp_edit_%d.mp4", time.Now().UnixNano())
	defer os.Remove(tempEdited)
	listFile := fmt.Sprintf("temp_list_%d.txt", time.Now().UnixNano())
	defer os.Remove(listFile)

	hasAudio := checkHasAudio(inputVideo)

	filterComplex := fmt.Sprintf(`
[0:v]setpts=%.4f*PTS,
     %s
     eq=contrast=%.2f:brightness=%.2f:saturation=%.2f,
     noise=alls=6:allf=t,
     scale=iw*1.06:-1,
     crop=iw/1.06:ih/1.06,
     scale=1080:960:force_original_aspect_ratio=increase,
     crop=1080:960,
     fps=30,format=yuv420p[v_copy];
[1:v]fps=30,format=yuv420p[v_meme];
color=c=black:s=1080x1920:d=%.3f:r=30,format=yuv420p[bg];
[bg][v_meme]overlay=0:0:shortest=1[bg1];
[bg1][v_copy]overlay=0:960:shortest=1[bg2];
[bg2]drawtext=text='%s':fontcolor=white:bordercolor=black:borderw=5:fontsize=52:x=(w-text_w)/2:y=885[v_final]
`, ptsFactor, mirror, contrast, brightness, saturation, duration, TitleText)

	args := []string{"-y",
		"-i", inputVideo,
		"-stream_loop", "-1", "-i", preparedMeme,
	}

	if hasAudio {
		audioFilter := fmt.Sprintf("atempo=%.4f,asetrate=44100*%.3f,aresample=44100,volume=0.95",
			speed, pitchFactor)
		args = append(args,
			"-filter_complex", filterComplex,
			"-map", "[v_final]",
			"-map", "0:a",
			"-af", audioFilter,
		)
	} else {
		args = append(args,
			"-f", "lavfi", "-i", fmt.Sprintf("anullsrc=r=44100:cl=stereo:d=%.3f", duration),
			"-filter_complex", filterComplex,
			"-map", "[v_final]",
			"-map", "2:a",
		)
	}

	args = append(args,
		"-c:v", "libx264", "-preset", FFmpegPreset, "-crf", FFmpegCRF,
		"-tune", "fastdecode",
		"-profile:v", "high", "-level", "4.2",
		"-c:a", "aac", "-b:a", "128k", "-ar", "44100", "-ac", "2",
		"-t", fmt.Sprintf("%.3f", duration),
		"-movflags", "+faststart",
		"-stats", "-loglevel", "warning",
		tempEdited,
	)

	fmt.Printf("   → Codificando segmento principal (%.2fx, espejo=%v)...\n", speed, MirrorVideo)
	if err := runCmdLive(exec.Command("ffmpeg", args...)); err != nil {
		return fmt.Errorf("encode principal: %w", err)
	}

	listContent := fmt.Sprintf("file '%s'\nfile '%s'\n",
		filepath.ToSlash(absPath(tempEdited)),
		filepath.ToSlash(absPath(preparedRisa)))
	if err := os.WriteFile(listFile, []byte(listContent), 0644); err != nil {
		return fmt.Errorf("escribiendo lista concat: %w", err)
	}

	fmt.Println("   → Concatenando con risa (stream copy, sin re-encode)...")
	cmdConcat := exec.Command("ffmpeg", "-y",
		"-f", "concat", "-safe", "0", "-i", listFile,
		"-c", "copy",
		"-movflags", "+faststart",
		"-stats", "-loglevel", "warning",
		outputFile,
	)
	if err := runCmdLive(cmdConcat); err != nil {
		return fmt.Errorf("concat: %w", err)
	}
	return nil
}

// ==========================================================
// PROBES + HELPERS DE FFMPEG
// ==========================================================
func getVideoDuration(filename string) (float64, error) {
	cmd := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filename)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(out.String()), 64)
}

func checkHasAudio(path string) bool {
	cmd := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "audio"
}

func runCmdLive(cmd *exec.Cmd) error {
	var buf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, buf.String())
	}
	return nil
}

func logProcesado(input, output string) {
	f, err := os.OpenFile(archivoProcesados, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\t%s\t%s\n", time.Now().Format("2006-01-02 15:04:05"), input, output)
}
