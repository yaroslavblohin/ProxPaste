package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHotkeyParsing(t *testing.T) {
	for _, sample := range []struct {
		text string
		want Hotkey
	}{
		{"ctrl + shift + v", Hotkey{modCtrl | modShift, "V"}},
		{"Control+Option+F12", Hotkey{modCtrl | modAlt, "F12"}},
		{"Command+Shift+9", Hotkey{modSuper | modShift, "9"}},
		{"Alt+F", Hotkey{modAlt, "F"}},
	} {
		got, err := parseHotkey(sample.text)
		if err != nil || got != sample.want {
			t.Fatalf("%q: %+v %v", sample.text, got, err)
		}
	}
	for _, text := range []string{"V", "Shift+A", "Ctrl+Ctrl+A", "Ctrl+Esc", "Alt+F13", "Ctrl+Я", "Ctrl+", "Ctrl+Shift", "Hyper+V"} {
		if _, err := parseHotkey(text); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}
func TestSettingsRoundTripAndCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ProxPaste", "settings.json")
	h, err := loadHotkey(path)
	if err != nil || h != defaultHotkey {
		t.Fatal(h, err)
	}
	for _, want := range []Hotkey{{modCtrl | modShift, "F8"}, {modAlt, "J"}} {
		if err := saveHotkey(path, want); err != nil {
			t.Fatal(err)
		}
		got, err := loadHotkey(path)
		if err != nil || got != want {
			t.Fatal(got, err)
		}
	}
	for _, bad := range []string{`{"hotkey":{"modifiers":32,"key":"V"}}`, `{`, `{"hotkey":{"modifiers":1,"key":"ESC"}}`} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadHotkey(path); err == nil {
			t.Fatal("accepted corrupt settings")
		}
	}
}
func TestChangeHotkeyRegistrationFailure(t *testing.T) {
	old := selectedHotkey
	err := changeHotkey("Ctrl+Shift+K", func(Hotkey) (func(), func(), error) { return nil, nil, errors.New("occupied") })
	if err == nil || selectedHotkey != old {
		t.Fatal("registration failure changed active hotkey")
	}
}
func TestChangeHotkeySaveFailureRollsBack(t *testing.T) {
	// All supported platforms use one of these roots; point both at a file.
	root := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(root, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	previous := userConfigDir
	userConfigDir = func() (string, error) { return root, nil }
	t.Cleanup(func() { userConfigDir = previous })
	old := selectedHotkey
	committed, rolledBack := false, false
	err := changeHotkey("Ctrl+Shift+K", func(Hotkey) (func(), func(), error) {
		return func() { committed = true }, func() { rolledBack = true }, nil
	})
	if err == nil || committed || !rolledBack || selectedHotkey != old {
		t.Fatal("failed persistence did not preserve previous binding")
	}
}

func TestAllSettingsPersistenceAndStartup(t *testing.T) {
	root := t.TempDir()
	previousDir := userConfigDir
	oldSettings, oldHotkey := currentSettings, selectedHotkey
	userConfigDir = func() (string, error) { return root, nil }
	t.Cleanup(func() { userConfigDir = previousDir; currentSettings = oldSettings; selectedHotkey = oldHotkey })
	want := Settings{Hotkey{modCtrl | modShift, "F9"}, 12, 180, true}
	committed := false
	err := changeSettings(want.Hotkey.String(), "12", "180", true, func(Hotkey) (func(), func(), error) {
		return func() { committed = true }, func() { t.Error("unexpected rollback") }, nil
	})
	if err != nil || !committed || currentSettings != want {
		t.Fatalf("save: %+v %v", currentSettings, err)
	}
	currentSettings = defaultSettings()
	selectedHotkey = defaultHotkey
	initializeSettings()
	if currentSettings != want || selectedHotkey != want.Hotkey {
		t.Fatalf("restart: %+v", currentSettings)
	}
	path, _ := settingsPath()
	want.AllowEnterTab = false
	if err := saveSettings(path, want); err != nil {
		t.Fatal(err)
	}
	initializeSettings()
	if currentSettings != want {
		t.Fatal("false checkbox not restored")
	}
}
func TestLegacySettingsMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"hotkey":{"modifiers":5,"key":"K"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := loadSettings(path)
	want := Settings{Hotkey{modCtrl | modShift, "K"}, 5, 40, false}
	if err != nil || got != want {
		t.Fatalf("legacy migration: %+v %v", got, err)
	}
}
func TestInvalidSettingsDoNotReplaceSavedValues(t *testing.T) {
	root := t.TempDir()
	previous := userConfigDir
	userConfigDir = func() (string, error) { return root, nil }
	t.Cleanup(func() { userConfigDir = previous })
	path, _ := settingsPath()
	initial := Settings{defaultHotkey, 6, 75, true}
	if err := saveSettings(path, initial); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"", "40"}, {"-1", "40"}, {"61", "40"}, {"5", "-1"}, {"5", "1001"}, {"2.5", "40"}} {
		err := changeSettings(defaultHotkey.String(), pair[0], pair[1], false, func(Hotkey) (func(), func(), error) {
			t.Fatal("invalid values reached registration")
			return nil, nil, nil
		})
		if err == nil {
			t.Fatalf("accepted %v", pair)
		}
		got, err := loadSettings(path)
		if err != nil || got != initial {
			t.Fatal("invalid values changed saved settings")
		}
	}
}

func TestZeroTimingsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := Settings{Hotkey: defaultHotkey, DelaySeconds: 0, IntervalMS: 0}
	if err := saveSettings(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadSettings(path)
	if err != nil || got != want {
		t.Fatalf("zero timings replaced by defaults: %+v %v", got, err)
	}
	want.DelaySeconds = 1
	want.IntervalMS = 1
	if err := validateSettings(want); err != nil {
		t.Fatal(err)
	}
}
