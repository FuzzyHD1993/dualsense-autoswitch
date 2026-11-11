package main

// dualsense-autoswitch — переключение профилей DualSense по запущенной игре.
// Замена bash-демонам dualsense-autoswitch и dualsense-game-auto.
// Сканирует /proc напрямую, без форков; эффекты применяет скриптами профилей.

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type rule struct {
	pattern string
	profile string
}

var (
	home     string
	confPath string
	overPath string
	offPath  string
	binDir   string
	interval time.Duration
	reapply  time.Duration
)

func logf(format string, a ...any) {
	fmt.Printf("%s %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
}

func loadRules() []rule {
	b, err := os.ReadFile(confPath)
	if err != nil {
		return nil
	}

	var rules []rule
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		pat := strings.TrimSpace(parts[0])
		prof := strings.TrimSpace(parts[1])
		if pat == "" || prof == "" {
			continue
		}
		rules = append(rules, rule{pattern: pat, profile: prof})
	}
	return rules
}

func profilesDisabled() bool {
	_, err := os.Stat(offPath)
	return err == nil
}

func readOverride() string {
	b, err := os.ReadFile(overPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// processNames собирает имена процессов: comm и basename из cmdline (для wine/Proton).
func processNames() map[string]struct{} {
	names := make(map[string]struct{}, 512)
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return names
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		if pid[0] < '0' || pid[0] > '9' {
			continue
		}
		base := "/proc/" + pid
		if b, err := os.ReadFile(base + "/comm"); err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				names[s] = struct{}{}
			}
		}
		if b, err := os.ReadFile(base + "/cmdline"); err == nil && len(b) > 0 {
			if i := bytes.IndexByte(b, 0); i >= 0 {
				b = b[:i]
			}
			if s := filepath.Base(string(b)); s != "" && s != "." && s != "/" {
				names[s] = struct{}{}
			}
		}
	}
	return names
}

func matchProfile(rules []rule, names map[string]struct{}) string {
	for _, r := range rules {
		pat := strings.ToLower(r.pattern)
		for n := range names {
			ok, err := filepath.Match(pat, strings.ToLower(n))
			if err != nil {
				continue
			}
			if ok {
				return r.profile
			}
		}
	}
	return "default"
}

// controllerPresent ищет DualSense через sysfs, не запуская dualsensectl.
func controllerPresent() bool {
	files, err := filepath.Glob("/sys/class/hidraw/hidraw*/device/uevent")
	if err != nil {
		return false
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		s := string(b)
		// Sony = 054C; Bluetooth bus 0005, USB 0003
		if strings.Contains(s, ":0000054C:") ||
			strings.Contains(s, "DualSense") ||
			strings.Contains(s, "Sony Interactive") {
			return true
		}
	}
	return false
}

func apply(profile string) error {
	script := filepath.Join(binDir, "dualsense-profile-"+profile)
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("нет скрипта профиля: %s", script)
	}
	out, err := exec.Command(script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ошибка: %v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func main() {
	h, _ := os.UserHomeDir()
	home = h

	once := flag.Bool("once", false, "один проход: определить профиль и выйти")
	verbose := flag.Bool("v", false, "печатать выбранный профиль каждый цикл")
	flag.Parse()

	confPath = envOr("DS_CONF", filepath.Join(home, ".config/dualsense/profiles.conf"))
	overPath = envOr("DS_OVERRIDE", filepath.Join(home, ".config/dualsense/override"))
	offPath = envOr("DS_OFF", filepath.Join(home, ".config/dualsense/disabled"))
	binDir = envOr("DS_BIN", filepath.Join(home, ".local/bin"))
	interval = 2 * time.Second
	reapply = 5 * time.Second
	if v := os.Getenv("DS_REAPPLY_SEC"); v != "" {
		var sec int
		if _, err := fmt.Sscanf(v, "%d", &sec); err == nil && sec >= 0 {
			reapply = time.Duration(sec) * time.Second
		}
	}
	if v := os.Getenv("DS_INTERVAL_MS"); v != "" {
		var ms int
		if _, err := fmt.Sscanf(v, "%d", &ms); err == nil && ms >= 200 {
			interval = time.Duration(ms) * time.Millisecond
		}
	}

	rules := loadRules()
	names := processNames()
	current := matchProfile(rules, names)

	if o := readOverride(); o != "" {
		current = o
	}

	if profilesDisabled() {
		current = "__off__"
	}
	if *once {
		fmt.Println(current)
		return
	}

	logf("autoswitch запущен (Go): профилей %d, интервал %s", len(rules), interval)

	// первичное применение
	if err := apply(current); err != nil {
		logf("profile=%s: %v", current, err)
	} else {
		logf("profile=%s применён", current)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastController := controllerPresent()
	lastApply := time.Now()

	for {
		select {
		case s := <-sig:
			if s == syscall.SIGHUP {
				rules = loadRules()
				logf("конфиг перечитан: профилей %d", len(rules))
				continue
			}
			logf("остановка")
			return
		case <-ticker.C:
		}

		// переподключение геймпада -> переприменить текущий профиль
		present := controllerPresent()
		if present && !lastController {
			logf("контроллер подключён — переприменяю %s", current)
			if err := apply(current); err != nil {
				logf("profile=%s: %v", current, err)
			}
		}
		lastController = present

		if profilesDisabled() {
			if current != "__off__" {
				current = "__off__"
				_ = apply("default")
				logf("профили ОТКЛЮЧЕНЫ — управление у игры")
			}
			continue
		}
		if current == "__off__" {
			current = ""
			logf("профили ВКЛЮЧЕНЫ")
		}

		names = processNames()
		matched := matchProfile(rules, names)
		if o := readOverride(); o != "" {
			matched = o
		}
		if matched != current {
			current = matched
			if err := apply(current); err != nil {
				logf("profile=%s: %v", current, err)
			} else {
				lastApply = time.Now()
				logf("profile=%s применён", current)
			}
		} else if matched != "default" && reapply > 0 && time.Since(lastApply) >= reapply && present {
			// игра может сбрасывать эффекты — тихо возвращаем свои
			if err := apply(current); err == nil {
				lastApply = time.Now()
			}
		} else if *verbose {
			logf("profile=%s (без изменений)", current)
		}
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
