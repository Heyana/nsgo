package android

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestLocalePrefsJava exercises LocalePrefs.java on the JVM across Unicode
// extensions, DateFormat.is24HourFormat, ICU measurement systems and region
// fallbacks, and decodes its wire output through Go's readEnvironment.
func TestLocalePrefsJava(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	args := []string{
		"-d", dir,
		filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"),
		filepath.Join("java", "dev", "oneo", "nsgo", "LocalePrefs.java"),
		filepath.Join("testdata", "LocalePrefsCheck.java"),
	}
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.LocalePrefsCheck").CombinedOutput(); err != nil {
		t.Fatalf("LocalePrefsCheck: %v\n%s", err, out)
	}

	for _, c := range []struct {
		tag  string
		h24  string
		icu  string
		want driver.LocaleSettings
	}{
		{
			"en-US-u-fw-mon-mu-celsius-ms-metric", "true", "us",
			driver.LocaleSettings{FirstDayOfWeek: 1, HourCycle: "h23", MeasurementSystem: "metric", TemperatureUnit: "celsius", Region: "US"},
		},
		{
			"en-US-u-fw-sun-mu-fahrenhe-ms-ussystem", "false", "metric",
			driver.LocaleSettings{FirstDayOfWeek: 7, HourCycle: "h12", MeasurementSystem: "us", TemperatureUnit: "fahrenheit", Region: "US"},
		},
		{
			"en-GB-u-ms-uksystem", "true", "metric",
			driver.LocaleSettings{FirstDayOfWeek: 1, HourCycle: "h23", MeasurementSystem: "uk", Region: "GB"},
		},
		{
			"de-DE", "false", "",
			driver.LocaleSettings{FirstDayOfWeek: 1, HourCycle: "h12", MeasurementSystem: "metric", Region: "DE"},
		},
	} {
		out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.LocalePrefsCheck", "encode", c.tag, c.h24, c.icu).Output()
		if err != nil {
			t.Fatalf("encode %s: %v", c.tag, err)
		}
		var w writer
		writeBaseEnvironment(&w, c.tag)
		w.buf = append(w.buf, out...)
		r := reader{w.buf}
		env := readEnvironment(&r)
		if len(r.buf) != 0 {
			t.Errorf("%s: %d bytes left unread", c.tag, len(r.buf))
		}
		if env.LocaleSettings != c.want {
			t.Errorf("%s: LocaleSettings = %+v, want %+v", c.tag, env.LocaleSettings, c.want)
		}
	}
}

func TestReadEnvironmentLocaleSettings(t *testing.T) {
	t.Run("full payload", func(t *testing.T) {
		var w writer
		writeBaseEnvironment(&w, "en-US-u-fw-mon-mu-celsius")
		w.byte(1)
		w.str("h23")
		w.str("metric")
		w.str("celsius")
		w.str("US")
		r := reader{w.buf}
		env := readEnvironment(&r)
		want := driver.LocaleSettings{
			FirstDayOfWeek:    1,
			HourCycle:         "h23",
			MeasurementSystem: "metric",
			TemperatureUnit:   "celsius",
			Region:            "US",
		}
		if len(r.buf) != 0 {
			t.Errorf("%d bytes left unread", len(r.buf))
		}
		if env.LocaleSettings != want {
			t.Errorf("LocaleSettings = %+v, want %+v", env.LocaleSettings, want)
		}
	})

	t.Run("out of range first weekday becomes zero", func(t *testing.T) {
		for _, bad := range []byte{0, 8, 255} {
			var w writer
			writeBaseEnvironment(&w, "en-US")
			w.byte(bad)
			w.str("h12")
			w.str("us")
			w.str("fahrenheit")
			w.str("US")
			env := readEnvironment(&reader{w.buf})
			if env.LocaleSettings.FirstDayOfWeek != 0 {
				t.Errorf("first byte %d: FirstDayOfWeek = %d, want 0", bad, env.LocaleSettings.FirstDayOfWeek)
			}
			if env.LocaleSettings.HourCycle != "h12" || env.LocaleSettings.MeasurementSystem != "us" || env.LocaleSettings.TemperatureUnit != "fahrenheit" || env.LocaleSettings.Region != "US" {
				t.Errorf("first byte %d: unexpected LocaleSettings %+v", bad, env.LocaleSettings)
			}
		}
	})

	t.Run("legacy payload without locale settings stays zero", func(t *testing.T) {
		var w writer
		writeBaseEnvironment(&w, "en-US")
		env := readEnvironment(&reader{w.buf})
		if env.LocaleSettings != (driver.LocaleSettings{}) {
			t.Errorf("legacy payload: LocaleSettings = %+v, want zero", env.LocaleSettings)
		}
	})

	t.Run("third capability byte is read when the runtime writes it", func(t *testing.T) {
		want := driver.LocaleSettings{FirstDayOfWeek: 1, HourCycle: "h23", MeasurementSystem: "metric", TemperatureUnit: "celsius", Region: "US"}
		for _, third := range [][]byte{nil, {0}, {255}} {
			var w writer
			writeBaseEnvironment(&w, "en-US")
			w.byte(1)
			w.str("h23")
			w.str("metric")
			w.str("celsius")
			w.str("US")
			w.buf = append(w.buf, third...)
			r := reader{w.buf}
			env := readEnvironment(&r)
			if len(r.buf) != 0 {
				t.Errorf("third byte %v: %d bytes left unread", third, len(r.buf))
			}
			if env.LocaleSettings != want {
				t.Errorf("third byte %v: LocaleSettings = %+v, want %+v", third, env.LocaleSettings, want)
			}
		}
	})

	t.Run("resize carries locale settings", func(t *testing.T) {
		var w writer
		w.floats(412, 915)
		w.u32(0)
		writeBaseEnvironment(&w, "en-GB")
		w.byte(1)
		w.str("h23")
		w.str("uk")
		w.str("celsius")
		w.str("GB")
		r := reader{w.buf}
		_, _, env := readResize(&r)
		if len(r.buf) != 0 || env == nil {
			t.Fatalf("readResize left %d bytes, env %+v", len(r.buf), env)
		}
		want := driver.LocaleSettings{FirstDayOfWeek: 1, HourCycle: "h23", MeasurementSystem: "uk", TemperatureUnit: "celsius", Region: "GB"}
		if env.LocaleSettings != want {
			t.Errorf("readResize LocaleSettings = %+v, want %+v", env.LocaleSettings, want)
		}
	})
}

func writeBaseEnvironment(w *writer, locale string) {
	w.floats(0, 0, 0, 0, 0, 1, 2.5)
	w.byte(0)
	w.byte(0)
	w.byte(byte(driver.IdiomPhone))
	w.str(locale)
	w.str("Pixel")
	w.str("Google Pixel 9")
	w.floats(56, 96)
	w.u32(35)
	// The thermal state and low-power mode bytes NsgoView writes before the locale settings.
	w.byte(0)
	w.byte(0)
}
