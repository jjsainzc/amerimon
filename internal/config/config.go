package config

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.yaml.in/yaml/v3"
)

// App es la configuración global compartida por los controladores.
var App Config
var amiConfigMu sync.RWMutex

func CurrentAMI() AMIConfig {
	amiConfigMu.RLock()
	defer amiConfigMu.RUnlock()
	return App.AMI
}

func SetAMI(value AMIConfig) {
	amiConfigMu.Lock()
	defer amiConfigMu.Unlock()
	App.AMI = value
}

type Config struct {
	Version string `yaml:"-"`
	Path    string `yaml:"-"`
	EnvPath string `yaml:"-"`
	DB      *sql.DB
	UseDB   bool
	Sources SourcesConfig `yaml:"sources"`
	AMI     AMIConfig     `yaml:"ami"`
}

type SourcesConfig struct {
	AstDBPath        string `yaml:"astdb_path"`
	AstDBUrl         string `yaml:"astdb_url"`
	PrivateNodesPath string `yaml:"privatenodes_path"`
	// AllmonINIPath fuerza un allmon.ini; vacío = buscar en /etc/allmon3, /var/www/html/supermon y el propio.
	AllmonINIPath string `yaml:"allmon_ini_path"`
	// AllmonINIOwnPath es el allmon.ini que edita el admin desde la web.
	AllmonINIOwnPath string `yaml:"allmon_ini_own_path"`
}

// AMIConfig reúne las credenciales del AMI de Asterisk y el puerto interno de listen-live.
type AMIConfig struct {
	// ListenPort: puerto TCP local donde EAGI entrega el audio (por defecto 8086).
	ListenPort           int    `yaml:"listen_port"`
	RPTConfigPath        string `yaml:"rpt_config_path"`
	PJSIPConfigPath      string `yaml:"pjsip_config_path"`
	ActivityPollInterval string `yaml:"activity_poll_interval"`
	Host                 string `yaml:"host"`
	Port                 string `yaml:"port"`
	User                 string `yaml:"user"`
	Secret               string `yaml:"secret"`
}

// Load lee el YAML (se expanden ${VAR}) y deja el resultado en App. Un archivo inexistente usa los valores por defecto.
func Load(path string) error {
	cfg := Config{
		Version: "dev",
		Sources: SourcesConfig{
			AstDBPath:        "/var/lib/asterisk/astdb.txt",
			PrivateNodesPath: "/var/lib/asterisk/privatenodes.txt",
			AllmonINIOwnPath: "/etc/amerimon/allmon.ini",
		},
		AMI: AMIConfig{
			ListenPort:           8086,
			RPTConfigPath:        "/etc/asterisk/rpt.conf",
			PJSIPConfigPath:      "/etc/asterisk/pjsip.conf",
			ActivityPollInterval: "5s",
			Host:                 "127.0.0.1",
			Port:                 "5038",
		},
	}

	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(data))), &cfg); err != nil {
				return fmt.Errorf("config %s: %w", path, err)
			}
		case !os.IsNotExist(err):
			return err
		}
	}

	App = cfg
	App.Path = path
	App.EnvPath = os.Getenv("AMERIMON_ENV_FILE")
	if App.EnvPath == "" && filepath.Clean(filepath.Dir(path)) == "/etc/amerimon" {
		App.EnvPath = filepath.Join(filepath.Dir(path), "amerimon.env")
	}
	return nil
}
