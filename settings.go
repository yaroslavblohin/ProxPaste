package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Modifier bits are shared by both platform adapters, not OS key codes.
const (
	modCtrl = 1 << iota
	modAlt
	modShift
	modSuper
)

type Hotkey struct {
	Modifiers uint32 `json:"modifiers"`
	Key       string `json:"key"`
}

var defaultHotkey = Hotkey{modCtrl | modAlt, "V"}
var selectedHotkey = defaultHotkey
var userConfigDir = os.UserConfigDir

type Settings struct {
	Hotkey        Hotkey `json:"hotkey"`
	DelaySeconds  int    `json:"delay_seconds"`
	IntervalMS    int    `json:"interval_ms"`
	AllowEnterTab bool   `json:"allow_enter_tab"`
}

func defaultSettings() Settings {
	return Settings{Hotkey: defaultHotkey, DelaySeconds: 5, IntervalMS: 40}
}

var currentSettings = defaultSettings()

func validateSettings(cfg Settings) error {
	if cfg.Hotkey.Modifiers & ^uint32(15) != 0 {
		return fmt.Errorf("неверные модификаторы")
	}
	if _, err := parseHotkey(cfg.Hotkey.String()); err != nil {
		return err
	}
	if cfg.DelaySeconds < 0 || cfg.DelaySeconds > 60 || cfg.IntervalMS < 0 || cfg.IntervalMS > 1000 {
		return fmt.Errorf("Задержка: 0–60 сек. Интервал: 0–1000 мс.")
	}
	return nil
}

func parseHotkey(text string) (Hotkey, error) {
	parts := strings.Split(strings.TrimSpace(text), "+")
	var h Hotkey
	if len(parts) < 2 {
		return h, fmt.Errorf("Укажите модификатор и клавишу, например Ctrl+Alt+V")
	}
	for _, part := range parts[:len(parts)-1] {
		var bit uint32
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "ctrl", "control":
			bit = modCtrl
		case "alt", "option":
			bit = modAlt
		case "shift":
			bit = modShift
		case "win", "cmd", "command", "super":
			bit = modSuper
		default:
			return h, fmt.Errorf("Неизвестный модификатор: %s", part)
		}
		if h.Modifiers&bit != 0 {
			return h, fmt.Errorf("Модификатор указан дважды: %s", part)
		}
		h.Modifiers |= bit
	}
	h.Key = strings.ToUpper(strings.TrimSpace(parts[len(parts)-1]))
	valid := len(h.Key) == 1 && ((h.Key[0] >= 'A' && h.Key[0] <= 'Z') || (h.Key[0] >= '0' && h.Key[0] <= '9'))
	if strings.HasPrefix(h.Key, "F") {
		n, err := strconv.Atoi(h.Key[1:])
		if err == nil && n >= 1 && n <= 12 {
			h.Key = fmt.Sprintf("F%d", n)
			valid = true
		}
	}
	if !valid {
		return h, fmt.Errorf("Выберите A–Z, 0–9 или F1–F12; Esc зарезервирован для остановки")
	}
	if h.Modifiers&(modCtrl|modAlt|modSuper) == 0 {
		return h, fmt.Errorf("Добавьте Ctrl, Alt/Option или Win/Command, чтобы не перехватывать обычный ввод")
	}
	return h, nil
}
func (h Hotkey) String() string {
	var parts []string
	if h.Modifiers&modCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if h.Modifiers&modAlt != 0 {
		name := "Alt"
		if runtime.GOOS == "darwin" {
			name = "Option"
		}
		parts = append(parts, name)
	}
	if h.Modifiers&modShift != 0 {
		parts = append(parts, "Shift")
	}
	if h.Modifiers&modSuper != 0 {
		name := "Win"
		if runtime.GOOS == "darwin" {
			name = "Command"
		}
		parts = append(parts, name)
	}
	return strings.Join(append(parts, h.Key), "+")
}
func settingsPath() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ProxPaste", "settings.json"), nil
}
func loadHotkey(path string) (Hotkey, error) {
	cfg, err := loadSettings(path)
	return cfg.Hotkey, err
}
func loadSettings(path string) (Settings, error) {
	cfg := defaultSettings()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	// Start with defaults so files from versions storing only a hotkey migrate.
	if err = json.Unmarshal(data, &cfg); err != nil {
		return defaultSettings(), err
	}
	if err = validateSettings(cfg); err != nil {
		return defaultSettings(), err
	}
	cfg.Hotkey, _ = parseHotkey(cfg.Hotkey.String())
	return cfg, nil
}
func saveHotkey(path string, h Hotkey) error {
	cfg, err := loadSettings(path)
	if err != nil {
		return err
	}
	cfg.Hotkey = h
	return saveSettings(path, cfg)
}
func saveSettings(path string, cfg Settings) error {
	if err := validateSettings(cfg); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "settings-*.tmp")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func initializeSettings() {
	path, err := settingsPath()
	if err == nil {
		currentSettings, err = loadSettings(path)
		selectedHotkey = currentSettings.Hotkey
	}
	if err != nil {
		setStatus("Не удалось прочитать настройки. Используются значения по умолчанию.")
	}
}

// Register before saving. On any failure the old registration stays active.
// Platform code supplies a transaction, committing only after disk write.
func changeHotkey(text string, prepare func(Hotkey) (commit, rollback func(), err error)) error {
	return changeSettings(text, strconv.Itoa(currentSettings.DelaySeconds), strconv.Itoa(currentSettings.IntervalMS), currentSettings.AllowEnterTab, prepare)
}
func changeSettings(text, delayText, intervalText string, allow bool, prepare func(Hotkey) (commit, rollback func(), err error)) error {
	if busy, _ := snapshot(); busy {
		return fmt.Errorf("Дождитесь окончания ввода")
	}
	h, err := parseHotkey(text)
	if err != nil {
		return err
	}
	delay, e1 := strconv.Atoi(strings.TrimSpace(delayText))
	interval, e2 := strconv.Atoi(strings.TrimSpace(intervalText))
	if e1 != nil || e2 != nil {
		return fmt.Errorf("Задержка и интервал должны быть целыми числами")
	}
	cfg := Settings{h, delay, interval, allow}
	if err := validateSettings(cfg); err != nil {
		return err
	}
	commit, rollback, err := prepare(h)
	if err != nil {
		return err
	}
	path, err := settingsPath()
	if err == nil {
		err = saveSettings(path, cfg)
	}
	if err != nil {
		rollback()
		return fmt.Errorf("Не удалось сохранить настройки: %w", err)
	}
	commit()
	selectedHotkey = h
	currentSettings = cfg
	setStatus("Все настройки сохранены.")
	return nil
}
