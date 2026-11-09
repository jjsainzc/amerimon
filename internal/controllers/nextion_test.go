package controllers

import (
	"amerimon/internal/config"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNextionRenderSendsDateAnd24HourTime(t *testing.T) {
	panel := &panel{sent: make(map[string]string)}
	var output bytes.Buffer
	if err := panel.render(&output); err != nil {
		t.Fatal(err)
	}

	commands := make(map[string]string)
	for _, frame := range bytes.Split(output.Bytes(), []byte{0xff, 0xff, 0xff}) {
		if len(frame) == 0 {
			continue
		}
		command := string(frame)
		key, value, ok := strings.Cut(command, "=")
		if ok {
			commands[key] = value
		}
	}

	gotDate := strings.Trim(commands["t_date.txt"], `"`)
	date, err := time.Parse("2006-01-02", gotDate)
	if err != nil || date.Format("2006-01-02") != gotDate {
		t.Errorf("t_date.txt = %q; want YYYY-MM-DD format", gotDate)
	}
	gotTime := strings.Trim(commands["t_time.txt"], `"`)
	parsedTime, err := time.Parse("15:04:05", gotTime)
	if err != nil || parsedTime.Format("15:04:05") != gotTime {
		t.Errorf("t_time.txt = %q; want HH:mm:ss in 24-hour format: %v", gotTime, err)
	}
}

func TestParseNextionSIPChannelsUsesCallerIDAndKeepsChannelID(t *testing.T) {
	raw := "Response: Follows\r\n" +
		"Output: PJSIP/phone-portal-00000001!from-internal!616262!1!Up!AppDial!data!EA4ABC!00:00:10!!!bridge\r\n" +
		"Output: PJSIP/another-00000002!from-internal!616262!1!Down!AppDial!data!EA4XYZ!00:00:10!!!bridge\r\n" +
		"Output: IAX2/remote-00000003!from-internal!616262!1!Up!AppDial!data!N0CALL!00:00:10!!!bridge\r\n" +
		"--END COMMAND--\r\n"

	channels := parseNextionSIPChannels(raw, map[string]string{"phone-portal": "Configured ID"})
	if len(channels) != 1 {
		t.Fatalf("expected one active PJSIP channel, got %+v", channels)
	}
	if channels[0].ID != "SIP:PJSIP/phone-portal-00000001" || channels[0].CallerID != "EA4ABC" {
		t.Fatalf("unexpected SIP channel metadata: %+v", channels[0])
	}
}

func TestNextionNodeLabelsUseEchoLinkAndIAXRptCallsigns(t *testing.T) {
	tests := []struct {
		name         string
		node         string
		info         AstDBEntry
		echoCallsign string
		want         string
	}{
		{name: "EchoLink lookup", node: "3001234", echoCallsign: "K1ABC", want: "EchoLink K1ABC"},
		{name: "EchoLink AstDB", node: "3001234", info: AstDBEntry{Callsign: "K2XYZ"}, want: "EchoLink K2XYZ"},
		{name: "IAXRpt callsign", node: "700001", info: AstDBEntry{Callsign: "EA4ABC"}, want: "IAXRpt EA4ABC"},
		{name: "IAXRpt fallback", node: "700001", want: "IAXRpt 700001"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nextionNodeLabel(test.node, test.info, test.echoCallsign); got != test.want {
				t.Fatalf("nextionNodeLabel() = %q; want %q", got, test.want)
			}
		})
	}
}

func TestAMIStatusIncludesSIPIAXRptAndEchoLinkLabels(t *testing.T) {
	dir := t.TempDir()
	astdbPath := filepath.Join(dir, "astdb.txt")
	privateNodesPath := filepath.Join(dir, "privatenodes.txt")
	pjsipPath := filepath.Join(dir, "pjsip.conf")
	for path, content := range map[string]string{
		astdbPath:        "700001|EA4ABC|IAXRpt station|Madrid\n",
		privateNodesPath: "",
		pjsipPath:        "[phone-portal]\ntype=endpoint\ncallerid=K6ABC\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	previousConfig := config.App
	t.Cleanup(func() { config.App = previousConfig })
	config.App.Sources.AstDBPath = astdbPath
	config.App.Sources.PrivateNodesPath = privateNodesPath
	config.App.AMI.PJSIPConfigPath = pjsipPath

	host, port := mockConnectionAMI(t, func(fields map[string]string) string {
		switch fields["Command"] {
		case "NodeStat":
			return "Response: Success\r\nTransmitterKeyed: NO\r\nSignalOnInput: NO\r\n" +
				"NodesCurrentlyConnectedToUs: 3001234,700001\r\n\r\n"
		case "echolink dbget nodename 1234":
			return "Response: Follows\r\nOutput: 1234|K1ABC|Remote station\r\n--END COMMAND--\r\n\r\n"
		case "core show channels concise":
			return "Response: Follows\r\n" +
				"Output: PJSIP/phone-portal-00000001!from-internal!616262!1!Up!AppDial!data!<unknown>!00:00:10!!!bridge\r\n" +
				"Output: --END COMMAND--\r\n\r\n"
		default:
			return fmt.Sprintf("Response: Error\r\nMessage: unexpected command %q\r\n\r\n", fields["Command"])
		}
	})
	status, err := (&ami{host: host, port: port, username: "test", secret: "test"}).status()
	if err != nil {
		t.Fatal(err)
	}
	wantLabels := map[string]string{
		"3001234":                         "EchoLink K1ABC",
		"700001":                          "IAXRpt EA4ABC",
		"SIP:PJSIP/phone-portal-00000001": "SIP K6ABC",
	}
	for node, want := range wantLabels {
		if got := status.labels[node]; got != want {
			t.Errorf("label for %q = %q; want %q", node, got, want)
		}
		if !hasNode(status.nodes, node) {
			t.Errorf("active link %q missing from status: %v", node, status.nodes)
		}
	}
}

func TestNextionPanelDisplaysLabelsAndDisconnectsPressedSlotTarget(t *testing.T) {
	const sipChannel = "SIP:PJSIP/phone-portal-00000001"
	radio := &nextionTestRadio{current: radioStatus{
		nodes: []string{sipChannel, "3001234", "700001"},
		labels: map[string]string{
			sipChannel: "SIP EA4ABC", "3001234": "EchoLink K1ABC", "700001": "IAXRpt EA4XYZ",
		},
	}}
	p := &panel{radio: radio, sent: make(map[string]string)}
	p.refresh()
	if p.slots != [4]string{sipChannel, "3001234", "700001", ""} ||
		p.slotLabels != [4]string{"SIP EA4ABC", "EchoLink K1ABC", "IAXRpt EA4XYZ", ""} {
		t.Fatalf("unexpected panel slots: ids=%q labels=%q", p.slots, p.slotLabels)
	}

	var output bytes.Buffer
	if err := p.render(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "t_n1.txt=\"SIP EA4ABC\"") ||
		strings.Contains(output.String(), "t_n1.txt=\""+sipChannel+"\"") {
		t.Fatalf("render did not send the SIP caller ID as the visible label: %q", output.String())
	}

	p.read([]byte("HOLD_N1 SIP EA4ABC\n"))
	if p.held[0].node != sipChannel {
		t.Fatalf("hold target = %q; want connection ID %q", p.held[0].node, sipChannel)
	}
	p.read([]byte("DISC_N1"))
	if radio.target != sipChannel {
		t.Fatalf("disconnect target = %q; want connection ID %q", radio.target, sipChannel)
	}
}

func TestNextionSIPDisconnectRequestsHangupForValidatedChannel(t *testing.T) {
	host, port := mockConnectionAMI(t, func(fields map[string]string) string {
		if fields["Command"] != "channel request hangup PJSIP/phone-portal-00000001" {
			t.Errorf("unexpected AMI command: %q", fields["Command"])
		}
		return "Response: Follows\r\nOutput: Command sent\r\n--END COMMAND--\r\n\r\n"
	})
	radio := &ami{host: host, port: port, username: "test", secret: "test"}
	if err := radio.link("1", "SIP:PJSIP/phone-portal-00000001"); err != nil {
		t.Fatal(err)
	}
	if err := radio.link("1", "SIP:PJSIP/phone-portal;Application(agi)"); err == nil {
		t.Fatal("invalid SIP channel was accepted")
	}
}

type nextionTestRadio struct {
	current radioStatus
	target  string
}

func (r *nextionTestRadio) status() (radioStatus, error) {
	return r.current, nil
}

func (r *nextionTestRadio) link(_, node string) error {
	r.target = node
	return nil
}

func (*nextionTestRadio) close() {}
