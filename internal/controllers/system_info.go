package controllers

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

type SystemInfo struct {
	OS              SystemOS            `json:"os"`
	CPU             SystemCPU           `json:"cpu"`
	Memory          SystemMemory        `json:"mem"`
	Storage         []SystemStorage     `json:"storage"`
	Network         []SystemNetwork     `json:"network"`
	Traffic         SystemTraffic       `json:"traffic"`
	TopProcesses    []SystemProcess     `json:"top_proc"`
	Temperature     []SystemTemperature `json:"temperature"`
	Voltage         string              `json:"voltage"`
	AsteriskVersion string              `json:"asterisk_version"`
}

type SystemOS struct {
	Hostname     string `json:"hostname"`
	Platform     string `json:"platform"`
	Distribution string `json:"distribution"`
	Version      string `json:"version"`
	Architecture string `json:"os_arch"`
	Kernel       string `json:"kernel"`
	Uptime       string `json:"uptime"`
}

type SystemCPU struct {
	Cores    int     `json:"cores"`
	Logicals int     `json:"logicals"`
	Model    string  `json:"model"`
	Speed    string  `json:"speed"`
	Used     string  `json:"used"`
	Load1    float64 `json:"load1"`
	Load5    float64 `json:"load5"`
	Load15   float64 `json:"load15"`
}

type SystemMemory struct {
	Total       string `json:"total"`
	Available   string `json:"available"`
	Used        string `json:"used"`
	UsedPercent string `json:"used_percent"`
}

type SystemStorage struct {
	Mountpoint  string `json:"mountpoint"`
	FSType      string `json:"fs_type"`
	Total       string `json:"total"`
	Free        string `json:"free"`
	UsedPercent string `json:"used_percent"`
	Swap        string `json:"swap"`
}

type SystemNetwork struct {
	Device      string   `json:"device"`
	MACAddress  string   `json:"mac_address"`
	Status      string   `json:"status"`
	IPAddresses []string `json:"ip_address"`
}

type SystemTraffic struct {
	BytesRecv uint64 `json:"bytes_recv"`
	BytesSent uint64 `json:"bytes_sent"`
}

type SystemProcess struct {
	PID        int32   `json:"PID"`
	Name       string  `json:"Nombre"`
	CPUPercent float64 `json:"CPUPercent"`
}

type SystemTemperature struct {
	Zone    string  `json:"zona"`
	Type    string  `json:"tipo"`
	Celsius float64 `json:"celsius"`
}

// FormatUptime formatea una duración en una cadena legible.
func formatUptime(d time.Duration) string {
	d = d.Round(time.Second)

	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// GetSystemInfo recopila métricas del sistema y Asterisk disponibles y las devuelve como JSON.
func GetSystemInfo(c echo.Context) error {
	info := SystemInfo{
		Storage:         make([]SystemStorage, 0),
		Network:         make([]SystemNetwork, 0),
		TopProcesses:    make([]SystemProcess, 0),
		Temperature:     readSystemTemperatures(),
		Voltage:         readSystemVoltage(),
		AsteriskVersion: readAsteriskVersion(),
	}

	if hostInfo, err := host.Info(); err == nil {
		info.OS = SystemOS{
			Hostname: hostInfo.Hostname, Platform: hostInfo.Platform,
			Distribution: hostInfo.Platform, Version: hostInfo.PlatformVersion,
			Architecture: hostInfo.KernelArch, Kernel: hostInfo.KernelVersion,
			Uptime: formatUptime(time.Duration(hostInfo.Uptime) * time.Second),
		}
	}

	physical, _ := cpu.Counts(false)
	logical, _ := cpu.Counts(true)
	cpuInfo, _ := cpu.Info()
	cpuUsage, _ := cpu.Percent(200*time.Millisecond, false)
	cpuLoad, _ := load.Avg()
	info.CPU = SystemCPU{Cores: physical, Logicals: logical}
	if len(cpuInfo) > 0 {
		info.CPU.Model = cpuInfo[0].ModelName
		info.CPU.Speed = fmt.Sprintf("%.0f MHz", cpuInfo[0].Mhz)
	}
	if len(cpuUsage) > 0 {
		info.CPU.Used = fmt.Sprintf("%.2f%%", cpuUsage[0])
	}
	if cpuLoad != nil {
		info.CPU.Load1, info.CPU.Load5, info.CPU.Load15 = cpuLoad.Load1, cpuLoad.Load5, cpuLoad.Load15
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		info.Memory = SystemMemory{
			Total: formatGB(vm.Total), Available: formatGB(vm.Available),
			Used: formatGB(vm.Used), UsedPercent: fmt.Sprintf("%.2f%%", vm.UsedPercent),
		}
	}
	swap, _ := mem.SwapMemory()
	if partitions, err := disk.Partitions(false); err == nil {
		for _, partition := range partitions {
			if partition.Mountpoint != "/" {
				continue
			}
			usage, err := disk.Usage(partition.Mountpoint)
			if err != nil {
				continue
			}
			swapText := "—"
			if swap != nil {
				swapText = fmt.Sprintf("%s (%.1f%%)", formatGB(swap.Total), swap.UsedPercent)
			}
			info.Storage = append(info.Storage, SystemStorage{
				Mountpoint: partition.Mountpoint, FSType: partition.Fstype,
				Total: formatGB(usage.Total), Free: formatGB(usage.Free),
				UsedPercent: fmt.Sprintf("%.2f%%", usage.UsedPercent), Swap: swapText,
			})
		}
	}
	if interfaces, err := net.Interfaces(); err == nil {
		for _, item := range interfaces {
			ips := make([]string, 0, len(item.Addrs))
			for _, address := range item.Addrs {
				ips = append(ips, address.Addr)
			}
			info.Network = append(info.Network, SystemNetwork{Device: item.Name, MACAddress: item.HardwareAddr, Status: fmt.Sprint(item.Flags), IPAddresses: ips})
		}
	}
	if counters, err := net.IOCounters(false); err == nil && len(counters) > 0 {
		info.Traffic = SystemTraffic{BytesRecv: counters[0].BytesRecv, BytesSent: counters[0].BytesSent}
	}
	info.TopProcesses = readTopProcesses()
	return c.JSON(http.StatusOK, info)
}

// readAsteriskVersion consulta la versión por AMI y devuelve una cadena vacía si no puede obtenerla.
func readAsteriskVersion() string {
	ami := config.App.AMI
	if ami.Host == "" || ami.Port == "" || ami.User == "" || ami.Secret == "" {
		return ""
	}
	raw, err := config.ExecuteAMICommand(ami.Host, ami.Port, ami.User, ami.Secret, "core show version", 3*time.Second)
	if err != nil {
		return ""
	}
	versionPattern := regexp.MustCompile(`Asterisk\s+([0-9][^\s]*)`)
	if match := versionPattern.FindStringSubmatch(raw); len(match) > 1 {
		return match[1]
	}
	return ""
}

// readSystemTemperatures lee las zonas térmicas accesibles de sysfs y omite las que no puede interpretar.
func readSystemTemperatures() []SystemTemperature {
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	readings := make([]SystemTemperature, 0, len(zones))
	for _, zone := range zones {
		tempRaw, err := os.ReadFile(filepath.Join(zone, "temp"))
		if err != nil {
			continue
		}
		milliCelsius, err := strconv.ParseFloat(strings.TrimSpace(string(tempRaw)), 64)
		if err != nil {
			continue
		}
		typeRaw, _ := os.ReadFile(filepath.Join(zone, "type"))
		readings = append(readings, SystemTemperature{
			Zone: filepath.Base(zone), Type: strings.TrimSpace(string(typeRaw)), Celsius: milliCelsius / 1000,
		})
	}
	return readings
}

// readSystemVoltage obtiene el voltaje con vcgencmd en ARM o desde hwmon; devuelve vacío si no está disponible.
func readSystemVoltage() string {
	if runtime.GOARCH == "arm" || runtime.GOARCH == "arm64" {
		output, err := exec.Command("vcgencmd", "measure_volts", "core").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(output)), "volt="))
	}
	paths, _ := filepath.Glob("/sys/class/hwmon/hwmon*/in0_input")
	var fallback string
	for _, path := range paths {
		nameBytes, err := os.ReadFile(filepath.Join(filepath.Dir(path), "name"))
		if err != nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(string(nameBytes)))
		if strings.Contains(name, "k10temp") || strings.Contains(name, "zenpower") || strings.HasPrefix(name, "nct") || strings.HasPrefix(name, "it") || strings.Contains(name, "asus") {
			fallback = path
			break
		}
		fallback = path
	}
	if fallback == "" {
		return ""
	}
	raw, err := os.ReadFile(fallback)
	if err != nil {
		return ""
	}
	millivolts, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil {
		return ""
	}
	if millivolts >= 0 && millivolts < 3 {
		return fmt.Sprintf("%.3fV", millivolts)
	}
	return fmt.Sprintf("%.3fV", millivolts/1000)
}

// readTopProcesses muestrea el uso de CPU, ordena los procesos y devuelve como máximo los diez primeros.
func readTopProcesses() []SystemProcess {
	processes, err := process.Processes()
	if err != nil {
		return []SystemProcess{}
	}
	for _, item := range processes {
		_, _ = item.CPUPercent()
	}
	time.Sleep(100 * time.Millisecond)
	result := make([]SystemProcess, 0, 10)
	for _, item := range processes {
		percent, err := item.CPUPercent()
		if err != nil || percent <= 0 {
			continue
		}
		name, _ := item.Name()
		result = append(result, SystemProcess{PID: item.Pid, Name: name, CPUPercent: percent})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CPUPercent > result[j].CPUPercent })
	if len(result) > 10 {
		result = result[:10]
	}
	return result
}

// formatGB convierte una cantidad de bytes a una cadena en gigabytes binarios con dos decimales.
func formatGB(bytes uint64) string {
	return fmt.Sprintf("%.2f GB", float64(bytes)/1024/1024/1024)
}
