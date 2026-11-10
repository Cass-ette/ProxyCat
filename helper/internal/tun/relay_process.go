package tun

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// Reserve distinct private endpoints while preparation is still harmless. Only
// root owns these ports; the next ordinary core never has to rebind either one.
func preparePrivateListeners(config string) (relaySettings, func(), error) {
	var settings relaySettings
	proxy, err := net.Listen("tcp4", loopback(0))
	if err != nil {
		return settings, nil, err
	}
	api, err := net.Listen("tcp4", loopback(0))
	if err != nil {
		proxy.Close()
		return settings, nil, err
	}
	release := func() { proxy.Close(); api.Close() }
	settings.ProxyPort = proxy.Addr().(*net.TCPAddr).Port
	settings.ControllerPort = api.Addr().(*net.TCPAddr).Port
	content, err := os.ReadFile(config)
	var cfg map[string]interface{}
	if err == nil {
		err = yaml.Unmarshal(content, &cfg)
	}
	if err == nil && cfg == nil {
		err = fmt.Errorf("empty TUN configuration")
	}
	if err == nil {
		cfg["mixed-port"] = settings.ProxyPort
		cfg["external-controller"] = loopback(settings.ControllerPort)
		content, err = yaml.Marshal(cfg)
	}
	if err == nil {
		err = os.WriteFile(config, content, 0600)
	}
	if err != nil {
		release()
		return settings, nil, err
	}
	return settings, release, nil
}

type relayProcess struct {
	command *exec.Cmd
	done    chan struct{}
}

// Start the installed, root-owned executable after dropping all user/group
// privileges. Listening in a root process and only chowning files is not enough:
// macOS checks the credentials which created the TCP socket, including TIME_WAIT.
func startUserRelay(s session, settings relaySettings, log io.Writer, failed chan<- error) (*relayProcess, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(settings)
	command := exec.Command(executable, "tun", "relay")
	command.Dir = filepath.Dir(executable)
	command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL=C"}
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
		Uid: uint32(s.UID), Gid: uint32(s.GID), Groups: []uint32{uint32(s.GID)},
	}}
	command.Stdin, command.Stderr = bytes.NewReader(data), log
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		output.Close()
		return nil, err
	}
	relay := &relayProcess{command: command, done: make(chan struct{})}
	go func() {
		err := command.Wait()
		close(relay.done)
		select {
		case failed <- fmt.Errorf("TUN 本地入口已退出：%v", err):
		default:
		}
	}()
	ack := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(output).ReadString('\n')
		if err == nil && line != "ready\n" {
			err = fmt.Errorf("invalid local relay acknowledgement")
		}
		ack <- err
	}()
	defer output.Close()
	select {
	case err = <-ack:
	case <-time.After(3 * time.Second):
		err = fmt.Errorf("TUN 本地入口未能及时就绪")
	}
	if err != nil {
		relay.stop()
		return nil, err
	}
	return relay, nil
}

func (r *relayProcess) stop() {
	if r == nil {
		return
	}
	select {
	case <-r.done:
		return
	default:
	}
	_ = r.command.Process.Signal(syscall.SIGTERM)
	select {
	case <-r.done:
	case <-time.After(time.Second):
		_ = r.command.Process.Kill()
		<-r.done
	}
}
