package controller

import (
	"context"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Some macOS/kernel combinations cannot populate process metadata. Resolve
// missing applications from the owning user's socket table, never host guesses.
// Absolute executable paths are used only to find the containing app bundle.
func populateConnectionApps(ctx context.Context, connections []trackedConnection) {
	if runtime.GOOS != "darwin" {
		return
	}
	needed := false
	for _, conn := range connections {
		if conn.Metadata.Process == "" && conn.Metadata.ProcessPath == "" && socketKey(conn.Metadata.Network, conn.Metadata.SourceIP, string(conn.Metadata.SourcePort)) != "" {
			needed = true
			break
		}
	}
	if !needed {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	output, _ := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-a", "-u", strconv.Itoa(os.Getuid()), "-iTCP", "-iUDP", "+c0", "-FpcPn").Output()
	owners := parseSocketOwners(string(output))
	pids := map[string]bool{}
	for _, conn := range connections {
		if owner, ok := owners[socketKey(conn.Metadata.Network, conn.Metadata.SourceIP, string(conn.Metadata.SourcePort))]; ok && owner.pid != "" {
			pids[owner.pid] = true
		}
	}
	if len(pids) == 0 {
		return
	}
	ids := make([]string, 0, len(pids))
	for pid := range pids {
		ids = append(ids, pid)
	}
	paths, _ := exec.CommandContext(ctx, "/bin/ps", "-p", strings.Join(ids, ","), "-o", "pid=,comm=").Output()
	processPaths := parseProcessPaths(string(paths))
	for i := range connections {
		metadata := &connections[i].Metadata
		if metadata.Process != "" || metadata.ProcessPath != "" {
			continue
		}
		owner, ok := owners[socketKey(metadata.Network, metadata.SourceIP, string(metadata.SourcePort))]
		if !ok || owner.pid == "" {
			continue
		}
		metadata.Process, metadata.ProcessPath = owner.command, processPaths[owner.pid]
	}
}

type socketOwner struct{ pid, command string }

func socketKey(network, ip, port string) string {
	parsed := net.ParseIP(strings.Trim(ip, "[]"))
	n, err := strconv.Atoi(port)
	if parsed == nil || err != nil || n < 1 || n > 65535 {
		return ""
	}
	network = strings.ToLower(network)
	if network != "tcp" && network != "udp" {
		return ""
	}
	return network + "|" + net.JoinHostPort(parsed.String(), strconv.Itoa(n))
}

func parseSocketOwners(output string) map[string]socketOwner {
	owners := map[string]socketOwner{}
	var owner socketOwner
	var network string
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			owner = socketOwner{pid: line[1:]}
			network = ""
		case 'c':
			owner.command = line[1:]
		case 'P':
			network = line[1:]
		case 'n':
			// Only connected sockets supply evidence; wildcard/listening endpoints
			// cannot identify an application's outgoing connection reliably.
			local, _, connected := strings.Cut(line[1:], "->")
			if !connected {
				continue
			}
			ip, port, err := net.SplitHostPort(local)
			if err != nil {
				continue
			}
			key := socketKey(network, ip, port)
			if key == "" {
				continue
			}
			if previous, exists := owners[key]; exists && previous.pid != owner.pid {
				// Reused local ports across applications are ambiguous.
				owners[key] = socketOwner{}
			} else if !exists {
				owners[key] = owner
			}
		}
	}
	return owners
}

func parseProcessPaths(output string) map[string]string {
	paths := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		separator := strings.IndexAny(line, " \t")
		if separator < 1 {
			continue
		}
		pid := line[:separator]
		if _, err := strconv.Atoi(pid); err == nil {
			paths[pid] = strings.TrimSpace(line[separator:])
		}
	}
	return paths
}
