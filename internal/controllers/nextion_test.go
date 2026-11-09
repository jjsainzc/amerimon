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

func TestNextionNodeLabelsShowOnlyCallsignsOrNumbers(t *testing.T) {
	tests := []struct {
		name         string
		node         string
		info         AstDBEntry
		echoCallsign string
		want         string
	}{
		{name: "EchoLink lookup", node: "3001234", echoCallsign: "K1ABC", want: "K1ABC"},
		{name: "EchoLink AstDB", node: "3001234", info: AstDBEntry{Callsign: "K2XYZ"}, want: "K2XYZ"},
		{name: "EchoLink number fallback", node: "3001234", want: "3001234"},
		{name: "numeric node keeps its number", node: "616262", info: AstDBEntry{Callsign: "EA4ABC"}, want: "616262"},
		{name: "IAXRpt shows callsign only", node: "EA4ABC", info: AstDBEntry{Callsign: "EA4ABC"}, want: "EA4ABC"},
		{name: "IAXRpt fallback", node: "EA4ABC", want: "EA4ABC"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nextionNodeLabel(test.node, test.info, test.echoCallsign); got != test.want {
				t.Fatalf("nextionNodeLabel() = %q; want %q", got, test.want)
			}
		})
	}
}

func TestNextionActivityLabelIncludesOnlyAvailableDescription(t *testing.T) {
	for _, test := range []struct {
		name, label, description, want string
	}{
		{name: "number with description", label: "700001", description: "Remote station", want: "700001 - Remote station"},
		{name: "callsign with description", label: "HC1", description: "Club radio", want: "HC1 - Club radio"},
		{name: "missing description", label: "700001", want: "700001"},
		{name: "blank description", label: "HC1", description: " \t ", want: "HC1"},
		{name: "unsafe display characters", label: "700001", description: " \"Remote\"\nstation ", want: "700001 - Remotestation"},
	} {
		t.Run(test.name, func(t *testing.T) {
			info := AstDBEntry{Descripcion: test.description, Ubicacion: "Not part of activity text"}
			if got := nextionActivityLabel(test.label, info); got != test.want {
				t.Fatalf("activity label = %q; want %q", got, test.want)
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
				"NodesCurrentlyConnectedToUs: 3001234,700001,HC1\r\n\r\n"
		case "SawStat":
			return "Response: Success\r\nConn: 3001234 0 0 0\r\nConn: 700001 0 0 0\r\nConn: HC1 0 0 0\r\n\r\n"
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
		"3001234":                         "K1ABC",
		"700001":                          "700001",
		"HC1":                             "HC1",
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

func TestNextionStatusPrioritizesReceptionAndRendersMatchingColors(t *testing.T) {
	setupReceiveActivityTest(t)
	dir := t.TempDir()
	for _, name := range []string{"astdb.txt", "privatenodes.txt", "pjsip.conf"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config.App.Sources.AstDBPath = filepath.Join(dir, "astdb.txt")
	config.App.Sources.PrivateNodesPath = filepath.Join(dir, "privatenodes.txt")
	config.App.AMI.PJSIPConfigPath = filepath.Join(dir, "pjsip.conf")

	for _, test := range []struct {
		name, tx, rx, sawStat string
		astdb, privateNodes   string
		sipVoice, sipUp       bool
		wantLabel             string
		wantColor             int
		wantError             bool
	}{
		{name: "idle", tx: "NO", rx: "NO", wantLabel: "", wantColor: idleColor},
		{name: "transmit only", tx: "YES", rx: "NO", wantLabel: "616265", wantColor: txColor},
		{name: "local reception", tx: "NO", rx: "YES", wantLabel: "616265", wantColor: rxColor},
		{name: "local reception overrides TX", tx: "YES", rx: "YES", wantLabel: "616265", wantColor: rxColor},
		{name: "IAX reception overrides TX", tx: "YES", rx: "NO", sawStat: "Conn: 700001 1 90 -1", wantLabel: "700001", wantColor: rxColor},
		{name: "IAXRpt reception overrides TX", tx: "YES", rx: "NO", sawStat: "Conn: HC1 1 0 -1", wantLabel: "HC1", wantColor: rxColor},
		{name: "EchoLink reception overrides TX", tx: "YES", rx: "NO", sawStat: "Conn: 3001234 1 0 -1", wantLabel: "3001234", wantColor: rxColor},
		{name: "recent unkey is not reception", tx: "YES", rx: "NO", sawStat: "Conn: HC1 0 0 0", wantLabel: "616265", wantColor: txColor},
		{name: "SIP voice overrides TX", tx: "YES", rx: "NO", sipVoice: true, sipUp: true, wantLabel: "1005", wantColor: rxColor},
		{name: "simultaneous sources", tx: "YES", rx: "YES", sawStat: "Conn: HC1 1 0 -1\r\nConn: 700001 1 0 -1\r\nConn: HC1 1 0 -1\r\nConn: HC2 0 0 0", sipVoice: true, sipUp: true, wantLabel: "1005, 616265, 700001, HC1", wantColor: rxColor},
		{name: "connected SIP without voice is not reception", tx: "YES", rx: "NO", sawStat: "Conn: SIP/phone-portal 1 0 -1", sipUp: true, wantLabel: "616265", wantColor: txColor},
		{name: "disconnected SIP voice is ignored", tx: "NO", rx: "NO", sipVoice: true, wantLabel: "", wantColor: idleColor},
		{name: "active IAX node description", tx: "YES", rx: "NO", sawStat: "Conn: 700001 1 0 -1", astdb: "700001|EA4ABC|Remote station|Madrid\n", wantLabel: "700001 - Remote station", wantColor: rxColor},
		{name: "active IAXRpt description", tx: "YES", rx: "NO", sawStat: "Conn: HC1 1 0 -1", astdb: "HC1|HC1|Club radio|Quito\n", wantLabel: "HC1 - Club radio", wantColor: rxColor},
		{name: "active EchoLink description", tx: "YES", rx: "NO", sawStat: "Conn: 3001234 1 0 -1", astdb: "3001234|K1ABC|EchoLink station|Boston\n", wantLabel: "K1ABC - EchoLink station", wantColor: rxColor},
		{name: "local TX description", tx: "YES", rx: "NO", astdb: "616265|HC1|Local station|Quito\n", wantLabel: "616265 - Local station", wantColor: txColor},
		{name: "local COS description", tx: "NO", rx: "YES", astdb: "616265|HC1|Local station|Quito\n", wantLabel: "616265 - Local station", wantColor: rxColor},
		{name: "private node description takes precedence", tx: "YES", rx: "NO", sawStat: "Conn: 1998 1 0 -1", astdb: "1998|HC1|Public description|Quito\n", privateNodes: "1998|HC1|Private station|Quito\n", wantLabel: "1998 - Private station", wantColor: rxColor},
		{name: "malformed reception reports error", tx: "YES", rx: "NO", sawStat: "Conn: HC1 2 0 0", wantError: true},
		{name: "failed reception query reports error", tx: "YES", rx: "NO", sawStat: "Response: Error\r\nMessage: simulated failure", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for name, content := range map[string]string{
				"astdb.txt":        test.astdb,
				"privatenodes.txt": test.privateNodes,
			} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			host, port := mockConnectionAMI(t, func(fields map[string]string) string {
				switch fields["Command"] {
				case "NodeStat":
					return fmt.Sprintf("Response: Success\r\nTransmitterKeyed: %s\r\nSignalOnInput: %s\r\nNodesCurrentlyConnectedToUs: \r\n\r\n", test.tx, test.rx)
				case "SawStat":
					if strings.HasPrefix(test.sawStat, "Response: Error") {
						return test.sawStat + "\r\n\r\n"
					}
					return "Response: Success\r\n" + test.sawStat + "\r\n\r\n"
				case "core show channels concise":
					output := ""
					if test.sipUp {
						output = "Output: PJSIP/phone-portal-00000001!from-internal!616265!1!Up!AppDial!data!1005\r\n"
					}
					return "Response: Follows\r\n" + output + "--END COMMAND--\r\n\r\n"
				default:
					return "Response: Error\r\nMessage: unexpected command\r\n\r\n"
				}
			})
			if test.sipVoice {
				updateSIPTalkingEvent(sipActivityServer{host, port, "test", "test"}, map[string]string{
					"Event": "ChannelTalkingStart", "Channel": "PJSIP/phone-portal-00000001",
					"Uniqueid": "1", "Exten": "616265",
				})
			}
			status, err := (&ami{host: host, port: port, username: "test", secret: "test", node: "616265"}).status()
			if test.wantError {
				if err == nil {
					t.Fatal("expected reception query error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if status.label != test.wantLabel || status.color != test.wantColor {
				t.Fatalf("status=%+v; want label %q and color %d", status, test.wantLabel, test.wantColor)
			}
			for node, label := range status.labels {
				if strings.Contains(label, " - ") {
					t.Errorf("link row %q unexpectedly includes a description: %q", node, label)
				}
			}
			p := &panel{status: status, sent: make(map[string]string)}
			var output bytes.Buffer
			if err := p.render(&output); err != nil {
				t.Fatal(err)
			}
			for _, command := range []string{
				fmt.Sprintf("b_estado.bco=%d", test.wantColor),
				fmt.Sprintf("t0.bco=%d", test.wantColor),
				fmt.Sprintf("t0.txt=%q", test.wantLabel),
			} {
				if !bytes.Contains(output.Bytes(), append([]byte(command), 0xff, 0xff, 0xff)) {
					t.Errorf("missing Nextion frame %q", command)
				}
			}
			if test.sipVoice && test.sipUp {
				updateSIPTalkingEvent(sipActivityServer{host, port, "test", "test"}, map[string]string{
					"Event": "ChannelTalkingStop", "Channel": "PJSIP/phone-portal-00000001",
					"Uniqueid": "1",
				})
				status, err := (&ami{host: host, port: port, username: "test", secret: "test", node: "616265"}).status()
				wantLabel, wantColor := "616265", txColor
				if test.rx == "YES" {
					wantLabel, wantColor = "616265, 700001, HC1", rxColor
				}
				if err != nil || status.color != wantColor || status.label != wantLabel {
					t.Fatalf("SIP voice stop did not restore TX status: status=%+v error=%v", status, err)
				}
			}
		})
	}
}

func TestNextionPanelDisplaysLabelsAndDisconnectsPressedSlotTarget(t *testing.T) {
	const sipChannel = "SIP:PJSIP/phone-portal-00000001"
	radio := &nextionTestRadio{current: radioStatus{
		nodes: []string{sipChannel, "3001234", "700001", "HC1"},
		labels: map[string]string{
			sipChannel: "SIP EA4ABC", "3001234": "K1ABC", "700001": "700001", "HC1": "HC1",
		},
	}}
	p := &panel{radio: radio, sent: make(map[string]string)}
	p.refresh()
	if p.slots != [4]string{sipChannel, "3001234", "700001", "HC1"} ||
		p.slotLabels != [4]string{"SIP EA4ABC", "K1ABC", "700001", "HC1"} {
		t.Fatalf("unexpected panel slots: ids=%q labels=%q", p.slots, p.slotLabels)
	}

	var output bytes.Buffer
	if err := p.render(&output); err != nil {
		t.Fatal(err)
	}
	for i, label := range p.slotLabels {
		if !strings.Contains(output.String(), fmt.Sprintf("t_n%d.txt=\"%s\"", i+1, label)) {
			t.Fatalf("render did not send label %q: %q", label, output.String())
		}
	}

	p.read([]byte("HOLD_N1 SIP EA4ABC\n"))
	if p.held[0].node != sipChannel {
		t.Fatalf("hold target = %q; want connection ID %q", p.held[0].node, sipChannel)
	}
	p.read([]byte("DISC_N1"))
	if radio.target != sipChannel {
		t.Fatalf("disconnect target = %q; want connection ID %q", radio.target, sipChannel)
	}
	p.read([]byte("HOLD_N2 K1ABC\nDISC_N2"))
	if radio.target != "3001234" {
		t.Fatalf("disconnect target = %q; want EchoLink node number", radio.target)
	}
	p.read([]byte("HOLD_N4 HC1\nDISC_N4"))
	if radio.target != "HC1" {
		t.Fatalf("disconnect target = %q; want IAXRpt callsign", radio.target)
	}
}

func TestNextionActivityTextClearsAndPreservesSystemMessages(t *testing.T) {
	p := &panel{status: radioStatus{label: "HC1", color: rxColor}, sent: make(map[string]string)}
	var output bytes.Buffer
	if err := p.render(&output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	p.status = radioStatus{color: idleColor}
	if err := p.render(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `t0.txt=""`) {
		t.Fatalf("idle did not clear the audio source: %q", output.String())
	}
	output.Reset()
	p.status = radioStatus{label: "HC2", color: rxColor}
	p.systemMessage = "Reiniciando Asterisk"
	p.messageUntil = time.Now().Add(time.Minute)
	if err := p.render(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `t0.txt="Reiniciando Asterisk"`) {
		t.Fatalf("temporary system message was lost: %q", output.String())
	}
	output.Reset()
	p.messageUntil = time.Time{}
	if err := p.render(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `t0.txt="HC2"`) {
		t.Fatalf("current source was not restored after the system message: %q", output.String())
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
