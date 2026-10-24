package config

import (
	"bufio"
	"fmt"
	"strings"
	"time"
)

// ExecuteAMIAudioAction solo permite consultar soporte o cambiar el audio entrante de un canal.
func ExecuteAMIAudioAction(host, port, user, secret, channel string, muted *bool) (string, error) {
	if muted != nil && (channel == "" || strings.ContainsAny(channel, "\r\n")) {
		return "", fmt.Errorf("canal de audio inválido")
	}
	conn, err := dialAMI(host, port, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	if _, err := reader.ReadString('\n'); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintf(conn, "Action: Login\r\nUsername: %s\r\nSecret: %s\r\nEvents: off\r\n\r\n", user, secret); err != nil {
		return "", err
	}
	login, err := readAMIMessage(reader)
	if err != nil {
		return "", err
	}
	if !strings.Contains(login, "Response: Success") {
		return "", fmt.Errorf("login AMI rechazado")
	}
	id := fmt.Sprintf("amerimon-audio-%d", time.Now().UnixNano())
	action := "Action: ListCommands\r\n"
	if muted != nil {
		state := "off"
		if *muted {
			state = "on"
		}
		action = fmt.Sprintf("Action: MuteAudio\r\nChannel: %s\r\nDirection: in\r\nState: %s\r\n", channel, state)
	}
	if _, err := fmt.Fprintf(conn, "%sActionID: %s\r\n\r\n", action, id); err != nil {
		return "", err
	}
	response, err := readAMIActionResponse(reader, id)
	if err != nil {
		return "", err
	}
	if err := amiResponseError(response); err != nil {
		return "", err
	}
	if !strings.Contains(response, "Response: Success") {
		return "", fmt.Errorf("AMI no confirmó la operación de audio")
	}
	return response, nil
}
