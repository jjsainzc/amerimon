package controllers

// Este archivo contiene pruebas para la validación y construcción de canales para Listen Live.

import (
	"amerimon/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListenLiveOriginateChannel(t *testing.T) {
	previous := config.App
	defer func() { config.App = previous }()
	path := filepath.Join(t.TempDir(), "rpt.conf")
	config.App.AMI.RPTConfigPath = path
	for _, rxchannel := range []string{"SimpleUSB/616265", "Radio/616265", "Local/pseudo", "DAHDI/pseudo"} {
		t.Run(rxchannel, func(t *testing.T) {
			content := "[node-main](!)\nrxchannel = " + rxchannel +
				"\n[616265](node-main)\n[nodes]\n616262 = radio@192.0.2.10/616262,NONE\n616265 = radio@127.0.0.1/616265,NONE\n"
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			channel, err := listenLiveOriginateChannel("nodo")
			if err != nil || channel != "Local/616265@amerimon-listen-node/n" {
				t.Fatalf("wrong rxchannel context: %q, error=%v", channel, err)
			}
		})
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := listenLiveOriginateChannel("nodo"); err == nil {
		t.Fatal("missing rpt.conf must fail explicitly")
	}
	if err := os.WriteFile(path, []byte("[nodes]\n616262 = radio@192.0.2.10/616262,NONE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := listenLiveOriginateChannel("nodo"); err == nil {
		t.Fatal("missing principal must not fall back to another node")
	}
	channel, err := listenLiveOriginateChannel("1005")
	if err != nil || channel != "Local/1005@listen-live-spy/n" {
		t.Fatalf("PJSIP path changed: %q, error=%v", channel, err)
	}
}

func TestListenLiveRXChannelsCanBeStopped(t *testing.T) {
	uuid := "12345678-1234-1234-1234-123456789012"
	channel := "Local/616265@amerimon-listen-node-00000001"
	output := channel + ";1!listen-live-eagi!" + uuid + "!3!Up!EAGI!audio\n" +
		channel + ";2!amerimon-listen-node!616265!11!Up!ChanSpy!Local/pseudo@default,q\n"
	channels := listenLiveChannels(output, uuid)
	if len(channels) != 2 || !strings.HasSuffix(channels[0], ";1") || !strings.HasSuffix(channels[1], ";2") {
		t.Fatalf("expected both monitor channel legs, got %v", channels)
	}
}
