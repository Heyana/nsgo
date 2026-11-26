package android

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestAndroidRasterScaleMatchesDriver compiles RasterScale.java on the host JVM and verifies its half-step progression matches driver.RasterStep, its nested step combination caps at 4, and its bitmap budget keeps backing bitmaps within 4096 × 4096 pixels and a 16384-pixel side.
func TestAndroidRasterScaleMatchesDriver(t *testing.T) {
	javac, java := androidJDK(t)
	tmp := t.TempDir()
	driverSrc := filepath.Join(tmp, "RasterScaleDriver.java")
	const src = `package dev.oneo.nsgo;

public class RasterScaleDriver {
    public static void main(String[] args) {
        float[] scales = {Float.NaN, -2f, 0f, 0.5f, 1f, 1.01f, 1.5f, 1.51f, 2f, 2.25f, 2.5f, 3f, 3.4f, 3.6f, 4f, 8f};
        for (float s : scales) {
            System.out.println("STEP " + s + " " + RasterScale.step(s));
        }
        System.out.println("COMBINE " + RasterScale.combine(1.5f, 2f) + " " + RasterScale.combine(3f, 3f) + " " + RasterScale.combine(0.5f, 2f));
        // 200x200 at 3x -> 600x600 fits completely.
        float bSmall = RasterScale.boundedScale(200, 200, 3f);
        // 2048x2048 at 4x -> capped to 2x (4096x4096).
        float bSquare = RasterScale.boundedScale(2048, 2048, 4f);
        // 8192x20 at 4x -> capped by MAX_SIDE (16384 -> 2x).
        float bLong = RasterScale.boundedScale(8192, 20, 4f);
        // 5000x5000 at 4x -> already past 4096x4096, returns 1.
        float bHuge = RasterScale.boundedScale(5000, 5000, 4f);
        System.out.println("BOUND " + bSmall + " " + bSquare + " " + bLong + " " + bHuge);
        System.out.println("PIX "
            + RasterScale.scaledPixels(200, bSmall) + " "
            + RasterScale.scaledPixels(2048, bSquare) + " "
            + RasterScale.scaledPixels(8192, bLong) + " "
            + RasterScale.scaledPixels(5000, bHuge));
        // Canvas.ResolutionScale multiplies the bounded step; outside (0, 1) it keeps it.
        System.out.println("CANVAS "
            + RasterScale.canvasScale(300, 150, 1f, 0.5f) + " "
            + RasterScale.canvasScale(300, 150, 2f, 0.5f) + " "
            + RasterScale.canvasScale(2048, 2048, 4f, 0.5f) + " "
            + RasterScale.canvasScale(300, 150, 1f, 0f) + " "
            + RasterScale.canvasScale(300, 150, 2f, 1.5f) + " "
            + RasterScale.scaledPixels(300, RasterScale.canvasScale(300, 150, 1f, 0.5f)) + " "
            + RasterScale.scaledPixels(1, RasterScale.canvasScale(1, 1, 1f, 0.01f)));
    }
}
`
	if err := os.WriteFile(driverSrc, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	rasterSrc := filepath.Join("java", "dev", "oneo", "nsgo", "RasterScale.java")
	out, err := exec.CommandContext(t.Context(), javac, "-d", tmp, rasterSrc, driverSrc).CombinedOutput()
	if err != nil {
		t.Fatalf("javac failed: %v\n%s", err, out)
	}
	out, err = exec.CommandContext(t.Context(), java, "-cp", tmp, "dev.oneo.nsgo.RasterScaleDriver").CombinedOutput()
	if err != nil {
		t.Fatalf("java failed: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		t.Fatal("empty JVM output")
	}
	stepCount := 0
	for _, line := range lines {
		fields := strings.Fields(line)
		switch fields[0] {
		case "STEP":
			stepCount++
			var in float32
			if fields[1] == "NaN" {
				in = float32(math.NaN())
			} else {
				v, err := strconv.ParseFloat(fields[1], 32)
				if err != nil {
					t.Fatalf("parse input %q: %v", fields[1], err)
				}
				in = float32(v)
			}
			got, err := strconv.ParseFloat(fields[2], 64)
			if err != nil {
				t.Fatalf("parse step %q: %v", fields[2], err)
			}
			want := driver.RasterStep(in)
			if got != want {
				t.Errorf("RasterScale.step(%v) = %v, want %v (driver.RasterStep)", in, got, want)
			}
		case "COMBINE":
			if got := fmt.Sprintf("%s %s %s", fields[1], fields[2], fields[3]); got != "3.0 4.0 2.0" {
				t.Errorf("combine = %s, want 3.0 4.0 2.0", got)
			}
		case "BOUND":
			bSmall, _ := strconv.ParseFloat(fields[1], 64)
			bSquare, _ := strconv.ParseFloat(fields[2], 64)
			bLong, _ := strconv.ParseFloat(fields[3], 64)
			bHuge, _ := strconv.ParseFloat(fields[4], 64)
			if bSmall != 3 {
				t.Errorf("bSmall = %v, want 3", bSmall)
			}
			if math.Abs(bSquare-2) > 0.02 {
				t.Errorf("bSquare = %v, want ~2", bSquare)
			}
			if math.Abs(bLong-2) > 0.02 {
				t.Errorf("bLong = %v, want ~2", bLong)
			}
			if bHuge != 1 {
				t.Errorf("bHuge = %v, want 1", bHuge)
			}
		case "PIX":
			pSmall, _ := strconv.Atoi(fields[1])
			pSquare, _ := strconv.Atoi(fields[2])
			pLong, _ := strconv.Atoi(fields[3])
			pHuge, _ := strconv.Atoi(fields[4])
			if pSmall != 600 {
				t.Errorf("pSmall = %d, want 600", pSmall)
			}
			if pSquare > 4096 || pSquare < 4000 {
				t.Errorf("pSquare = %d, want <= 4096 and near 4096", pSquare)
			}
			if pLong > 16384 || pLong < 16000 {
				t.Errorf("pLong = %d, want <= 16384 and near 16384", pLong)
			}
			if pHuge != 5000 {
				t.Errorf("pHuge = %d, want 5000", pHuge)
			}
		}
	}
	if !strings.Contains(string(out), "CANVAS 0.5 1.0 ") {
		t.Errorf("no canvas scale line, or a wrong one, in %s", out)
	}
	for _, line := range lines {
		if f := strings.Fields(line); f[0] == "CANVAS" {
			square, _ := strconv.ParseFloat(f[3], 64)
			if math.Abs(square-1) > 0.01 || f[4] != "1.0" || f[5] != "2.0" || f[6] != "150" || f[7] != "1" {
				t.Errorf("canvas scales %v, want 0.5 1.0 ~1 1.0 2.0 150 1", f[1:])
			}
		}
	}
	if stepCount != 16 {
		t.Fatalf("checked %d steps, want 16", stepCount)
	}
}
