package diagnose

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Cass-ette/ProxyCat/helper/internal/controller"
	"github.com/Cass-ette/ProxyCat/helper/internal/core"
	"github.com/Cass-ette/ProxyCat/helper/internal/paths"
	"github.com/Cass-ette/ProxyCat/helper/internal/sysproxy"
	"github.com/Cass-ette/ProxyCat/helper/internal/tun"
)

const proxyPort = 7890
const testURL = "https://www.gstatic.com/generate_204"

// All dependencies are read-only; tests replace them without touching live networking.
type probes struct {
	core       func(context.Context) (bool, int, error)
	config     func(context.Context) (*controller.Config, error)
	port       func(context.Context, int) error
	proxy      func(context.Context) (*sysproxy.Status, error)
	website    func(context.Context, string) (*controller.WebsiteResult, error)
	tunReady   func(*controller.Config) bool
	tunSession bool
}

// Run never starts, stops, patches or switches anything, even on failure.
func Run(ctx context.Context, p paths.RuntimePaths) Report {
	c := tun.Client(p)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	c.SetClient(&http.Client{Transport: transport, Timeout: 2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
	_, sessionErr := os.Stat(filepath.Join(p.Config, "tun-session.json"))
	return run(ctx, p, probes{
		core: core.StatusContext, config: c.GetConfigContext, proxy: sysproxy.GetStatusContext,
		port: probePort, website: func(ctx context.Context, input string) (*controller.WebsiteResult, error) {
			return c.CheckWebsiteAtPort(ctx, input, proxyPort)
		}, tunReady: tun.Ready,
		tunSession: sessionErr == nil || !os.IsNotExist(sessionErr),
	})
}

func probePort(ctx context.Context, port int) error {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err == nil {
		conn.Close()
	}
	return err
}

func run(ctx context.Context, p paths.RuntimePaths, probe probes) Report {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	report := RunStatic(p)
	report.SchemaVersion = 2
	report.ObservedAt = time.Now().UTC().Format(time.RFC3339)
	report.Scope = "仅检查本地文件、ProxyCat 运行链路和一个 HTTPS 测试地址；不代表所有网站可达，也不证明所有应用已走代理。"
	step, done := context.WithTimeout(ctx, 2*time.Second)
	running, pid, processErr := probe.core(step)
	done()
	step, done = context.WithTimeout(ctx, 2*time.Second)
	cfg, configErr := probe.config(step)
	done()
	configOK := configErr == nil && cfg != nil && (cfg.Mode == "rule" || cfg.Mode == "global" || cfg.Mode == "direct")
	tunEnabled := cfg != nil && cfg.Tun["enable"] == true
	// A stale session is also a reason not to replace an ordinary core blindly.
	canRecover := !probe.tunSession && !tunEnabled && fileExists(p.ConfigYAML) && fileExists(p.Mihomo)
	recovery := "openLogs"
	if canRecover {
		recovery = "recoverCore"
	}
	engine := Check{Name: "core-process", Category: "runtime", Title: "内核进程", Status: StatusPass}
	switch {
	case processErr != nil:
		engine.Status, engine.Message = StatusSkipped, "无法读取进程状态；不能据此判断内核已停止。"
	case !running:
		engine.Status, engine.Message = StatusWarn, "未发现 Mihomo 进程；内核可能尚未启动。"
		if !configOK {
			engine.Action = recovery
		}
	default:
		engine.Message = fmt.Sprintf("发现 Mihomo 进程（PID %d）；进程存在不代表代理可用。", pid)
	}
	report.Checks = append(report.Checks, engine)
	control := Check{Name: "controller", Category: "runtime", Title: "内核控制器", Status: StatusPass,
		Message: "控制器已返回当前配置；不代表代理端口或外网可用。"}
	if !configOK {
		control.Status, control.Message, control.Action = StatusFail, "无法从本地控制器读取有效配置（可能未启动、无响应或认证失败）。", recovery
	}
	report.Checks = append(report.Checks, control)
	step, done = context.WithTimeout(ctx, 800*time.Millisecond)
	portErr := probe.port(step, proxyPort)
	done()
	listenerReady := configOK && cfg.MixedPort == proxyPort && portErr == nil
	listener := Check{Name: "proxy-listener", Category: "runtime", Title: "本地代理 7890", Status: StatusPass,
		Message: "127.0.0.1:7890 接受 TCP 连接，控制器也报告该端口；协议和外网见联网测试。"}
	switch {
	case portErr != nil:
		listener.Status, listener.Message, listener.Action = StatusFail, "127.0.0.1:7890 未接受连接；即使进程还在，当前本地代理也未就绪。", recovery
	case !configOK:
		listener.Status, listener.Message, listener.Action = StatusWarn, "7890 接受连接，但控制器不可用，无法确认它属于当前代理链路。", "openLogs"
	case cfg.MixedPort != proxyPort:
		listener.Status, listener.Message, listener.Action = StatusFail, "7890 接受连接，但当前内核未报告此 mixed-port；可能存在端口冲突或未完成交接。", recovery
	}
	report.Checks = append(report.Checks, listener)
	step, done = context.WithTimeout(ctx, 2*time.Second)
	proxy, proxyErr := probe.proxy(step)
	done()
	capture := Check{Name: "traffic-capture", Category: "runtime", Title: "流量接管", Status: StatusPass}
	switch {
	case tunEnabled:
		if probe.tunReady(cfg) {
			capture.Message = "TUN 接口已启用，系统代理可以关闭；本次显式代理测试不验证每个应用的接管情况。"
		} else {
			capture.Status, capture.Message, capture.Action = StatusWarn, "配置要求开启 TUN，但未确认到已就绪的 TUN 接口；请查看日志。", "openLogs"
		}
	case probe.tunSession:
		capture.Status, capture.Message, capture.Action = StatusWarn, "仍有 TUN 会话记录，尚未确认交接完成；未提供普通内核修复以免干扰隧道。", "openLogs"
	case proxyErr != nil || proxy == nil:
		capture.Status, capture.Message = StatusSkipped, "系统代理状态未能读取；流量是否自动进入 ProxyCat 尚未确认。"
	case proxy.UsesLocalPort(proxyPort) && !listenerReady:
		capture.Status, capture.Message, capture.Action = StatusFail, "系统代理仍指向 7890，但本地链路未就绪，使用系统代理的应用可能无法联网。", recovery
	case proxy.UsesLocalPort(proxyPort):
		capture.Message = "系统代理指向 ProxyCat；不读取系统代理的应用仍可能直连。"
		if !(proxy.HTTPEnabled && proxy.HTTPHost == "127.0.0.1" && proxy.HTTPPort == proxyPort &&
			proxy.HTTPSEnabled && proxy.HTTPSHost == "127.0.0.1" && proxy.HTTPSPort == proxyPort &&
			proxy.SOCKSEnabled && proxy.SOCKSHost == "127.0.0.1" && proxy.SOCKSPort == proxyPort) {
			capture.Status, capture.Message = StatusWarn, "仅部分系统代理协议指向 ProxyCat，不代表所有应用已接管。"
		}
	case proxy.HTTPEnabled || proxy.HTTPSEnabled || proxy.SOCKSEnabled:
		capture.Status, capture.Message = StatusWarn, "系统代理指向其他地址；本次测试显式使用 ProxyCat，不代表应用正在使用它。"
	default:
		capture.Status, capture.Message = StatusWarn, "系统代理与 TUN 均未开启，只有主动使用本地代理的程序会经过 ProxyCat。"
		if listenerReady {
			capture.Action = "enableSystemProxy"
		}
	}
	report.Checks = append(report.Checks, capture)
	network := Check{Name: "https-request", Category: "network", Title: "HTTPS 实际访问", Status: StatusSkipped,
		Message: "本地代理链路尚未确认就绪，未进行外网测试。"}
	if listenerReady && ctx.Err() == nil {
		step, done = context.WithTimeout(ctx, 8*time.Second)
		result, err := probe.website(step, testURL)
		done()
		network = websiteCheck(result, err)
		if result != nil && result.Mode != "" && result.Mode != cfg.Mode {
			network.Status, network.Action = StatusSkipped, ""
			network.Message = "检测期间模式发生变化，本次联网结果不用于确认当前链路；请重新诊断。"
		}
	}
	report.Checks = append(report.Checks, network)
	return report
}

func websiteCheck(result *controller.WebsiteResult, err error) Check {
	check := Check{Name: "https-request", Category: "network", Title: "HTTPS 实际访问", Status: StatusFail, Action: "checkWebsite"}
	prefix := "通过本地代理访问 " + testURL + "："
	if err != nil || result == nil {
		check.Message = prefix + "本次未取得有效测试结果。可以检测具体网站或查看日志。"
		return check
	}
	switch result.Outcome {
	case "responded":
		if result.StatusCode == http.StatusNoContent {
			check.Status, check.Action = StatusPass, ""
			check.Message = prefix + fmt.Sprintf("收到预期 HTTP 204（%d ms）。仅代表该地址在检测时可达。", result.ElapsedMS)
		} else {
			check.Status = StatusWarn
			check.Message = prefix + fmt.Sprintf("收到 HTTP %d，与预期 204 不同，不能确认正常。", result.StatusCode)
		}
	case "timeout":
		check.Message = prefix + "本次超时。可能与当前网络、规则或节点有关，不能仅凭此判断节点故障。"
	case "certificate", "tlsFailed":
		check.Message = prefix + "TLS 或证书验证未通过；未忽略证书错误。请检测具体网站，并检查系统时间或网络拦截。"
	case "proxyUnavailable", "engineUnavailable":
		check.Message = prefix + "测试过程中本地代理或控制器不可用了，请重新诊断。"
	case "redirect", "denied", "httpError":
		check.Status = StatusWarn
		check.Message = prefix + fmt.Sprintf("收到 HTTP %d，与预期 204 不同；目标有响应不等于服务正常。", result.StatusCode)
	default:
		check.Message = prefix + "本次连接未完成。可以检测具体网站，查看实际命中规则和节点后再处理。"
	}
	return check
}
