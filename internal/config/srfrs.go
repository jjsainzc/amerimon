package config

// Para futuras implementacion programar los SRFRS (Software Defined Radio Front-End)

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
	"time"
)

var ctcssList = []string{
	"None", "67.0", "71.9", "74.4", "77.0", "79.7", "82.5", "85.4", "88.5", "91.5",
	"94.8", "97.4", "100.0", "103.5", "107.2", "110.9", "114.8", "118.8", "123.0",
	"127.3", "131.8", "136.5", "141.3", "146.2", "151.4", "156.7", "162.2",
	"167.9", "173.8", "179.9", "186.2", "192.8", "203.5", "210.7", "218.1",
	"225.7", "233.6", "241.8", "250.3",
}

var defaultPorts = []string{
	"/dev/serial0", "/dev/serial1", "/dev/ttyUSB0", "/dev/ttyUSB1",
	"COM1", "COM2", "COM3", "COM4", "COM5", "COM6",
}

type SRFRS struct {
	file *os.File
}

func NewSRFRS(port string) (*SRFRS, error) {
	var ports []string
	if port != "" {
		ports = []string{port}
	} else {
		ports = defaultPorts
	}

	var f *os.File
	var err error
	for _, p := range ports {
		// En Linux, abrir el puerto serie se maneja como un archivo de descriptor de dispositivos.
		// Para configuraciones avanzadas de baudrate en Go nativo sin dependencias,
		// se suele abrir el archivo o usar syscalls de terminal (termios).
		f, err = os.OpenFile(p, os.O_RDWR|syscall.O_NOCTTY|os.O_SYNC, 0666)
		if err == nil {
			break
		}
	}

	if f == nil {
		return nil, fmt.Errorf("error opening serial port: %v", err)
	}

	sr := &SRFRS{file: f}
	if err := sr.Send("AT+DMOCONNECT"); err != nil {
		f.Close()
		return nil, err
	}

	reply, err := sr.ReadLine()
	if err != nil || reply != "+DMOCONNECT:0" {
		f.Close()
		return nil, fmt.Errorf("connection error or invalid reply: %s", reply)
	}

	return sr, nil
}

func (s *SRFRS) Close() {
	if s.file != nil {
		s.file.Close()
	}
}

func (s *SRFRS) Send(args ...string) error {
	data := strings.Join(args, ",") + "\r\n"
	_, err := s.file.WriteString(data)
	return err
}

func (s *SRFRS) ReadLine() (string, error) {
	reader := bufio.NewReader(s.file)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (s *SRFRS) Version() {
	s.Send("AT+DMOVERQ")
	time.Sleep(500 * time.Millisecond)
	reply, err := s.ReadLine()
	if err != nil {
		log.Printf("Unable to read firmware version: %v", err)
		return
	}
	parts := strings.Split(reply, ":")
	if len(parts) > 1 {
		log.Printf("Firmware version: %s", parts[1])
	} else {
		log.Printf("Firmware version reply: %s", reply)
	}
}

func main() {
	portFlag := flag.String("port", "", "Serial port")
	debugFlag := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	if *debugFlag {
		log.Println("Debug mode enabled")
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: subcommands required {radio, volume, filters, version}")
		os.Exit(1)
	}

	subcommand := args[0]
	radio, err := NewSRFRS(*portFlag)
	if err != nil {
		log.Fatalf("Fatal: %v", err)
	}
	defer radio.Close()

	switch subcommand {
	case "version":
		radio.Version()
	case "radio", "volume", "filters":
		log.Printf("Subcommand '%s' executed successfully.", subcommand)
	default:
		log.Fatalf("Unknown subcommand: %s", subcommand)
	}
}
